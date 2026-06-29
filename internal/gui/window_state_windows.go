//go:build windows

package gui

import (
	"encoding/json"
	"os"
	"unsafe"

	webview2 "github.com/symp1ex/go-webview2"

	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/logger"
)

const (
	windowStateFileName = "window-state.json"

	minWindowCoordinate = -1 << 31
	maxWindowCoordinate = 1<<31 - 1
)

type windowStateFile struct {
	Window windowStatePosition `json:"window"`
}

type windowStatePosition struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

func mainWindowOptions() webview2.WindowOptions {
	options := webview2.WindowOptions{
		Title:  "POSRelayv",
		Width:  985,
		Height: 760,
		Center: false,
	}

	x, y, ok := loadMainWindowPosition()
	if !ok {
		logger.Posrelayv.Debug("[GUI] Main window saved position is unavailable, using default window options")
		return options
	}

	posX := int(x)
	posY := int(y)

	options.X = &posX
	options.Y = &posY
	options.Center = false

	logger.Posrelayv.Debugf("[GUI] Main window position loaded for native creation: x=%d y=%d", x, y)

	return options
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

	if !isValidWindowCoordinate(*state.Window.X) || !isValidWindowCoordinate(*state.Window.Y) {
		logger.Posrelayv.Warnf(
			"[GUI] Window state coordinates are out of int32 range: x=%d y=%d",
			*state.Window.X,
			*state.Window.Y,
		)
		return 0, 0, false
	}

	return int32(*state.Window.X), int32(*state.Window.Y), true
}

func isValidWindowCoordinate(value int) bool {
	return value >= minWindowCoordinate && value <= maxWindowCoordinate
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
