package gui

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	webview2 "github.com/symp1ex/go-webview2"

	"posrelayd-viewer/internal/logger"
)

const (
	WM_SETICON = 0x0080

	ICON_SMALL = 0
	ICON_BIG   = 1

	SHGFI_ICON      = 0x000000100
	SHGFI_LARGEICON = 0x000000000
	SHGFI_SMALLICON = 0x000000001
)

type shFileInfo struct {
	HIcon         uintptr
	IIcon         int32
	DwAttributes  uint32
	SzDisplayName [260]uint16
	SzTypeName    [80]uint16
}

var (
	shell32Icon        = windows.NewLazySystemDLL("shell32.dll")
	procSHGetFileInfoW = shell32Icon.NewProc("SHGetFileInfoW")
)

func loadExeShellIcon(small bool) (uintptr, error) {
	exePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("os.Executable failed: %w", err)
	}

	exePathPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return 0, fmt.Errorf("UTF16PtrFromString failed: %w", err)
	}

	var info shFileInfo

	flags := uintptr(SHGFI_ICON)
	if small {
		flags |= SHGFI_SMALLICON
	} else {
		flags |= SHGFI_LARGEICON
	}

	ret, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(exePathPtr)),
		0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		flags,
	)

	if ret == 0 || info.HIcon == 0 {
		return 0, fmt.Errorf("SHGetFileInfoW failed: %w", callErr)
	}

	return info.HIcon, nil
}

func setTaskbarIcon(w webview2.WebView) error {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Taskbar icon was not set because window handle is empty")
		return nil
	}

	bigIcon, err := loadExeShellIcon(false)
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to load large exe shell icon: %v", err)
		return err
	}

	smallIcon, err := loadExeShellIcon(true)
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to load small exe shell icon: %v", err)
		return err
	}

	_, _, _ = procSendMessage.Call(hwnd, WM_SETICON, ICON_BIG, bigIcon)
	_, _, _ = procSendMessage.Call(hwnd, WM_SETICON, ICON_SMALL, smallIcon)

	logger.Posrelayv.Debug("[GUI] Taskbar icon set from exe shell icon")
	return nil
}
