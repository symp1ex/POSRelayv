package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	portableDirName = "userdata"
	appDataDirName  = "posrelayv"
)

var (
	initOnce sync.Once
	initErr  error

	workDir    string
	configsDir string
	storageDir string
	cacheDir   string
)

func Init() error {
	initOnce.Do(func() {
		workDir, initErr = resolveWorkDir()
		if initErr != nil {
			return
		}

		configsDir = filepath.Join(workDir, "configs")
		storageDir = filepath.Join(workDir, "storage")
		cacheDir = filepath.Join(workDir, "cache")

		for _, dir := range []string{workDir, configsDir, storageDir, cacheDir} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				initErr = fmt.Errorf("create directory %s: %w", dir, err)
				return
			}
		}
	})

	return initErr
}

func WorkDir() string {
	mustInit()
	return workDir
}

func ConfigsDir() string {
	mustInit()
	return configsDir
}

func StorageDir() string {
	mustInit()
	return storageDir
}

func CacheDir() string {
	mustInit()
	return cacheDir
}

func ConfigPath(name string) string {
	return filepath.Join(ConfigsDir(), name)
}

func StoragePath(name string) string {
	return filepath.Join(StorageDir(), name)
}

func resolveWorkDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}

	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlinks: %w", err)
	}

	exeDir := filepath.Dir(exePath)

	portableDir := filepath.Join(exeDir, portableDirName)
	if info, err := os.Stat(portableDir); err == nil && info.IsDir() {
		return portableDir, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("check portable directory %s: %w", portableDir, err)
	}

	appDataDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(appDataDir, appDataDirName), nil
}

func mustInit() {
	if err := Init(); err != nil {
		panic(err)
	}
}
