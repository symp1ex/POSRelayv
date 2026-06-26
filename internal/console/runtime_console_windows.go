//go:build windows

package console

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"posrelayd-viewer/internal/logger"
)

var runtimeConsoleOnce sync.Once
var runtimeConsoleErr error

func EnsureRuntimeConsole() error {
	logger.Posrelayv.Debug("[console] Ensuring runtime console")

	runtimeConsoleOnce.Do(func() {
		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		allocConsole := kernel32.NewProc("AllocConsole")

		ret, _, err := allocConsole.Call()
		if ret == 0 {
			if err != syscall.ERROR_ACCESS_DENIED {
				runtimeConsoleErr = fmt.Errorf("AllocConsole failed: %w", err)
				logger.Posrelayv.Errorf("[console] Failed to allocate runtime console: %v", err)
				return
			}

			logger.Posrelayv.Debug("[console] Runtime console already exists")
		} else {
			logger.Posrelayv.Info("[console] Runtime console allocated")
		}

		stdin, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			runtimeConsoleErr = fmt.Errorf("open CONIN$ failed: %w", err)
			logger.Posrelayv.Errorf("[console] Failed to open CONIN$: %v", err)
			return
		}

		stdout, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			runtimeConsoleErr = fmt.Errorf("open CONOUT$ failed: %w", err)
			logger.Posrelayv.Errorf("[console] Failed to open CONOUT$: %v", err)
			return
		}

		stderr, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			runtimeConsoleErr = fmt.Errorf("open CONOUT$ for stderr failed: %w", err)
			logger.Posrelayv.Errorf("[console] Failed to open CONOUT$ for stderr: %v", err)
			return
		}

		os.Stdin = stdin
		os.Stdout = stdout
		os.Stderr = stderr

		logger.Posrelayv.Debug("[console] Standard streams redirected to runtime console")
	})

	if runtimeConsoleErr != nil {
		logger.Posrelayv.Errorf("[console] Runtime console is unavailable: %v", runtimeConsoleErr)
		return runtimeConsoleErr
	}

	logger.Posrelayv.Debug("[console] Runtime console is ready")
	return nil
}
