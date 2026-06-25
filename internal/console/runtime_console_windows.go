//go:build windows

package console

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

var runtimeConsoleOnce sync.Once
var runtimeConsoleErr error

func EnsureRuntimeConsole() error {
	runtimeConsoleOnce.Do(func() {
		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		allocConsole := kernel32.NewProc("AllocConsole")

		ret, _, err := allocConsole.Call()
		if ret == 0 {
			if err != syscall.ERROR_ACCESS_DENIED {
				runtimeConsoleErr = fmt.Errorf("AllocConsole failed: %w", err)
				return
			}
		}

		stdin, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			runtimeConsoleErr = fmt.Errorf("open CONIN$ failed: %w", err)
			return
		}

		stdout, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			runtimeConsoleErr = fmt.Errorf("open CONOUT$ failed: %w", err)
			return
		}

		stderr, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			runtimeConsoleErr = fmt.Errorf("open CONOUT$ for stderr failed: %w", err)
			return
		}

		os.Stdin = stdin
		os.Stdout = stdout
		os.Stderr = stderr
	})

	return runtimeConsoleErr
}
