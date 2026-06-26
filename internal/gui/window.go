package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

type OutgoingSignal struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

type rdWebViewWindow struct {
	sessionID string
	w         webview2.WebView
	done      chan struct{}
	send      func(OutgoingSignal) error
	onClose   func(sessionID string)
}

type StartSessionHandler func(clientID string, password string, startRD bool, showConsole bool) error

var (
	windowsByID sync.Map // sessionID -> *rdWebViewWindow

	mainWindowMu sync.Mutex
	mainWindow   webview2.WebView
)

func startConnectionProcess(clientID string, password string, startRD bool, showConsole bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	uiBaseURL, err := ensureRDWebServer()
	if err != nil {
		return err
	}

	mainUIEventURL := uiBaseURL + "api/main-ui-event"

	startRDValue := "0"
	if startRD {
		startRDValue = "1"
	}

	showConsoleValue := "0"
	if showConsole {
		showConsoleValue = "1"
	}

	cmd := exec.Command(exePath, "-session")

	cmd.Env = append(
		os.Environ(),
		"POSRELAY_CLIENT_ID="+clientID,
		"POSRELAY_PASSWORD="+password,
		"POSRELAY_START_RD="+startRDValue,
		"POSRELAY_SHOW_CONSOLE="+showConsoleValue,
		"POSRELAY_MAIN_UI_EVENT_URL="+mainUIEventURL,
	)

	if err := cmd.Start(); err != nil {
		return err
	}

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("OpenProcess failed: %w", err)
	}
	defer windows.CloseHandle(processHandle)

	if err := addProcessToSessionJob(processHandle); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	return cmd.Process.Release()
}

func ShowMainWindowPopup(message string) {
	mainWindowMu.Lock()
	w := mainWindow
	mainWindowMu.Unlock()

	if w == nil || message == "" {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"message": message,
	})
	if err != nil {
		return
	}

	w.Dispatch(func() {
		js := fmt.Sprintf(
			"window.dispatchEvent(new CustomEvent('main-ui-popup', { detail: %s }));",
			string(payload),
		)
		w.Eval(js)
	})
}

func OpenMainWindow(startSession StartSessionHandler) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "POSRelayv",
			Width:  985,
			Height: 760,
			Center: false,
		},
	})
	if w == nil {
		return fmt.Errorf("webview2.NewWithOptions returned nil")
	}

	mainWindowMu.Lock()
	mainWindow = w
	// Set icon for taskbar after window creation
	if err := setTaskbarIcon(w); err != nil {
		fmt.Println("setTaskbarIcon error:", err)
	}
	mainWindowMu.Unlock()

	defer func() {
		mainWindowMu.Lock()
		if mainWindow == w {
			mainWindow = nil
		}
		mainWindowMu.Unlock()
	}()

	defer w.Destroy()
	defer closeSessionJob()

	if err := w.Bind("startHiddenConsole", func(clientID string, password string) map[string]any {
		if err := startConnectionProcess(clientID, password, true, false); err != nil {
			return map[string]any{
				"ok":      false,
				"message": err.Error(),
			}
		}

		return map[string]any{
			"ok":      true,
			"message": "Подключение с RD запущено",
		}
	}); err != nil {
		return err
	}

	if err := w.Bind("startHiddenConsoleNoRD", func(clientID string, password string) map[string]any {
		if err := startConnectionProcess(clientID, password, false, true); err != nil {
			return map[string]any{
				"ok":      false,
				"message": err.Error(),
			}
		}

		return map[string]any{
			"ok":      true,
			"message": "Подключение без RD запущено",
		}
	}); err != nil {
		return err
	}

	if err := w.Bind("mainWindowMinimize", func() {
		MinimizeMainWindow(w)
	}); err != nil {
		return err
	}

	if err := w.Bind("mainWindowClose", func() {
		CloseMainWindow(w)
	}); err != nil {
		return err
	}

	if err := w.Bind("mainWindowDrag", func() {
		DragMainWindow(w)
	}); err != nil {
		return err
	}

	uiURL, err := rdWebURL("")
	if err != nil {
		return err
	}

	w.SetSize(500, 550, webview2.HintMin)
	w.SetSize(1150, 1400, webview2.HintMax)

	if err := ApplyMainWindowChrome(w); err != nil {
		return err
	}

	w.Navigate(uiURL)

	w.Run()

	return nil
}

