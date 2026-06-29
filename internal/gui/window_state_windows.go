//go:build windows

package gui

import (
	"encoding/json"
	"math"
	"os"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"

	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/logger"
)

const windowStateFileName = "window-state.json"

type windowStateFile struct {
	Window windowStatePosition `json:"window"`
}

type windowStatePosition struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

func loadMainWindowPosition() (int32, int32, bool) {
	path := config.CachePath(windowStateFileName)

	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Posrelayv.Warnf("[GUI] Failed to read window state: %v", err)
		}
		return 0, 0, false
	}

	var state windowStateFile
	if err := json.Unmarshal(raw, &state); err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to parse window state: %v", err)
		return 0, 0, false
	}

	if state.Window.X == nil || state.Window.Y == nil {
		logger.Posrelayv.Warn("[GUI] Window state does not contain coordinates")
		return 0, 0, false
	}

	if *state.Window.X < math.MinInt32 ||
		*state.Window.X > math.MaxInt32 ||
		*state.Window.Y < math.MinInt32 ||
		*state.Window.Y > math.MaxInt32 {
		logger.Posrelayv.Warnf(
			"[GUI] Window state coordinates are out of int32 range: x=%d y=%d",
			*state.Window.X,
			*state.Window.Y,
		)
		return 0, 0, false
	}

	return int32(*state.Window.X), int32(*state.Window.Y), true
}

func restoreMainWindowPosition(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Main window position restore skipped because hwnd is empty")
		return
	}

	x, y, ok := loadMainWindowPosition()
	if !ok {
		logger.Posrelayv.Debug("[GUI] Main window position restore skipped because saved state is unavailable")
		return
	}

	_, _, _ = procSetWindowPos.Call(
		hwnd,
		0,
		uintptr(int(x)),
		uintptr(int(y)),
		0,
		0,
		swpNoSize|swpNoZOrder|swpNoActivate,
	)

	logger.Posrelayv.Debugf("[GUI] Main window position restored: x=%d y=%d", x, y)
}

func saveMainWindowPosition(hwnd uintptr) {
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Main window position save skipped because hwnd is empty")
		return
	}

	var rect winRect

	ret, _, callErr := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		logger.Posrelayv.Warnf("[GUI] Failed to get main window rect for state save: %v", callErr)
		return
	}

	x := int(rect.Left)
	y := int(rect.Top)

	state := windowStateFile{
		Window: windowStatePosition{
			X: &x,
			Y: &y,
		},
	}

	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to marshal window state: %v", err)
		return
	}

	raw = append(raw, '\n')

	if err := os.WriteFile(config.CachePath(windowStateFileName), raw, 0644); err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to write window state: %v", err)
		return
	}

	logger.Posrelayv.Debugf("[GUI] Main window position saved: x=%d y=%d", x, y)
}
