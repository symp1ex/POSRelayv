//go:build windows

package gui

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	webview2 "github.com/jchv/go-webview2"

	"posrelayd-viewer/internal/logger"
)

const (
	mainWindowWidth  = 1272
	mainWindowHeight = 870

	mainWindowMinWidth  = mainWindowWidth / 2
	mainWindowMinHeight = mainWindowHeight / 2

	gwlStyle    = ^uintptr(15) // -16
	gwlpWndProc = ^uintptr(3)  // -4

	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsMinimizeBox  = 0x00020000
	wsMaximizeBox  = 0x00010000
	wsThickFrame   = 0x00040000
	wsBorder       = 0x00800000
	wsDlgFrame     = 0x00400000
	wsPopup        = 0x80000000
	wsVisible      = 0x10000000
	wsClipSiblings = 0x04000000
	wsClipChildren = 0x02000000

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	wmGetMinMaxInfo = 0x0024
	wmNcHitTest     = 0x0084
	wmNcDestroy     = 0x0082
	wmClose         = 0x0010
	wmNcLButtonDown = 0x00A1

	swHide     = 0
	swShow     = 5
	swMinimize = 6

	htClient      = 1
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
)

type winPoint struct {
	X int32
	Y int32
}

type minMaxInfo struct {
	Reserved     winPoint
	MaxSize      winPoint
	MaxPosition  winPoint
	MinTrackSize winPoint
	MaxTrackSize winPoint
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

var (
	user32Chrome = windows.NewLazySystemDLL("user32.dll")

	procGetWindowLongPtr = user32Chrome.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32Chrome.NewProc("SetWindowLongPtrW")
	procSetWindowPos     = user32Chrome.NewProc("SetWindowPos")
	procCallWindowProc   = user32Chrome.NewProc("CallWindowProcW")
	procGetWindowRect    = user32Chrome.NewProc("GetWindowRect")
	procShowWindow       = user32Chrome.NewProc("ShowWindow")
	procPostMessage      = user32Chrome.NewProc("PostMessageW")
	procReleaseCapture   = user32Chrome.NewProc("ReleaseCapture")
	procSendMessage      = user32Chrome.NewProc("SendMessageW")

	chromeOnce        sync.Once
	chromeWndProc     uintptr
	oldWindowProcByID sync.Map
)

func ApplyMainWindowChrome(w webview2.WebView) error {
	chromeOnce.Do(func() {
		chromeWndProc = syscall.NewCallback(mainWindowProc)
	})

	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Errorf("[GUI] Main window chrome cannot be applied because hwnd is empty")
		return fmt.Errorf("main window hwnd is empty")
	}

	logger.Posrelayv.Debug("[GUI] Applying main window chrome")

	oldProc, _, _ := procGetWindowLongPtr.Call(hwnd, gwlpWndProc)
	if oldProc != 0 {
		oldWindowProcByID.Store(hwnd, oldProc)
		_, _, _ = procSetWindowLongPtr.Call(hwnd, gwlpWndProc, chromeWndProc)
	}

	style, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)

	// Делаем настоящее borderless-окно:
	// убираем caption, системное меню, системную рамку и thick frame.
	style &^= wsCaption | wsSysMenu | wsMinimizeBox | wsMaximizeBox | wsThickFrame | wsBorder | wsDlgFrame

	// Оставляем popup/visible/clip styles, чтобы окно было нашим собственным top-level окном.
	style |= wsPopup | wsVisible | wsClipSiblings | wsClipChildren

	_, _, _ = procSetWindowLongPtr.Call(hwnd, gwlStyle, style)

	_, _, _ = procSetWindowPos.Call(
		hwnd,
		0,
		0,
		0,
		0,
		0,
		swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged,
	)

	logger.Posrelayv.Debug("[GUI] Main window chrome applied")
	return nil
}

func MinimizeMainWindow(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Main window minimize skipped because hwnd is empty")
		return
	}

	logger.Posrelayv.Debug("[GUI] Minimizing main window")
	_, _, _ = procShowWindow.Call(hwnd, swMinimize)
}

func CloseMainWindow(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Main window close skipped because hwnd is empty")
		return
	}

	logger.Posrelayv.Debug("[GUI] Posting main window close message")
	_, _, _ = procPostMessage.Call(hwnd, wmClose, 0, 0)
}

func HideWebViewWindow(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] WebView hide skipped because hwnd is empty")
		return
	}

	logger.Posrelayv.Debug("[GUI] Hiding WebView window")
	_, _, _ = procShowWindow.Call(hwnd, swHide)
}

func ShowWebViewWindow(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] WebView show skipped because hwnd is empty")
		return
	}

	logger.Posrelayv.Debug("[GUI] Showing WebView window")
	_, _, _ = procShowWindow.Call(hwnd, swShow)
}

func DragMainWindow(w webview2.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		logger.Posrelayv.Debug("[GUI] Main window drag skipped because hwnd is empty")
		return
	}

	_, _, _ = procReleaseCapture.Call()
	_, _, _ = procSendMessage.Call(hwnd, wmNcLButtonDown, htCaption, 0)
}

func mainWindowProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	switch msg {
	case wmGetMinMaxInfo:
		info := (*minMaxInfo)(unsafe.Pointer(lParam))
		info.MinTrackSize.X = mainWindowMinWidth
		info.MinTrackSize.Y = mainWindowMinHeight
		info.MaxTrackSize.X = mainWindowWidth
		info.MaxTrackSize.Y = mainWindowHeight
		return 0

	case wmNcHitTest:
		return hitTestMainWindow(hwnd, lParam)

	case wmNcDestroy:
		oldWindowProcByID.Delete(hwnd)
	}

	if oldProc, ok := oldWindowProcByID.Load(hwnd); ok {
		ret, _, _ := procCallWindowProc.Call(oldProc.(uintptr), hwnd, uintptr(msg), wParam, lParam)
		return ret
	}

	return htClient
}

func hitTestMainWindow(hwnd uintptr, lParam uintptr) uintptr {
	var rect winRect

	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return htClient
	}

	x := int32(int16(uint16(lParam & 0xffff)))
	y := int32(int16(uint16((lParam >> 16) & 0xffff)))

	const resizeBorder int32 = 8
	const titleBarHeight int32 = 52
	const titleBarButtonsWidth int32 = 120

	left := x >= rect.Left && x < rect.Left+resizeBorder
	right := x <= rect.Right && x > rect.Right-resizeBorder
	top := y >= rect.Top && y < rect.Top+resizeBorder
	bottom := y <= rect.Bottom && y > rect.Bottom-resizeBorder

	switch {
	case top && left:
		return htTopLeft
	case top && right:
		return htTopRight
	case bottom && left:
		return htBottomLeft
	case bottom && right:
		return htBottomRight
	case left:
		return htLeft
	case right:
		return htRight
	case top:
		return htTop
	case bottom:
		return htBottom
	}

	// Левая часть нашей кастомной панели двигает окно.
	// Правая часть оставлена как client-area, чтобы кнопки свернуть/закрыть были кликабельны.
	if y >= rect.Top && y < rect.Top+titleBarHeight && x < rect.Right-titleBarButtonsWidth {
		return htCaption
	}

	return htClient
}
