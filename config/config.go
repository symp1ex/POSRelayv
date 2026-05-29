package config

import (
	"encoding/json"
	"log"
	"os"
)

const configPath = "config.json"

var Cfg Config

type Config struct {
	Connection ConnectionConfig `json:"connection""`
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

func defaultConfig() Config {
	return Config{
		Connection: ConnectionConfig{
			Url:    "-",
			APIKey: "-",
		},
		Logs: LogsConfig{
			LogLevel:  "error",
			StoreDays: 2,
		},
	}
}
