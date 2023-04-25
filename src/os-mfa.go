package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/term"

	"gopkg.in/yaml.v3"
)

type CloudsConfig struct {
	Clouds map[string]CloudConfig `yaml:"clouds"`
}

type CloudConfig struct {
	AuthType           *string    `yaml:"auth_type,omitempty"`
	Auth               AuthConfig `yaml:"auth,omitempty"`
	RegionName         string     `yaml:"region_name,omitempty"`
	CloudInterface     string     `yaml:"interface,omitempty"`
	IdentityApiVersion int        `yaml:"identity_api_version,omitempty""`
}

type AuthConfig struct {
	AuthUrl           string  `yaml:"auth_url,omitempty"`
	Username          string  `yaml:"username,omitempty"`
	ProjectId         string  `yaml:"project_id,omitempty"`
	ProjectName       string  `yaml:"project_name,omitempty"`
	UserDomainName    *string `yaml:"user_domain_name,omitempty"`
	ProjectDomainName *string `yaml:"project_domain_name,omitempty"`
	Token             *string `yaml:"token,omitempty"`
	Password          *string `yaml:"password,omitempty"`
}

func getToken(auth AuthConfig, passwordAndTOTP string) string {

	url := fmt.Sprintf("%s/%s", auth.AuthUrl, "v3/auth/tokens")

	data := map[string]interface{}{
		"auth": map[string]interface{}{
			"identity": map[string]interface{}{
				"methods": []string{"password"},
				"password": map[string]interface{}{
					"user": map[string]interface{}{
						"name": auth.Username,
						"domain": map[string]string{
							"name": *auth.UserDomainName},
						"password": passwordAndTOTP,
					},
				},
			},
			"scope": map[string]interface{}{
				"project": map[string]string{
					"id": auth.ProjectId,
				},
			},
		},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := http.Post(url, "application/json; charset=UTF-8", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("An Error Occured %v", err)
	}
	defer resp.Body.Close()

	token := resp.Header.Get("x-subject-token")
	return token
}

func findOpenStackConfigFile() (string, error) {

	// Check the current directory
	currentDir, _ := os.Getwd()
	// Check the user's configuration directory
	userHome, _ := os.UserHomeDir()
	userDir := filepath.Join(userHome, ".config", "openstack")

	// Check the system-wide configuration directory
	systemDir := ""
	if runtime.GOOS == "windows" {
		systemDir = filepath.Join("C:", "ProgramData", "openstack")
	} else {
		systemDir = filepath.Join("/", "etc", "openstack")
	}

	searchDirs := []string{currentDir, userDir, systemDir}

	// Prepend value of OS_CLIENT_CONFIG_FILE to search if defined.
	value, present := os.LookupEnv("OS_CLIENT_CONFIG_FILE")
	if present == true {
		searchDirs = append([]string{value}, searchDirs...)
	}

	for _, directory := range searchDirs {
		for _, filename := range []string{"clouds.yaml", "clouds.yml"} {
			path := filepath.Join(directory, filename)
			// log.Printf("Checking search path: %s", path)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				return path, nil
			} else {
				fmt.Println(err)
			}
		}
	}

	return "", errors.New("clouds.yaml file not found")
}

func readCloudsYAML(path string) CloudsConfig {

	// log.Printf("Loading config from file: %s", path)

	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}

	cloudsConfig := CloudsConfig{}
	err = yaml.Unmarshal([]byte(yamlFile), &cloudsConfig)

	if err != nil {
		log.Fatalf("%s", err)
	}

	return cloudsConfig
}

func writeCloudsYAML(path string, config CloudsConfig) {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}
	os.WriteFile(path, bytes, 644)
}

