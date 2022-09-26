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
	AuthType           *string     `yaml:"auth_type,omitempty"`
	Auth               AuthOptions `yaml:"auth,omitempty"`
	RegionName         string      `yaml:"region_name,omitempty"`
	CloudInterface     string      `yaml:"interface,omitempty"`
	IdentityApiVersion int         `yaml:"identity_api_version,omitempty""`
}

type AuthOptions struct {
	AuthUrl           string  `yaml:"auth_url,omitempty"`
	Username          string  `yaml:"username,omitempty"`
	ProjectId         string  `yaml:"project_id,omitempty"`
	ProjectName       string  `yaml:"project_name,omitempty"`
	UserDomainName    *string `yaml:"user_domain_name,omitempty"`
	ProjectDomainName *string `yaml:"project_domain_name,omitempty"`
	Token             *string `yaml:"token,omitempty"`
	Password          *string `yaml:"password,omitempty"`
}

func get_token(auth AuthOptions, password_totp string) string {

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

	jsonData, _ := json.Marshal(data)

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
	bytes, _ := yaml.Marshal(config)
	os.WriteFile(path, bytes, 644)
}

func prompt_password() string {
	fmt.Printf("Enter Password: ")
	bytePassword, _ := term.ReadPassword(int(syscall.Stdin))
	fmt.Println("")
	return string(bytePassword)
}

func prompt_totp() string {

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("TOTP (press enter to skip): ")
	totp, _ := reader.ReadString('\n')
	totp = strings.TrimRight(totp, "\n")
	totp = strings.TrimRight(totp, " ")
	return totp
}

func create_token_config(config CloudConfig, token string) CloudConfig {

	new_auth := AuthOptions{
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
	token := get_token(
		config.Auth,
		fmt.Sprintf("%s%s", password, totp),
	)
	fmt.Println(token)
	return create_token_config(config, token)
}

// Creates a long term config from a default config
func get_long_term_config(config CloudConfig) CloudConfig {
	new_auth := AuthOptions{
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
		ephemeral_config := get_ephemeral_config(long_term)
		config.Clouds[os_cloud] = ephemeral_config
		write_clouds_yaml(config_path, config)
		// 2. Create OS_CLOUD_LONG_TERM from OS_CLOUD
	} else if default_config, ok := config.Clouds[os_cloud]; ok {
		long_term_config := get_long_term_config(default_config)
		config.Clouds[os_cloud_long_term] = long_term_config
		write_clouds_yaml(config_path, config)

		// 3. Create OS_CLOUD from OS_CLOUD-long-term
		config := read_clouds_yaml(config_path)
		long_term := config.Clouds[os_cloud_long_term]
		ephemeral_config := get_ephemeral_config(long_term)
		config.Clouds[os_cloud] = ephemeral_config
		write_clouds_yaml(config_path, config)
	} else {

	}
}
