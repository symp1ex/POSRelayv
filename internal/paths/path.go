package paths

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

	workDir      string
	configsDir   string
	notebooksDir string
)

func Init() error {
	initOnce.Do(func() {
		workDir, initErr = resolveWorkDir()
		if initErr != nil {
			return
		}

		configsDir = filepath.Join(workDir, "configs")
		notebooksDir = filepath.Join(workDir, "notebooks")

		for _, dir := range []string{workDir, configsDir, notebooksDir} {
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

func NotebooksDir() string {
	mustInit()
	return notebooksDir
}

func ConfigPath(name string) string {
	return filepath.Join(ConfigsDir(), name)
}

func NotebookPath(name string) string {
	return filepath.Join(NotebooksDir(), name)
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
