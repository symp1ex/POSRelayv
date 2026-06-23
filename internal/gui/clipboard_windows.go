//go:build windows

package gui

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clipboardMu sync.Mutex

	user32clip                      = windows.NewLazySystemDLL("user32.dll")
	kernel32clip                    = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboardViewer         = user32clip.NewProc("OpenClipboard")
	procCloseClipboardViewer        = user32clip.NewProc("CloseClipboard")
	procEmptyClipboardViewer        = user32clip.NewProc("EmptyClipboard")
	procSetClipboardDataViewer      = user32clip.NewProc("SetClipboardData")
	procGetClipboardDataViewer      = user32clip.NewProc("GetClipboardData")
	procIsClipboardFormatAvailableV = user32clip.NewProc("IsClipboardFormatAvailable")
	procGlobalAllocViewer           = kernel32clip.NewProc("GlobalAlloc")
	procGlobalLockViewer            = kernel32clip.NewProc("GlobalLock")
	procGlobalUnlockViewer          = kernel32clip.NewProc("GlobalUnlock")
	procGlobalFreeViewer            = kernel32clip.NewProc("GlobalFree")
)

const (
	viewerCFUnicodeText = 13
	viewerGMemMoveable  = 0x0002
)

func ClipboardReadText() (string, error) {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()

	if ret, _, _ := procIsClipboardFormatAvailableV.Call(viewerCFUnicodeText); ret == 0 {
		return "", nil
	}

	if err := openViewerClipboardWithRetry(); err != nil {
		return "", err
	}
	defer procCloseClipboardViewer.Call()

	h, _, err := procGetClipboardDataViewer.Call(viewerCFUnicodeText)
	if h == 0 {
		return "", fmt.Errorf("GetClipboardData failed: %w", err)
	}

	ptr, _, err := procGlobalLockViewer.Call(h)
	if ptr == 0 {
		return "", fmt.Errorf("GlobalLock failed: %w", err)
	}
	defer procGlobalUnlockViewer.Call(h)

	var chars []uint16
	for p := ptr; ; p += 2 {
		ch := *(*uint16)(unsafe.Pointer(p))
		if ch == 0 {
			break
		}
		chars = append(chars, ch)
	}

	return strings.TrimRight(syscall.UTF16ToString(chars), "\x00"), nil
}

func ClipboardWriteText(text string) error {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()

	if err := openViewerClipboardWithRetry(); err != nil {
		return err
	}
	defer procCloseClipboardViewer.Call()

	if ret, _, err := procEmptyClipboardViewer.Call(); ret == 0 {
		return fmt.Errorf("EmptyClipboard failed: %w", err)
	}

	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}

	size := uintptr(len(utf16) * 2)

	hMem, _, err := procGlobalAllocViewer.Call(viewerGMemMoveable, size)
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc failed: %w", err)
	}

	ptr, _, err := procGlobalLockViewer.Call(hMem)
	if ptr == 0 {
		procGlobalFreeViewer.Call(hMem)
		return fmt.Errorf("GlobalLock failed: %w", err)
	}

	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16)), utf16)
	procGlobalUnlockViewer.Call(hMem)

	if ret, _, err := procSetClipboardDataViewer.Call(viewerCFUnicodeText, hMem); ret == 0 {
		procGlobalFreeViewer.Call(hMem)
		return fmt.Errorf("SetClipboardData failed: %w", err)
	}

	return nil
}

func openViewerClipboardWithRetry() error {
	var lastErr error

	for attempt := 0; attempt < 20; attempt++ {
		if ret, _, err := procOpenClipboardViewer.Call(0); ret != 0 {
			return nil
		} else {
			lastErr = err
		}

		time.Sleep(10 * time.Millisecond)
	}

	return fmt.Errorf("OpenClipboard failed after retries: %w", lastErr)
}
