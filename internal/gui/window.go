package gui

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"sync"

	webview2 "github.com/jchv/go-webview2"
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
}

var (
	windowsByID sync.Map // sessionID -> *rdWebViewWindow
)

func OpenRDWindow(sessionID string, send func(OutgoingSignal) error) error {
	if sessionID == "" {
		sessionID = "rd-session"
	}

	if _, ok := windowsByID.Load(sessionID); ok {
		return nil
	}

	ready := make(chan error, 1)

	go func() {
		w := webview2.New(true)
		if w == nil {
			ready <- fmt.Errorf("webview2.New returned nil")
			return
		}

		win := &rdWebViewWindow{
			sessionID: sessionID,
			w:         w,
			done:      make(chan struct{}),
			send:      send,
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

		windowsByID.Store(sessionID, win)

		w.SetTitle("POSRelay RD " + sessionID)
		w.SetSize(1280, 720, webview2.HintNone)
		w.SetHtml(rdHTML(sessionID))

		ready <- nil

		w.Run()
		w.Destroy()

		windowsByID.Delete(sessionID)
		close(win.done)
	}()

	return <-ready
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
			"window.__RD_ON_SIGNAL(%s);",
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

// Временная совместимость со старым кодом.
// Потом можно удалить, когда везде перейдёшь на OpenRDWindow/CloseRDWindow.
func OpenVideoStub(sessionID string) error {
	return OpenRDWindow(sessionID, nil)
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

func rdHTML(sessionID string) string {
	safeSessionID := html.EscapeString(sessionID)

	return `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>POSRelay RD</title>
  <style>
    :root {
      --stream-ratio: 1.777777;
    }

    html, body {
      margin: 0;
      width: 100%;
      height: 100%;
      background: #050505;
      overflow: hidden;
      font-family: "Segoe UI", Arial, sans-serif;
      color: #d7d7d7;
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

    .stage {
      width: min(100vw, calc(100vh * var(--stream-ratio)));
      height: min(100vh, calc(100vw / var(--stream-ratio)));
      display: flex;
      align-items: center;
      justify-content: center;
      background: #000;
    }

    video {
      width: 100%;
      height: 100%;
      object-fit: contain;
      background: #000;
    }

    .status {
      position: fixed;
      left: 12px;
      bottom: 12px;
      padding: 6px 10px;
      font-size: 12px;
      background: rgba(0,0,0,.55);
      border: 1px solid #2d2d2d;
      border-radius: 8px;
      max-width: calc(100vw - 24px);
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
  </style>
</head>
<body>
  <div class="root">
    <div class="stage">
      <video id="rd-video" autoplay playsinline muted></video>
    </div>
  </div>

  <div class="status" id="status">session_id: ` + safeSessionID + ` | connecting...</div>

  <script>
    const video = document.getElementById("rd-video");
    const statusEl = document.getElementById("status");

    let pc = null;
    let lastSize = "";

    function setStatus(text) {
      statusEl.textContent = "session_id: ` + safeSessionID + ` | " + text;
    }

    function reportVideoSize() {
      if (!video.videoWidth || !video.videoHeight) return;

      const key = video.videoWidth + "x" + video.videoHeight;
      if (key === lastSize) return;

      lastSize = key;

      const ratio = video.videoWidth / video.videoHeight;
      document.documentElement.style.setProperty("--stream-ratio", String(ratio));

      if (window.rdVideoMeta) {
        window.rdVideoMeta(video.videoWidth, video.videoHeight);
      }
    }

    window.__RD_ON_SIGNAL = async function(raw) {
      const msg = typeof raw === "string" ? JSON.parse(raw) : raw;
      if (!pc) return;

      if (msg.type === "rd_answer" && msg.sdp) {
        await pc.setRemoteDescription({ type: "answer", sdp: msg.sdp });
        setStatus("remote answer applied");
        return;
      }

      if (msg.type === "rd_ice" && msg.candidate) {
        await pc.addIceCandidate(msg.candidate);
      }
    };

    async function startPeer() {
      pc = new RTCPeerConnection();

      pc.createDataChannel("control");
      pc.addTransceiver("video", { direction: "recvonly" });

      pc.onicecandidate = (event) => {
        if (!event.candidate) return;

        window.rdSignalOut(JSON.stringify({
          type: "rd_ice",
          candidate: event.candidate.toJSON(),
        }));
      };

      pc.ontrack = (event) => {
        if (event.streams && event.streams[0]) {
          video.srcObject = event.streams[0];
        } else {
          video.srcObject = new MediaStream([event.track]);
        }

        setStatus("video track received");
      };

      pc.onconnectionstatechange = () => {
        setStatus("pc=" + pc.connectionState);
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      window.rdSignalOut(JSON.stringify({
        type: "rd_offer",
        sdp: offer.sdp,
      }));

      setStatus("offer sent");
    }

    video.addEventListener("loadedmetadata", reportVideoSize);
    setInterval(reportVideoSize, 500);

    window.addEventListener("beforeunload", () => {
      try {
        window.rdSignalOut(JSON.stringify({ type: "rd_stop" }));
      } catch (_) {}

      if (pc) {
        pc.close();
      }
    });

    startPeer().catch((err) => {
      setStatus("error: " + err.message);
    });
  </script>
</body>
</html>`
}
