package config

import (
	"encoding/json"
	"os"
	"strings"
)

const displayConfigName = "Display.json"

const (
	DefaultDisplayQuality   = "auto"
	DefaultDisplayCodec     = "h264"
	DefaultShowRemoteCursor = true
)

var (
	DefaultDisplayQualityList = []string{
		"auto",
		"ultra",
		"high",
		"medium",
		"low",
	}

	DefaultDisplayCodecList = []string{
		"h264",
		"av1",
		"vp8",
	}
)

type DisplayOption struct {
	Active string   `json:"active"`
	List   []string `json:"list"`
}

type DisplayConfig struct {
	Quality DisplayOption      `json:"Quality"`
	Codec   DisplayOption      `json:"Codec"`
	Other   DisplayOtherConfig `json:"Other"`
}

type DisplayOtherConfig struct {
	ShowRemoteCursor bool `json:"Show_Remote_Cursor"`
}

func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfig{
		Quality: DisplayOption{
			Active: DefaultDisplayQuality,
			List:   DefaultDisplayQualityList,
		},
		Codec: DisplayOption{
			Active: DefaultDisplayCodec,
			List:   DefaultDisplayCodecList,
		},
		Other: DisplayOtherConfig{
			ShowRemoteCursor: DefaultShowRemoteCursor,
		},
	}
}

func LoadDisplayConfig() DisplayConfig {
	cfg := DefaultDisplayConfig()
	path := ConfigPath(displayConfigName)

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
		cfgLogger.Warnf(
			"Display config contains unsupported values, recreating defaults: quality=%q codec=%q",
			cfg.Quality.Active,
			cfg.Codec.Active,
		)
		normalized = DefaultDisplayConfig()
		_ = SaveDisplayConfig(normalized)
		return normalized
	}

	if !displayConfigEqual(normalized, cfg) {
		_ = SaveDisplayConfig(normalized)
	}

	return normalized
}

func SaveDisplayConfig(cfg DisplayConfig) error {
	data, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath(displayConfigName), data, 0644)
}

func NormalizeDisplayConfig(cfg DisplayConfig) (DisplayConfig, bool) {
	cfg.Quality.Active = strings.ToLower(strings.TrimSpace(cfg.Quality.Active))
	cfg.Codec.Active = strings.ToLower(strings.TrimSpace(cfg.Codec.Active))

	if cfg.Quality.Active == "" {
		cfg.Quality.Active = DefaultDisplayQuality
	}
	if cfg.Codec.Active == "" {
		cfg.Codec.Active = DefaultDisplayCodec
	}

	switch cfg.Quality.Active {
	case "auto", "low", "medium", "high", "ultra":
	default:
		return DisplayConfig{}, false
	}

	switch cfg.Codec.Active {
	case "auto", "vp8", "h264", "av1":
	default:
		return DisplayConfig{}, false
	}

	// list пока не обрабатываем как пользовательскую настройку,
	// а всегда приводим к дефолтному виду.
	cfg.Quality.List = DefaultDisplayQualityList
	cfg.Codec.List = DefaultDisplayCodecList

	return cfg, true
}

func displayConfigEqual(a, b DisplayConfig) bool {
	return a.Quality.Active == b.Quality.Active &&
		a.Codec.Active == b.Codec.Active &&
		a.Other.ShowRemoteCursor == b.Other.ShowRemoteCursor &&
		stringSlicesEqual(a.Quality.List, b.Quality.List) &&
		stringSlicesEqual(a.Codec.List, b.Codec.List)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
