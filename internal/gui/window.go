package gui

import (
	"encoding/json"
	"fmt"
	"github.com/iancoleman/orderedmap"
	"os"
	"os/exec"
	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/logger"
	"runtime"
	"strconv"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

const (
	settingsWindowWidth  = 985
	settingsWindowHeight = 760
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

	settingsWindowMu sync.Mutex
	settingsWindow   webview2.WebView
)

func startConnectionProcess(clientID string, password string, startRD bool, showConsole bool) error {
	logger.Posrelayv.Infof("[GUI] Starting connection process: client_id=%s start_rd=%t show_console=%t", clientID, startRD, showConsole)

	exePath, err := os.Executable()
	if err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to resolve executable path: %v", err)
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
		logger.Posrelayv.Errorf("[GUI] Failed to start connection process: %v", err)
		return err
	}

	logger.Posrelayv.Debugf("[GUI] Connection process started: pid=%d", cmd.Process.Pid)

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		logger.Posrelayv.Errorf("[GUI] Failed to open connection process handle: pid=%d error=%v", cmd.Process.Pid, err)
		return fmt.Errorf("OpenProcess failed: %w", err)
	}
	defer windows.CloseHandle(processHandle)

	if err := addProcessToSessionJob(processHandle); err != nil {
		_ = cmd.Process.Kill()
		logger.Posrelayv.Errorf("[GUI] Failed to attach connection process to session job: pid=%d error=%v", cmd.Process.Pid, err)
		return err
	}

	if err := cmd.Process.Release(); err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to release connection process handle: pid=%d error=%v", cmd.Process.Pid, err)
		return err
	}

	logger.Posrelayv.Infof("[GUI] Connection process is running: pid=%d", cmd.Process.Pid)
	return nil
}

func ShowMainWindowPopup(message string) {
	mainWindowMu.Lock()
	w := mainWindow
	mainWindowMu.Unlock()

	if w == nil || message == "" {
		logger.Posrelayv.Debug("[GUI] Main window popup skipped because window or message is empty")
		return
	}

	payload, err := json.Marshal(map[string]string{
		"message": message,
	})
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to marshal main window popup payload: %v", err)
		return
	}

	logger.Posrelayv.Debugf("[GUI] Dispatching main window popup: length=%d", len(message))
	w.Dispatch(func() {
		js := fmt.Sprintf(
			"window.dispatchEvent(new CustomEvent('main-ui-popup', { detail: %s }));",
			string(payload),
		)
		w.Eval(js)
	})
}

func dispatchSettingsWindowState(open bool) {
	mainWindowMu.Lock()
	w := mainWindow
	mainWindowMu.Unlock()

	if w == nil {
		logger.Posrelayv.Debug("[GUI] Settings window state dispatch skipped because main window is empty")
		return
	}

	payload, err := json.Marshal(map[string]bool{
		"open": open,
	})
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to marshal settings window state payload: %v", err)
		return
	}

	w.Dispatch(func() {
		js := fmt.Sprintf(
			"window.dispatchEvent(new CustomEvent('settings-window-state', { detail: %s }));",
			string(payload),
		)
		w.Eval(js)
	})
}

func ToggleSettingsWindow(version string) bool {
	settingsWindowMu.Lock()
	existingWindow := settingsWindow
	settingsWindowMu.Unlock()

	if existingWindow != nil {
		logger.Posrelayv.Debug("[GUI] Closing settings window by toggle button")
		existingWindow.Dispatch(func() {
			CloseMainWindow(existingWindow)
		})
		return false
	}

	OpenSettingsWindow(version)
	return true
}

