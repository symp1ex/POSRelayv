package gui

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

const rdWindowClassName = "POSRelayvRDStubWindow"

var (
	registerOnce sync.Once
	windowsByID  sync.Map // sessionID -> win.HWND
)

func mustUTF16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

func registerWindowClass() error {
	var regErr error

	registerOnce.Do(func() {
		wc := win.WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			Style:         win.CS_HREDRAW | win.CS_VREDRAW,
			LpfnWndProc:   syscall.NewCallback(wndProc),
			HInstance:     win.GetModuleHandle(nil),
			HbrBackground: win.HBRUSH(win.GetStockObject(win.BLACK_BRUSH)),
			LpszClassName: mustUTF16Ptr(rdWindowClassName),
		}

		if win.RegisterClassEx(&wc) == 0 {
			regErr = fmt.Errorf("RegisterClassEx failed")
		}
	})

	return regErr
}

func OpenVideoStub(sessionID string) error {
	if sessionID == "" {
		sessionID = "rd-session"
	}

	if _, ok := windowsByID.Load(sessionID); ok {
		return nil
	}

	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if err := registerWindowClass(); err != nil {
			ready <- err
			return
		}

		title := mustUTF16Ptr("RD Stub " + sessionID)

		hwnd := win.CreateWindowEx(
			win.WS_EX_NOACTIVATE|win.WS_EX_TOOLWINDOW,
			mustUTF16Ptr(rdWindowClassName),
			title,
			win.WS_POPUP,
			100,
			100,
			1280,
			720,
			0,
			0,
			win.GetModuleHandle(nil),
			nil,
		)

		if hwnd == 0 {
			ready <- fmt.Errorf("CreateWindowEx failed")
			return
		}

		windowsByID.Store(sessionID, hwnd)

		win.ShowWindow(hwnd, win.SW_SHOWNOACTIVATE)

		win.SetWindowPos(
			hwnd,
			0,
			100,
			100,
			1280,
			720,
			win.SWP_NOZORDER|win.SWP_NOACTIVATE|win.SWP_SHOWWINDOW,
		)

		win.UpdateWindow(hwnd)

		ready <- nil

		var msg win.MSG
		for {
			ret := win.GetMessage(&msg, 0, 0, 0)
			if ret == 0 || ret == -1 {
				break
			}
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}

		windowsByID.Delete(sessionID)
	}()

	return <-ready
}

func CloseVideoStub(sessionID string) {
	if sessionID == "" {
		return
	}

	raw, ok := windowsByID.Load(sessionID)
	if !ok {
		return
	}

	hwnd := raw.(win.HWND)
	win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
}

func wndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_DESTROY:
		win.PostQuitMessage(0)
		return 0
	default:
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
}
