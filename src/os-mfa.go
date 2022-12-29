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

func get_token(auth AuthConfig, password_totp string) string {

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
						"password": password_totp,
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

func find_confg() (string, error) {

	search := []string{
		"./clouds.yaml",
		"~/.config/openstack/clouds.yaml",
		"/etc/openstack/clouds.yaml",
	}

	for _, s := range search {
		_, err := os.Stat(s)
		if err == nil {
			return s, nil
		}
	}

	return "", errors.New("clouds.yaml not found")
}

func read_clouds_yaml(path string) CloudsConfig {

	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}

	clouds_config := CloudsConfig{}
	err = yaml.Unmarshal([]byte(yamlFile), &clouds_config)

	if err != nil {
		log.Fatalf("%s", err)
	}

	return clouds_config
}

func write_clouds_yaml(path string, config CloudsConfig) {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}
	os.WriteFile(path, bytes, 644)
}

func prompt_password() string {
	fmt.Printf("Enter Password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("")
	return string(bytePassword)
}

func prompt_totp() string {

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

func create_token_config(config CloudConfig, token string) CloudConfig {

	new_auth := AuthConfig{
		AuthUrl:     config.Auth.AuthUrl,
		ProjectId:   config.Auth.ProjectId,
		ProjectName: config.Auth.ProjectName,
		Token:       &token,
	}

	auth_typeStr := "token"

	new_config := CloudConfig{
		AuthType:           &auth_typeStr,
		Auth:               new_auth,
		IdentityApiVersion: config.IdentityApiVersion,
		CloudInterface:     config.CloudInterface,
		RegionName:         config.RegionName,
	}

	return new_config
}

// Creates a temporary token based authorisation from a long term config
func get_ephemeral_config(config CloudConfig) CloudConfig {
	fmt.Printf("Authenticating '%s' in project '%s'\n", config.Auth.Username, config.Auth.ProjectName)
	password := prompt_password()
	totp := prompt_totp()

	password_and_totp := fmt.Sprintf("%s%s", password, totp)

	token := get_token(
		config.Auth,
		password_and_totp,
	)

	return create_token_config(config, token)
}

// Creates a long term config from a default config
func get_long_term_config(config CloudConfig) CloudConfig {
	new_auth := AuthConfig{
		AuthUrl:        config.Auth.AuthUrl,
		ProjectId:      config.Auth.ProjectId,
		ProjectName:    config.Auth.ProjectName,
		Username:       config.Auth.Username,
		UserDomainName: config.Auth.UserDomainName,
	}
	new_config := CloudConfig{
		Auth:               new_auth,
		IdentityApiVersion: config.IdentityApiVersion,
		CloudInterface:     config.CloudInterface,
		RegionName:         config.RegionName,
	}
	return new_config
}

func create_ephemeral_config(long_term CloudConfig, os_cloud string, config_path string, config CloudsConfig) {
	fmt.Printf("Creating Ephemeral Config '%s' in '%s'\n", os_cloud, config_path)
	ephemeral_config := get_ephemeral_config(long_term)
	config.Clouds[os_cloud] = ephemeral_config
	write_clouds_yaml(config_path, config)
}

func create_long_term_config(ephemeral CloudConfig, os_cloud string, config_path string, config CloudsConfig) {
	fmt.Printf("Creating Long Term Config '%s' in '%s'\n", os_cloud, config_path)
	long_term_config := get_long_term_config(ephemeral)
	config.Clouds[os_cloud] = long_term_config
	write_clouds_yaml(config_path, config)
}

func main() {

	os_cloud := os.Getenv("OS_CLOUD")
	if os_cloud == "" {
		fmt.Println("$OS_CLOUD not set.")
		os.Exit(1)
	}
	os_cloud_long_term := fmt.Sprintf("%s-long-term", os_cloud)

	config_path, err := find_confg()
	if err != nil {
		os.Exit(1)
	}

	config := read_clouds_yaml(config_path)

	// 1. Check if OS_CLOUD-long-term exist
	if long_term, ok := config.Clouds[os_cloud_long_term]; ok {
		// If so, use it to create OS_CLOUD
		create_ephemeral_config(long_term, os_cloud, config_path, config)
	} else if default_config, ok := config.Clouds[os_cloud]; ok {
		// Otherwise, if OS_CLOUD exits, create OS_CLOUD-long-term
		create_long_term_config(default_config, os_cloud_long_term, config_path, config)
		// Then create OS_CLOUD from that
		create_ephemeral_config(long_term, os_cloud, config_path, config)
	} else {
		fmt.Printf("Could not find '%s' or '%s' in '%s'\n", os_cloud, os_cloud_long_term, config_path)
	}
}