func OpenSettingsWindow(version string) {
	settingsWindowMu.Lock()
	existingWindow := settingsWindow
	settingsWindowMu.Unlock()

	if existingWindow != nil {
		logger.Posrelayv.Debug("[GUI] Settings window is already open")
		existingWindow.Dispatch(func() {
			ShowWebViewWindow(existingWindow)
		})
		dispatchSettingsWindowState(true)
		return
	}

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		logger.Posrelayv.Debug("[GUI] Creating settings WebView window")

		debugWebView := IsWebView2DebugEnabled()

		w := webview2.NewWithOptions(webview2.WebViewOptions{
			Debug:     debugWebView,
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  "Settings",
				Width:  settingsWindowWidth,
				Height: settingsWindowHeight,
				Center: true,
			},
		})
		if w == nil {
			logger.Posrelayv.Errorf("[GUI] Failed to create settings WebView window")
			return
		}

		settingsWindowMu.Lock()
		settingsWindow = w
		settingsWindowMu.Unlock()

		settingsWindowMu.Lock()
		settingsWindow = w
		settingsWindowMu.Unlock()

		dispatchSettingsWindowState(true)

		defer func() {
			settingsWindowMu.Lock()
			if settingsWindow == w {
				settingsWindow = nil
			}
			settingsWindowMu.Unlock()

			dispatchSettingsWindowState(false)

			logger.Posrelayv.Debug("[GUI] Settings window reference cleared")
		}()

		defer w.Destroy()

		if err := setTaskbarIcon(w); err != nil {
			logger.Posrelayv.Warnf("[GUI] Settings taskbar icon setup failed: %v", err)
		}

		if err := w.Bind("loadSettingsConfigs", func() map[string]any {
			configs, err := config.LoadSettingsConfigs()
			if err != nil {
				logger.Posrelayv.Errorf("[GUI] Failed to load settings configs: %v", err)
				return map[string]any{
					"ok":      false,
					"message": err.Error(),
					"configs": []config.SettingsConfigFile{},
				}
			}

			return map[string]any{
				"ok":      true,
				"message": "",
				"configs": configs,
			}
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind loadSettingsConfigs: %v", err)
			return
		}

		if err := w.Bind("saveSettingsConfig", func(name string, data *orderedmap.OrderedMap) map[string]any {
			if err := config.SaveSettingsConfig(name, data); err != nil {
				logger.Posrelayv.Errorf("[GUI] Failed to save settings config %s: %v", name, err)
				return map[string]any{
					"ok":      false,
					"message": err.Error(),
				}
			}

			return map[string]any{
				"ok":      true,
				"message": "Settings saved",
			}
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind saveSettingsConfig: %v", err)
			return
		}

		if err := w.Bind("settingsWindowMinimize", func() {
			MinimizeMainWindow(w)
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind settingsWindowMinimize: %v", err)
			return
		}

		if err := w.Bind("settingsWindowClose", func() {
			CloseMainWindow(w)
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind settingsWindowClose: %v", err)
			return
		}

		if err := w.Bind("settingsWindowDrag", func() {
			DragMainWindow(w)
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind settingsWindowDrag: %v", err)
			return
		}

		if err := ApplyFixedWindowChrome(w, settingsWindowWidth, settingsWindowHeight); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to apply settings window chrome: %v", err)
			return
		}

		uiURL, err := rdWebURL("")
		if err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to resolve settings URL: %v", err)
			return
		}

		settingsURL := uiURL + "settings.html"

		w.SetTitle("POSRelayv Settings")

		logger.Posrelayv.Infof("[GUI] Navigating settings window: url=%s", settingsURL)
		w.Navigate(settingsURL)

		w.Run()

		logger.Posrelayv.Debug("[GUI] Settings window event loop stopped")
	}()
}

func OpenMainWindow(startSession StartSessionHandler, version string) error {
	logger.Posrelayv.Infof(
		"POSRelayv.v%s starting...", version)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	logger.Posrelayv.Debug("[GUI] Creating main WebView window")

	debugWebView := IsWebView2DebugEnabled()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     debugWebView,
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "POSRelayv",
			Width:  985,
			Height: 760,
			Center: false,
		},
	})
	if w == nil {
		logger.Posrelayv.Errorf("[GUI] Failed to create main WebView window")
		return fmt.Errorf("webview2.NewWithOptions returned nil")
	}

	mainWindowMu.Lock()
	mainWindow = w
	// Set icon for taskbar after window creation
	if err := setTaskbarIcon(w); err != nil {
		logger.Posrelayv.Warnf("[GUI] Taskbar icon setup failed: %v", err)
	}
	mainWindowMu.Unlock()

	defer func() {
		mainWindowMu.Lock()
		if mainWindow == w {
			mainWindow = nil
		}
		mainWindowMu.Unlock()
		logger.Posrelayv.Debug("[GUI] Main window reference cleared")
	}()

	defer w.Destroy()
	defer closeSessionJob()

	if err := w.Bind("toggleSettingsWindow", func() bool {
		return ToggleSettingsWindow(version)
	}); err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to bind toggleSettingsWindow: %v", err)
		return err
	}

	if err := w.Bind("startHiddenConsole", func(clientID string, password string) map[string]any {
		if err := startConnectionProcess(clientID, password, true, false); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to start hidden console with RD: client_id=%s error=%v", clientID, err)
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
		logger.Posrelayv.Errorf("[GUI] Failed to bind startHiddenConsole: %v", err)
		return err
	}

	if err := w.Bind("startHiddenConsoleNoRD", func(clientID string, password string) map[string]any {
		if err := startConnectionProcess(clientID, password, false, true); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to start hidden console without RD: client_id=%s error=%v", clientID, err)
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
		logger.Posrelayv.Errorf("[GUI] Failed to bind startHiddenConsoleNoRD: %v", err)
		return err
	}

	if err := w.Bind("mainWindowMinimize", func() {
		MinimizeMainWindow(w)
	}); err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to bind mainWindowMinimize: %v", err)
		return err
	}

	if err := w.Bind("mainWindowClose", func() {
		CloseMainWindow(w)
	}); err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to bind mainWindowClose: %v", err)
		return err
	}

	if err := w.Bind("mainWindowDrag", func() {
		DragMainWindow(w)
	}); err != nil {
		logger.Posrelayv.Errorf("[GUI] Failed to bind mainWindowDrag: %v", err)
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

	logger.Posrelayv.Debug("[GUI] Navigating main window")
	w.Navigate(uiURL)

	logger.Posrelayv.Debug("[GUI] Running main window event loop")
	w.Run()
	logger.Posrelayv.Debug("[GUI] Main window event loop stopped")

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
		logger.Posrelayv.Debugf("[GUI] RD window is already open: session_id=%s", sessionID)
		return nil
	}

	logger.Posrelayv.Infof("[GUI] Opening RD window: session_id=%s", sessionID)

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

		debugWebView := IsWebView2DebugEnabled()

		w := webview2.New(debugWebView)

		if w == nil {
			logger.Posrelayv.Errorf("[GUI] Failed to create RD WebView window: session_id=%s", sessionID)
			markReady(fmt.Errorf("webview2.New returned nil"))
			return
		}
		defer w.Destroy()

		if err := setTaskbarIcon(w); err != nil {
			logger.Posrelayv.Warnf("[GUI] RD taskbar icon setup failed: %v", err)
		}

		win := &rdWebViewWindow{
			sessionID: sessionID,
			w:         w,
			done:      make(chan struct{}),
			send:      send,
			onClose:   onClose,
		}

		if err := w.Bind("rdWindowReady", func() {
			logger.Posrelayv.Debugf("[GUI] RD window reported ready: session_id=%s", sessionID)
			markReady(nil)
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind rdWindowReady: session_id=%s error=%v", sessionID, err)
			markReady(err)
			return
		}

		if err := w.Bind("rdSignalOut", func(raw string) {
			var in OutgoingSignal
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				logger.Posrelayv.Warnf("[GUI] Failed to parse outgoing RD signal: session_id=%s error=%v", sessionID, err)
				return
			}

			if win.send != nil {
				if err := win.send(in); err != nil {
					logger.Posrelayv.Warnf("[GUI] Failed to send outgoing RD signal: session_id=%s type=%s error=%v", sessionID, in.Type, err)
				}
			}
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind rdSignalOut: session_id=%s error=%v", sessionID, err)
			ready <- err
			return
		}

		if err := w.Bind("rdVideoMeta", func(width, height int) {
			if width <= 0 || height <= 0 {
				logger.Posrelayv.Debugf("[GUI] Ignoring invalid RD video metadata: session_id=%s width=%d height=%d", sessionID, width, height)
				return
			}

			fw, fh := fitWindow(width, height, 1600, 1000)
			logger.Posrelayv.Debugf("[GUI] Resizing RD window from video metadata: session_id=%s source=%dx%d window=%dx%d", sessionID, width, height, fw, fh)

			w.Dispatch(func() {
				w.SetSize(fw, fh, webview2.HintNone)
			})
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind rdVideoMeta: session_id=%s error=%v", sessionID, err)
			ready <- err
			return
		}

		if err := w.Bind("rdClipboardRead", func() string {
			text, err := ClipboardReadText()
			if err != nil {
				logger.Posrelayv.Warnf("[GUI] RD clipboard read failed: session_id=%s error=%v", sessionID, err)
				return ""
			}
			return text
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind rdClipboardRead: session_id=%s error=%v", sessionID, err)
			ready <- err
			return
		}

		if err := w.Bind("rdClipboardWrite", func(text string) bool {
			if err := ClipboardWriteText(text); err != nil {
				logger.Posrelayv.Warnf("[GUI] RD clipboard write failed: session_id=%s error=%v", sessionID, err)
				return false
			}
			return true
		}); err != nil {
			logger.Posrelayv.Errorf("[GUI] Failed to bind rdClipboardWrite: session_id=%s error=%v", sessionID, err)
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
		logger.Posrelayv.Infof("[GUI] Navigating RD window: session_id=%s url=%s", sessionID, uiURL)
		w.Navigate(uiURL)

		w.Run()

		windowsByID.Delete(sessionID)
		close(win.done)
		logger.Posrelayv.Infof("[GUI] RD window closed: session_id=%s", sessionID)

		if win.onClose != nil {
			go win.onClose(sessionID)
		}
	}()

	select {
	case err := <-ready:
		if err != nil {
			logger.Posrelayv.Errorf("[GUI] RD window failed to become ready: session_id=%s error=%v", sessionID, err)
		}
		return err

	case <-time.After(10 * time.Second):
		CloseRDWindow(sessionID)
		logger.Posrelayv.Errorf("[GUI] RD window did not become ready in time: session_id=%s", sessionID)
		return fmt.Errorf("RD window did not become ready in time: session_id=%s", sessionID)
	}
}

func PushRDSignal(sessionID string, msg any) error {
	raw, ok := windowsByID.Load(sessionID)
	if !ok {
		logger.Posrelayv.Warnf("[GUI] Cannot push RD signal because window was not found: session_id=%s", sessionID)
		return fmt.Errorf("window not found: %s", sessionID)
	}

	win := raw.(*rdWebViewWindow)

	data, err := json.Marshal(msg)
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to marshal incoming RD signal: session_id=%s error=%v", sessionID, err)
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
		logger.Posrelayv.Debugf("[GUI] Terminating RD window: session_id=%s", sessionID)
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
		w = h * srcW / srcH
	}

	return w, h
}