func OpenRDWindow(
	sessionID string,
	send func(OutgoingSignal) error,
	onClose func(sessionID string),
) error {
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

		var readyOnce sync.Once

		markReady := func(err error) {
			readyOnce.Do(func() {
				ready <- err
			})
		}

		w := webview2.New(true)
		if w == nil {
			markReady(fmt.Errorf("webview2.New returned nil"))
			return
		}
		defer w.Destroy()

		win := &rdWebViewWindow{
			sessionID: sessionID,
			w:         w,
			done:      make(chan struct{}),
			send:      send,
			onClose:   onClose,
		}

		if err := w.Bind("rdWindowReady", func() {
			markReady(nil)
		}); err != nil {
			markReady(err)
			return
		}

		if err := w.Bind("rdSignalOut", func(raw string) {
			var in OutgoingSignal
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return
			}

			if win.send != nil {
				_ = win.send(in)
			}
		}); err != nil {
			ready <- err
			return
		}

		if err := w.Bind("rdVideoMeta", func(width, height int) {
			if width <= 0 || height <= 0 {
				return
			}

			fw, fh := fitWindow(width, height, 1600, 1000)

			w.Dispatch(func() {
				w.SetSize(fw, fh, webview2.HintNone)
			})
		}); err != nil {
			ready <- err
			return
		}

		if err := w.Bind("rdClipboardRead", func() string {
			text, err := ClipboardReadText()
			if err != nil {
				return ""
			}
			return text
		}); err != nil {
			ready <- err
			return
		}

		if err := w.Bind("rdClipboardWrite", func(text string) bool {
			return ClipboardWriteText(text) == nil
		}); err != nil {
			ready <- err
			return
		}

		uiURL, err := rdWebURL(sessionID)
		if err != nil {
			ready <- err
			return
		}

		windowsByID.Store(sessionID, win)

		w.SetTitle("POSRelay RD " + sessionID)
		w.SetSize(1280, 720, webview2.HintNone)

		// Важно:
		// React/Vite UI больше не передаём через SetHtml/NavigateToString.
		// Иначе большой bundle может отображаться как текст или ломаться на HTML parser edge cases.
		w.Navigate(uiURL)

		w.Run()

		windowsByID.Delete(sessionID)
		close(win.done)

		if win.onClose != nil {
			go win.onClose(sessionID)
		}
	}()

	select {
	case err := <-ready:
		return err

	case <-time.After(10 * time.Second):
		CloseRDWindow(sessionID)
		return fmt.Errorf("RD window did not become ready in time: session_id=%s", sessionID)
	}
}

func PushRDSignal(sessionID string, msg any) error {
	raw, ok := windowsByID.Load(sessionID)
	if !ok {
		return fmt.Errorf("window not found: %s", sessionID)
	}

	win := raw.(*rdWebViewWindow)

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	win.w.Dispatch(func() {
		js := fmt.Sprintf(
			"if (window.__RD_ON_SIGNAL) { window.__RD_ON_SIGNAL(%s); }",
			strconv.Quote(string(data)),
		)
		win.w.Eval(js)
	})

	return nil
}

func CloseRDWindow(sessionID string) {
	if sessionID == "" {
		return
	}

	raw, ok := windowsByID.Load(sessionID)
	if !ok {
		return
	}

	win := raw.(*rdWebViewWindow)

	if win.w != nil {
		win.w.Terminate()
	}
}

func OpenVideoStub(sessionID string) error {
	return OpenRDWindow(sessionID, nil, nil)
}

func CloseVideoStub(sessionID string) {
	CloseRDWindow(sessionID)
}

func fitWindow(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= 0 || srcH <= 0 {
		return 1280, 720
	}

	w := maxW
	h := w * srcH / srcW

	if h > maxH {
		h = maxH
		w = h * srcW / srcH
	}

	if w < 640 {
		w = 640
		h = w * srcH / srcW
	}

	if h < 360 {
		h = 360
		w = h * srcH / srcW
	}

	return w, h
}
