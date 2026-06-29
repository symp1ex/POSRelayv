package config

import (
	"encoding/json"
	"fmt"
	"github.com/iancoleman/orderedmap"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SettingsConfigFile struct {
	Name string                 `json:"name"`
	Data *orderedmap.OrderedMap `json:"data"`
}

func LoadSettingsConfigs() ([]SettingsConfigFile, error) {
	dir := ConfigsDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read configs dir: %w", err)
	}

	configs := make([]SettingsConfigFile, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if strings.ToLower(filepath.Ext(fileName)) != ".json" {
			continue
		}

		path := filepath.Join(dir, fileName)

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", fileName, err)
		}

		data := orderedmap.New()
		if err := json.Unmarshal(raw, data); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", fileName, err)
		}

		configs = append(configs, SettingsConfigFile{
			Name: strings.TrimSuffix(fileName, filepath.Ext(fileName)),
			Data: data,
		})
	}

	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	return configs, nil
}

func SaveSettingsConfig(name string, data map[string]any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("config name is empty")
	}

	if strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
		return fmt.Errorf("invalid config name")
	}

	path := filepath.Join(ConfigsDir(), name+".json")

	cleanBase := filepath.Clean(ConfigsDir())
	cleanPath := filepath.Clean(path)

	if filepath.Dir(cleanPath) != cleanBase {
		return fmt.Errorf("invalid config path")
	}

	orderedData, err := loadOrderedConfigForSave(cleanPath)
	if err != nil {
		return err
	}

	mergeSettingsIntoOrderedMap(orderedData, data)

	raw, err := json.MarshalIndent(orderedData, "", "\t")
	if err != nil {
		return fmt.Errorf("marshal config %s: %w", name, err)
	}

	raw = append(raw, '\n')

	return os.WriteFile(cleanPath, raw, 0644)
}

func loadOrderedConfigForSave(path string) (*orderedmap.OrderedMap, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config for ordered save: %w", err)
	}

	orderedData := orderedmap.New()
	if err := json.Unmarshal(raw, orderedData); err != nil {
		return nil, fmt.Errorf("parse config for ordered save: %w", err)
	}

	return orderedData, nil
}

func mergeSettingsIntoOrderedMap(target *orderedmap.OrderedMap, source map[string]any) {
	for _, key := range target.Keys() {
		sourceValue, ok := source[key]
		if !ok {
			continue
		}

		targetValue, _ := target.Get(key)

		targetNested, targetIsNested := targetValue.(*orderedmap.OrderedMap)
		sourceNested, sourceIsNested := sourceValue.(map[string]any)

		if targetIsNested && sourceIsNested {
			mergeSettingsIntoOrderedMap(targetNested, sourceNested)
			continue
		}

		target.Set(key, sourceValue)
	}

	appendMissingSettingsKeys(target, source)
}

func appendMissingSettingsKeys(target *orderedmap.OrderedMap, source map[string]any) {
	missingKeys := make([]string, 0)

	for key := range source {
		if _, ok := target.Get(key); ok {
			continue
		}

		missingKeys = append(missingKeys, key)
	}

	sort.Strings(missingKeys)

	for _, key := range missingKeys {
		target.Set(key, source[key])
	}
}
