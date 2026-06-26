package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"posrelayd-viewer/internal/crypto"
	"strings"
)

const configPath = "config.json"

var Cfg Config

type Config struct {
	Connection ConnectionConfig `json:"connection"`
	Logs       LogsConfig       `json:"logs"`
}

type ConnectionConfig struct {
	Url    string `json:"url"`
	APIKey string `json:"api_key"`
}

type LogsConfig struct {
	LogLevel  string `json:"log_level"`
	StoreDays int    `json:"store_days"`
}

func init() {
	cfg, err := load()
	if err != nil {
		log.Println("[config]", err)
	}
	Cfg = cfg
}

func load() (Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(configPath)
	if err != nil {
		_ = save(cfg)
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		_ = save(cfg)
		return cfg, err
	}

	return cfg, nil
}

func save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func Setup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter the server address: ")
	url, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	url = strings.TrimSpace(url)

	fmt.Print("Enter the API-key: ")
	apiKey, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	apiKey = strings.TrimSpace(apiKey)

	encURL, err := crypto.Encrypt(url)
	if err != nil {
		return err
	}

	encKey, err := crypto.Encrypt(apiKey)
	if err != nil {
		return err
	}

	Cfg.Connection.Url = encURL
	Cfg.Connection.APIKey = encKey

	return save(Cfg)
}

func defaultConfig() Config {
	return Config{
		Connection: ConnectionConfig{
			Url:    "-",
			APIKey: "-",
		},
		Logs: LogsConfig{
			LogLevel:  "warning",
			StoreDays: 2,
		},
	}
}
