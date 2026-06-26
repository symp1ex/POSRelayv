package gui

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	webview2 "github.com/jchv/go-webview2"

	"posrelayd-viewer/internal/logger"
)

const (
	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040
	WM_SETICON      = 0x0080
)

// Global variables for accessing user32.dll functions.
var (
	user32        = windows.NewLazySystemDLL("user32.dll")
	procLoadImage = user32.NewProc("LoadImageW")
	// Note: procSendMessage is declared in window_chrome_windows.go and reused here.
)

func setTaskbarIcon(w webview2.WebView) error {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Taskbar icon was not set because window handle is empty")
		return nil
	}

	iconPathPtr, err := syscall.UTF16PtrFromString(`ui\rd-web\src\assets\main.ico`)
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to convert taskbar icon path: %v", err)
		return fmt.Errorf("failed to convert icon path: %w", err)
	}

	hIcon, _, _ := procLoadImage.Call(
		0,
		uintptr(unsafe.Pointer(iconPathPtr)),
		IMAGE_ICON,
		0, 0,
		LR_LOADFROMFILE|LR_DEFAULTSIZE,
	)
	if hIcon == 0 {
		logger.Posrelayv.Warnf("[GUI] Failed to load taskbar icon")
		return fmt.Errorf("LoadImage failed")
	}

	_, _, err = procSendMessage.Call(hwnd, WM_SETICON, uintptr(1), hIcon)
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to set taskbar icon: %v", err)
		return fmt.Errorf("SendMessage failed: %w", err)
	}

	logger.Posrelayv.Debug("[GUI] Taskbar icon set")
	return nil
}