func promptPassword() string {
	fmt.Printf("Enter Password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("")
	return string(bytePassword)
}

func promptTOTP() string {

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("TOTP (press enter to skip): ")
	totp, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	totp = strings.TrimRight(totp, "\n")
	totp = strings.TrimRight(totp, " ")
	return totp
}

func createTokenConfig(config CloudConfig, token string) CloudConfig {

	newAuth := AuthConfig{
		AuthUrl:     config.Auth.AuthUrl,
		ProjectId:   config.Auth.ProjectId,
		ProjectName: config.Auth.ProjectName,
		Token:       &token,
	}

	authTypeStr := "token"

	newConfig := CloudConfig{
		AuthType:           &authTypeStr,
		Auth:               newAuth,
		IdentityApiVersion: config.IdentityApiVersion,
		CloudInterface:     config.CloudInterface,
		RegionName:         config.RegionName,
	}

	return newConfig
}

// Creates a temporary token based authorisation from a long term config
func getEphemeralConfig(config CloudConfig) CloudConfig {

	fmt.Printf("Authenticating '%s' in project '%s'\n", config.Auth.Username, config.Auth.ProjectName)
	password := promptPassword()
	totp := promptTOTP()

	passwordAndTOTP := fmt.Sprintf("%s%s", password, totp)

	token := getToken(
		config.Auth,
		passwordAndTOTP,
	)

	return createTokenConfig(config, token)
}

// Creates a long term config from a default config
func getLongTermConfig(config CloudConfig) CloudConfig {
	newAuth := AuthConfig{
		AuthUrl:        config.Auth.AuthUrl,
		ProjectId:      config.Auth.ProjectId,
		ProjectName:    config.Auth.ProjectName,
		Username:       config.Auth.Username,
		UserDomainName: config.Auth.UserDomainName,
	}
	newConfig := CloudConfig{
		Auth:               newAuth,
		IdentityApiVersion: config.IdentityApiVersion,
		CloudInterface:     config.CloudInterface,
		RegionName:         config.RegionName,
	}
	return newConfig
}

func createEphemeralConfig(longTerm CloudConfig, osCloud string, configPath string, config CloudsConfig) {
	fmt.Printf("Creating Ephemeral Config '%s' in '%s'\n", osCloud, configPath)
	ephemeralConfig := getEphemeralConfig(longTerm)
	config.Clouds[osCloud] = ephemeralConfig
	writeCloudsYAML(configPath, config)
}

func createLongTermConfig(ephemeral CloudConfig, osCloud string, configPath string, config CloudsConfig) {
	fmt.Printf("Creating Long Term Config '%s' in '%s'\n", osCloud, configPath)
	longTermConfig := getLongTermConfig(ephemeral)
	config.Clouds[osCloud] = longTermConfig
	writeCloudsYAML(configPath, config)
}

func main() {

	osCloud := os.Getenv("OS_CLOUD")
	if osCloud == "" {
		fmt.Println("$OS_CLOUD not set.")
		os.Exit(1)
	}

	osCloudLongTerm := fmt.Sprintf("%s-long-term", osCloud)
	configPath, err := findOpenStackConfigFile()

	if err != nil {
		os.Exit(1)
	}

	config := readCloudsYAML(configPath)

	// 1. Check if OS_CLOUD-long-term exist
	if longTerm, ok := config.Clouds[osCloudLongTerm]; ok {
		// If so, use it to create OS_CLOUD
		createEphemeralConfig(longTerm, osCloud, configPath, config)
	} else if defaultConfig, ok := config.Clouds[osCloud]; ok {
		// Otherwise, if OS_CLOUD exits, create OS_CLOUD-long-term
		createLongTermConfig(defaultConfig, osCloudLongTerm, configPath, config)
		// Then create OS_CLOUD from that
		config := readCloudsYAML(configPath)
		longTerm, _ := config.Clouds[osCloudLongTerm]
		createEphemeralConfig(longTerm, osCloud, configPath, config)
	} else {
		fmt.Printf("Could not find '%s' or '%s' in '%s'\n", osCloud, osCloudLongTerm, configPath)
	}
}
