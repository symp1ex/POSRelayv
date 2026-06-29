package config

import (
	"encoding/json"
	"os"
	"strings"
)

const displayConfigName = "Display.json"

const (
	DefaultDisplayQuality   = "Auto"
	DefaultDisplayCodec     = "h264"
	DefaultShowRemoteCursor = true
)

var (
	DefaultDisplayQualityList = []string{
		"Auto",
		"Ultra",
		"High",
		"Medium",
		"Low",
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

type DisplayVideoStreamConfig struct {
	Quality DisplayOption `json:"Quality"`
	Codec   DisplayOption `json:"Codec"`
}

type DisplayConfig struct {
	VideoStream DisplayVideoStreamConfig `json:"Video_Stream"`
	Other       DisplayOtherConfig       `json:"Other"`
}

type DisplayOtherConfig struct {
	ShowRemoteCursor bool `json:"Show_Remote_Cursor"`
}

func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfig{
		VideoStream: DisplayVideoStreamConfig{
			Quality: DisplayOption{
				Active: DefaultDisplayQuality,
				List:   DefaultDisplayQualityList,
			},
			Codec: DisplayOption{
				Active: DefaultDisplayCodec,
				List:   DefaultDisplayCodecList,
			},
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
			cfg.VideoStream.Quality.Active,
			cfg.VideoStream.Codec.Active,
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

func EnsureDisplayConfig() {
	_ = LoadDisplayConfig()
}

func NormalizeDisplayConfig(cfg DisplayConfig) (DisplayConfig, bool) {
	cfg.VideoStream.Quality.Active = strings.ToLower(strings.TrimSpace(cfg.VideoStream.Quality.Active))
	cfg.VideoStream.Codec.Active = strings.ToLower(strings.TrimSpace(cfg.VideoStream.Codec.Active))

	if cfg.VideoStream.Quality.Active == "" {
		cfg.VideoStream.Quality.Active = DefaultDisplayQuality
	}
	if cfg.VideoStream.Codec.Active == "" {
		cfg.VideoStream.Codec.Active = DefaultDisplayCodec
	}

	switch cfg.VideoStream.Quality.Active {
	case "auto", "low", "medium", "high", "ultra":
	default:
		return DisplayConfig{}, false
	}

	switch cfg.VideoStream.Codec.Active {
	case "auto", "vp8", "h264", "av1":
	default:
		return DisplayConfig{}, false
	}

	cfg.VideoStream.Quality.List = DefaultDisplayQualityList
	cfg.VideoStream.Codec.List = DefaultDisplayCodecList

	return cfg, true
}

func displayConfigEqual(a, b DisplayConfig) bool {
	return a.VideoStream.Quality.Active == b.VideoStream.Quality.Active &&
		a.VideoStream.Codec.Active == b.VideoStream.Codec.Active &&
		a.Other.ShowRemoteCursor == b.Other.ShowRemoteCursor &&
		stringSlicesEqual(a.VideoStream.Quality.List, b.VideoStream.Quality.List) &&
		stringSlicesEqual(a.VideoStream.Codec.List, b.VideoStream.Codec.List)
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
