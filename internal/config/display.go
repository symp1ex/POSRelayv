package config

import (
	"encoding/json"
	"os"
	"posrelayd-viewer/internal/paths"
	"strings"
)

const displayConfigName = "display.json"

const (
	DefaultDisplayQuality = "auto"
	DefaultDisplayCodec   = "h264"
)

type DisplayConfig struct {
	Quality string `json:"quality"`
	Codec   string `json:"codec"`
}

func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfig{
		Quality: DefaultDisplayQuality,
		Codec:   DefaultDisplayCodec,
	}
}

func LoadDisplayConfig() DisplayConfig {
	cfg := DefaultDisplayConfig()
	path := paths.ConfigPath(displayConfigName)

	data, err := os.ReadFile(path)
	if err != nil {
		cfgLogger.Warnf("Display config is unavailable, recreating defaults: %v", err)
		_ = SaveDisplayConfig(cfg)
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		cfgLogger.Warnf("Display config is invalid, recreating defaults: %v", err)
		cfg = DefaultDisplayConfig()
		_ = SaveDisplayConfig(cfg)
		return cfg
	}

	normalized, ok := NormalizeDisplayConfig(cfg)
	if !ok {
		cfgLogger.Warnf("Display config contains unsupported values, recreating defaults: quality=%q codec=%q", cfg.Quality, cfg.Codec)
		normalized = DefaultDisplayConfig()
		_ = SaveDisplayConfig(normalized)
		return normalized
	}

	if normalized != cfg {
		_ = SaveDisplayConfig(normalized)
	}

	return normalized
}

func SaveDisplayConfig(cfg DisplayConfig) error {
	data, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(paths.ConfigPath(displayConfigName), data, 0644)
}

func NormalizeDisplayConfig(cfg DisplayConfig) (DisplayConfig, bool) {
	cfg.Quality = strings.ToLower(strings.TrimSpace(cfg.Quality))
	cfg.Codec = strings.ToLower(strings.TrimSpace(cfg.Codec))

	if cfg.Quality == "" {
		cfg.Quality = DefaultDisplayQuality
	}
	if cfg.Codec == "" {
		cfg.Codec = DefaultDisplayCodec
	}

	switch cfg.Quality {
	case "auto", "low", "medium", "high", "ultra":
	default:
		return DisplayConfig{}, false
	}

	switch cfg.Codec {
	case "auto", "vp8", "h264", "av1":
	default:
		return DisplayConfig{}, false
	}

	return cfg, true
}
