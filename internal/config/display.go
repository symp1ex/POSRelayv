package config

import (
	"encoding/json"
	"os"
	"strings"
)

const displayConfigName = "Display.json"

const (
	DefaultDisplayQuality         = "Medium"
	DefaultDisplayCodec           = "H264"
	DefaultEnableHardwareEncoding = true
	DefaultForceKeyframeOnPLI     = false
	DefaultShowRemoteCursor       = false
	DefaultStretch_Video          = false
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
		"H264",
		"AV1",
		"VP8",
	}
)

type DisplayOption struct {
	Active string   `json:"active"`
	List   []string `json:"list"`
}

type DisplayVideoStreamConfig struct {
	Quality                DisplayOption `json:"Quality"`
	Codec                  DisplayOption `json:"Codec"`
	EnableHardwareEncoding bool          `json:"Enable_Hardware_Encoding"`
	ForceKeyframeOnPLI     bool          `json:"Force_keyframe_on_PLI_(beta)"`
}
type DisplayConfig struct {
	VideoStream DisplayVideoStreamConfig `json:"Video_Stream"`
	Other       DisplayOtherConfig       `json:"Other"`
}

type DisplayOtherConfig struct {
	Stretch_Video    bool `json:"Stretch_Video"`
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
			EnableHardwareEncoding: DefaultEnableHardwareEncoding,
			ForceKeyframeOnPLI:     DefaultForceKeyframeOnPLI,
		},
		Other: DisplayOtherConfig{
			Stretch_Video:    DefaultStretch_Video,
			ShowRemoteCursor: DefaultShowRemoteCursor,
		},
	}
}

func LoadDisplayConfig() DisplayConfig {
	cfg := DefaultDisplayConfig()
	path := ConfigPath(displayConfigName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfgLogger.Warnf("Display config does not exist, creating defaults: %v", err)
		} else {
			cfgLogger.Warnf("Display config is unavailable, recreating defaults: %v", err)
		}

		_ = SaveDisplayConfig(cfg)
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		cfgLogger.Warnf("Display config is unreadable, recreating defaults: %v", err)

		cfg = DefaultDisplayConfig()
		_ = SaveDisplayConfig(cfg)

		return cfg
	}

	normalized, ok := NormalizeDisplayConfig(cfg)
	if !ok {
		cfgLogger.Warnf(
			"Display config contains unsupported values, using runtime defaults without rewriting file: quality=%q codec=%q",
			cfg.VideoStream.Quality.Active,
			cfg.VideoStream.Codec.Active,
		)

		return DefaultDisplayConfig()
	}

	if !displayConfigHasStretch(data) {
		_ = SaveDisplayConfig(normalized)
	}

	return normalized
}

func displayConfigHasStretch(data []byte) bool {
	var raw struct {
		Other map[string]any `json:"Other"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}

	if raw.Other == nil {
		return false
	}

	_, ok := raw.Other["Stretch_Video"]
	return ok
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
	quality := strings.ToLower(strings.TrimSpace(cfg.VideoStream.Quality.Active))
	codec := strings.ToLower(strings.TrimSpace(cfg.VideoStream.Codec.Active))

	if quality == "" {
		quality = strings.ToLower(DefaultDisplayQuality)
	}

	if codec == "" {
		codec = strings.ToLower(DefaultDisplayCodec)
	}

	switch quality {
	case "auto", "low", "medium", "high", "ultra":
	default:
		return DisplayConfig{}, false
	}

	switch codec {
	case "vp8", "h264", "av1":
	default:
		return DisplayConfig{}, false
	}

	cfg.VideoStream.Quality.Active = quality
	cfg.VideoStream.Codec.Active = codec
	cfg.VideoStream.Quality.List = DefaultDisplayQualityList
	cfg.VideoStream.Codec.List = DefaultDisplayCodecList

	return cfg, true
}

func EffectiveDisplayCodec(cfg DisplayConfig) string {
	if !cfg.VideoStream.EnableHardwareEncoding {
		return "vp8"
	}

	codec := strings.ToLower(strings.TrimSpace(cfg.VideoStream.Codec.Active))
	if codec == "" {
		return strings.ToLower(DefaultDisplayCodec)
	}

	return codec
}
