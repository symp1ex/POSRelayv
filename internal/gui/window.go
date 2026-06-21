package gui

import (
	"fmt"
	"html"
	"sync"

	webview "github.com/webview/webview_go"
)

type rdWebViewWindow struct {
	sessionID string
	w         webview.WebView
	done      chan struct{}
}

var (
	windowsByID sync.Map // sessionID -> *rdWebViewWindow
)

func OpenVideoStub(sessionID string) error {
	if sessionID == "" {
		sessionID = "rd-session"
	}

	if _, ok := windowsByID.Load(sessionID); ok {
		return nil
	}

	ready := make(chan error, 1)

	go func() {
		w := webview.New(true)
		if w == nil {
			ready <- fmt.Errorf("webview.New returned nil")
			return
		}

		win := &rdWebViewWindow{
			sessionID: sessionID,
			w:         w,
			done:      make(chan struct{}),
		}

		windowsByID.Store(sessionID, win)

		w.SetTitle("POSRelay RD " + sessionID)
		w.SetSize(1280, 720, webview.HintNone)
		w.SetHtml(stubHTML(sessionID))

		ready <- nil

		w.Run()
		w.Destroy()

		windowsByID.Delete(sessionID)
		close(win.done)
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

	win := raw.(*rdWebViewWindow)

	if win.w != nil {
		win.w.Terminate()
	}
}

func stubHTML(sessionID string) string {
	safeSessionID := html.EscapeString(sessionID)

	return `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>POSRelay RD</title>
  <style>
    html, body {
      margin: 0;
      width: 100%;
      height: 100%;
      background: #0b0b0b;
      color: #d7d7d7;
      font-family: "Segoe UI", Arial, sans-serif;
      overflow: hidden;
    }

    .root {
      width: 100vw;
      height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background:
        radial-gradient(circle at center, #202020 0, #111 45%, #050505 100%);
    }

    .panel {
      border: 1px solid #333;
      border-radius: 12px;
      padding: 28px 34px;
      background: rgba(20, 20, 20, 0.92);
      box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45);
      text-align: center;
      min-width: 420px;
    }

    .title {
      font-size: 22px;
      font-weight: 600;
      margin-bottom: 12px;
    }

    .subtitle {
      font-size: 14px;
      color: #aaa;
      margin-bottom: 18px;
    }

    .session {
      font-family: Consolas, "Courier New", monospace;
      font-size: 13px;
      color: #89c2ff;
      word-break: break-all;
      background: #111;
      border: 1px solid #2c2c2c;
      border-radius: 8px;
      padding: 10px 12px;
    }

    .status {
      margin-top: 18px;
      font-size: 13px;
      color: #8fdd8f;
    }
  </style>
</head>
<body>
  <div class="root">
    <div class="panel">
      <div class="title">POSRelay RD WebView</div>
      <div class="subtitle">Пустое WebView-окно. Видео и WebRTC пока не подключены.</div>
      <div class="session">session_id: ` + safeSessionID + `</div>
      <div class="status">Окно открыто по текущему RD-протоколу</div>
    </div>
  </div>
</body>
</html>`
}
