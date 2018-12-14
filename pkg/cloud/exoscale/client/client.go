package client

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path"

	"github.com/exoscale/egoscale"
	"github.com/spf13/viper"
)

//Client Exoscale client
var Client *egoscale.Client

type config struct {
	DefaultAccount string
	Accounts       []account
}

type account struct {
	Name            string
	Account         string
	Endpoint        string
	ComputeEndpoint string // legacy config.
	DNSEndpoint     string
	SosEndpoint     string
	Key             string
	Secret          string
	SecretCommand   []string
	DefaultZone     string
	DefaultSSHKey   string
	DefaultTemplate string
}

func init() {
	c, err := getConfig()
	if err != nil {
		log.Fatal(err)
	}
	Client = c
}

func getConfig() (*egoscale.Client, error) {
	// an attempt to mimic existing behaviours
	envEndpoint := readFromEnv(
		"EXOSCALE_ENDPOINT",
		"EXOSCALE_COMPUTE_ENDPOINT",
		"CLOUDSTACK_ENDPOINT")

	envKey := readFromEnv(
		"EXOSCALE_KEY",
		"EXOSCALE_API_KEY",
		"CLOUDSTACK_KEY",
		"CLOUSTACK_API_KEY",
	)

	envSecret := readFromEnv(
		"EXOSCALE_SECRET",
		"EXOSCALE_API_SECRET",
		"EXOSCALE_SECRET_KEY",
		"CLOUDSTACK_SECRET",
		"CLOUDSTACK_SECRET_KEY",
	)

	if envEndpoint != "" && envKey != "" && envSecret != "" {
		return egoscale.NewClient(envEndpoint, envKey, envSecret), nil
	}

	config := &config{}

	usr, err := user.Current()
	if err != nil {
		log.Println(`current user cannot be read, using "root"`)
		usr = &user.User{
			Uid:      "0",
			Gid:      "0",
			Username: "root",
			Name:     "root",
			HomeDir:  "/root",
		}
	}

	var configFolder string
	xdgHome, found := os.LookupEnv("XDG_CONFIG_HOME")
	if found {
		configFolder = path.Join(xdgHome, "exoscale")
	} else {
		// The XDG spec specifies a default XDG_CONFIG_HOME in $HOME/.config
		configFolder = path.Join(usr.HomeDir, ".config", "exoscale")
	}

	viper.SetConfigName("exoscale")
	viper.AddConfigPath(configFolder)
	// Retain backwards compatibility
	viper.AddConfigPath(path.Join(usr.HomeDir, ".exoscale"))
	viper.AddConfigPath(usr.HomeDir)
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("couldn't read config: %v", err)
	}

	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts were found into %q", viper.ConfigFileUsed())
	}

	if config.DefaultAccount == "" {
		return nil, fmt.Errorf("default account not defined")
	}

	for _, acc := range config.Accounts {
		if acc.Name == config.DefaultAccount {
			return egoscale.NewClient(acc.ComputeEndpoint, acc.Key, acc.Secret), nil
		}
	}

	return nil, fmt.Errorf("could't find any account with name: %q", config.Accounts)
}

// readFromEnv is a os.Getenv on steroids
func readFromEnv(keys ...string) string {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
	}
	return ""
}
