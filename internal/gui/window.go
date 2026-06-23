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
  const stage = document.querySelector(".stage");

  let pc = null;
  let control = null;
  let lastSize = "";

  const sessionID = "` + safeSessionID + `";
  const browserCrypto = globalThis.crypto || null;
const browserOrigin =
  browserCrypto && typeof browserCrypto.randomUUID === "function"
    ? browserCrypto.randomUUID()
    : ("browser-" + Math.random().toString(16).slice(2) + "-" + Date.now());
  const MAX_CLIPBOARD_TEXT_BYTES = 60 * 1024;

  let msgSeq = 0;
  let clipboardSeq = 0;
  let lastClipboardRevision = "";
  let queuedRemoteClipboard = null;

  let rdFocused = false;
  let rdWindowActive = !document.hidden && document.hasFocus();

  const pressedKeys = new Set();

  function setStatus(text) {
    statusEl.textContent = "session_id: ` + safeSessionID + ` | " + text;
  }

  function now() {
    return Date.now();
  }

async function sha256Text(text) {
  const input = String(text ?? "");
  const data = new TextEncoder().encode(input);

  if (
    browserCrypto &&
    browserCrypto.subtle &&
    typeof browserCrypto.subtle.digest === "function"
  ) {
    const digest = await browserCrypto.subtle.digest("SHA-256", data);

    return "sha256:" + Array.from(new Uint8Array(digest))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }

  // Fallback для WebView2 через SetHtml(), где crypto.subtle может быть недоступен.
  // Это НЕ криптографический SHA-256, но достаточно для локальной дедупликации viewer.
  // Agent всё равно умеет сам вычислять Revision(text), если revision пустой.
  let h = 2166136261;

  for (let i = 0; i < data.length; i++) {
    h ^= data[i];
    h = Math.imul(h, 16777619);
  }

  return "local-fnv1a:" + data.length + ":" + (h >>> 0).toString(16).padStart(8, "0");
}

  function utf8Bytes(text) {
    return new TextEncoder().encode(String(text ?? "")).length;
  }

  function normalizeClipboardText(text) {
    const safe = String(text ?? "").replace(/\u0000/g, "");
    const size = utf8Bytes(safe);

    if (size > MAX_CLIPBOARD_TEXT_BYTES) {
      throw new Error("clipboard payload too large: " + size + " bytes");
    }

    return safe;
  }

  function sendControl(msg) {
    if (!control || control.readyState !== "open") {
      return false;
    }

    if (control.bufferedAmount > 256 * 1024) {
      setStatus("control backpressure: " + control.bufferedAmount);
      return false;
    }

    control.send(JSON.stringify({
      id: String(++msgSeq),
      session_id: sessionID,
      ts: now(),
      ...msg,
    }));

    return true;
  }

  function setRDFocus(focused) {
    if (rdFocused === focused) {
      return;
    }

    rdFocused = focused;
    sendControl({ type: "focus_changed", focused });

    if (!focused) {
      releasePressedKeys();
    }
  }

  function releasePressedKeys() {
    for (const code of Array.from(pressedKeys)) {
      sendControl({
        type: "key_up",
        code,
        key: "",
        ctrl: false,
        shift: false,
        alt: false,
        meta: false,
        repeat: false,
      });
    }

    pressedKeys.clear();
  }

  function reportVideoSize() {
    if (!video.videoWidth || !video.videoHeight) {
      return;
    }

    const key = video.videoWidth + "x" + video.videoHeight;
    if (key === lastSize) {
      return;
    }

    lastSize = key;

    const ratio = video.videoWidth / video.videoHeight;
    document.documentElement.style.setProperty("--stream-ratio", String(ratio));

    if (window.rdVideoMeta) {
      window.rdVideoMeta(video.videoWidth, video.videoHeight);
    }
  }

  function getVideoContentRect() {
    const rect = video.getBoundingClientRect();

    if (!video.videoWidth || !video.videoHeight) {
      return rect;
    }

    const videoRatio = video.videoWidth / video.videoHeight;
    const boxRatio = rect.width / rect.height;

    if (boxRatio > videoRatio) {
      const contentWidth = rect.height * videoRatio;
      const x = rect.left + (rect.width - contentWidth) / 2;

      return {
        left: x,
        top: rect.top,
        width: contentWidth,
        height: rect.height,
        right: x + contentWidth,
        bottom: rect.bottom,
      };
    }

    const contentHeight = rect.width / videoRatio;
    const y = rect.top + (rect.height - contentHeight) / 2;

    return {
      left: rect.left,
      top: y,
      width: rect.width,
      height: contentHeight,
      right: rect.right,
      bottom: y + contentHeight,
    };
  }

  function normalizedPoint(event) {
    const rect = getVideoContentRect();

    const x = (event.clientX - rect.left) / rect.width;
    const y = (event.clientY - rect.top) / rect.height;

    return {
      x: Math.max(0, Math.min(1, x)),
      y: Math.max(0, Math.min(1, y)),
      inside:
        event.clientX >= rect.left &&
        event.clientX <= rect.right &&
        event.clientY >= rect.top &&
        event.clientY <= rect.bottom,
    };
  }

  function buttonName(button) {
    switch (button) {
      case 0:
        return "left";
      case 1:
        return "middle";
      case 2:
        return "right";
      default:
        return "left";
    }
  }

  async function readLocalClipboardText() {
    if (window.rdClipboardRead) {
      return await window.rdClipboardRead();
    }

    if (navigator.clipboard && navigator.clipboard.readText) {
      return await navigator.clipboard.readText();
    }

    return "";
  }

  async function writeLocalClipboardText(text) {
    if (window.rdClipboardWrite) {
      return await window.rdClipboardWrite(text);
    }

    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }

    return false;
  }

  async function readLocalClipboardSnapshot() {
    const text = normalizeClipboardText(await readLocalClipboardText());
    const revision = await sha256Text(text);

    return { text, revision };
  }

  async function pushLocalClipboardToAgent(reason = "local-event") {
    if (!control || control.readyState !== "open") {
      return false;
    }

    const snap = await readLocalClipboardSnapshot();

    if (snap.revision === lastClipboardRevision) {
      return false;
    }

    lastClipboardRevision = snap.revision;

return sendControl({
  type: "clipboard_sync",
  origin: browserOrigin,
  seq: ++clipboardSeq,

  // Важно:
  // если WebView2 не дал crypto.subtle, snap.revision будет local-fnv1a.
  // Agent ожидает sha256 и уже умеет сам вычислять Revision(text), когда revision пустой.
  revision: snap.revision.startsWith("sha256:") ? snap.revision : "",

  text: snap.text,
  reason,
});
  }

  async function applyClipboardFromAgent(msg) {
    if (!msg || msg.origin === browserOrigin) {
      return;
    }

    if (!("text" in msg)) {
      return;
    }

    let text;
    let revision;

    try {
      text = normalizeClipboardText(msg.text);
      revision = msg.revision || await sha256Text(text);
    } catch (err) {
      setStatus("clipboard payload rejected: " + err.message);
      return;
    }

    if (revision === lastClipboardRevision) {
      return;
    }

    // Если это обычный браузер без native bridge, запись clipboard может требовать
    // активного окна / user activation. В WebView2 основной путь — rdClipboardWrite.
    if (!rdWindowActive && !window.rdClipboardWrite) {
      queuedRemoteClipboard = { ...msg, text, revision };
      return;
    }

    try {
      const ok = await writeLocalClipboardText(text);

      if (!ok) {
        queuedRemoteClipboard = { ...msg, text, revision };
        setStatus("clipboard write unavailable");
        return;
      }

      lastClipboardRevision = revision;
    } catch (err) {
      queuedRemoteClipboard = { ...msg, text, revision };
      setStatus("clipboard write failed: " + err.message);
    }
  }

  function scheduleLocalClipboardSync(reason) {
    // copy/cut меняют системный clipboard default action-ом браузера/ОС.
    // Поэтому читаем не прямо внутри события, а чуть позже.
    setTimeout(() => {
      void pushLocalClipboardToAgent(reason).catch((err) => {
        setStatus("clipboard sync failed: " + err.message);
      });
    }, 25);
  }

  function onRDWindowActivated() {
    rdWindowActive = true;

    if (queuedRemoteClipboard) {
      const msg = queuedRemoteClipboard;
      queuedRemoteClipboard = null;
      void applyClipboardFromAgent(msg);
    }

    if (control && control.readyState === "open") {
      void pushLocalClipboardToAgent("window-activated").catch(() => {});
    }
  }

  function onRDWindowDeactivated() {
    rdWindowActive = false;
    setRDFocus(false);
  }

  video.addEventListener("pointerenter", () => {
    setRDFocus(true);
  });

  video.addEventListener("pointerleave", () => {
    setRDFocus(false);
  });

  video.addEventListener("pointerdown", (event) => {
    const p = normalizedPoint(event);
    if (!p.inside) {
      return;
    }

    video.setPointerCapture?.(event.pointerId);
    setRDFocus(true);

    event.preventDefault();

    sendControl({
      type: "mouse_down",
      x: p.x,
      y: p.y,
      button: buttonName(event.button),
    });
  });

  video.addEventListener("pointerup", (event) => {
    const p = normalizedPoint(event);

    event.preventDefault();

    sendControl({
      type: "mouse_up",
      x: p.x,
      y: p.y,
      button: buttonName(event.button),
    });
  });

  video.addEventListener("pointermove", (event) => {
    if (!rdFocused) {
      return;
    }

    const p = normalizedPoint(event);
    if (!p.inside) {
      return;
    }

    event.preventDefault();

    sendControl({
      type: "mouse_move",
      x: p.x,
      y: p.y,
    });
  });

  video.addEventListener("wheel", (event) => {
    if (!rdFocused) {
      return;
    }

    const p = normalizedPoint(event);
    if (!p.inside) {
      return;
    }

    event.preventDefault();

    sendControl({
      type: "mouse_wheel",
      x: p.x,
      y: p.y,
      delta_x: Math.trunc(event.deltaX),
      delta_y: Math.trunc(event.deltaY),
    });
  }, { passive: false });

  window.addEventListener("keydown", async (event) => {
    if (!rdFocused) {
      return;
    }

    event.preventDefault();

    if (event.repeat) {
      return;
    }

    // Важно: перед удалённым Ctrl+V сначала отправляем локальный clipboard в agent.
    // Иначе удалённое приложение может вставить старое содержимое буфера.
    if ((event.ctrlKey || event.metaKey) && event.code === "KeyV") {
      try {
        await pushLocalClipboardToAgent("paste-shortcut");
      } catch (err) {
        setStatus("clipboard sync before paste failed: " + err.message);
      }
    }

    pressedKeys.add(event.code);

    sendControl({
      type: "key_down",
      code: event.code,
      key: event.key,
      ctrl: event.ctrlKey,
      shift: event.shiftKey,
      alt: event.altKey,
      meta: event.metaKey,
      repeat: false,
    });
  });

  window.addEventListener("keyup", (event) => {
    const wasPressed = pressedKeys.has(event.code);

    if (!rdFocused && !wasPressed) {
      return;
    }

    event.preventDefault();

    pressedKeys.delete(event.code);

    sendControl({
      type: "key_up",
      code: event.code,
      key: event.key,
      ctrl: event.ctrlKey,
      shift: event.shiftKey,
      alt: event.altKey,
      meta: event.metaKey,
      repeat: false,
    });
  });

  window.addEventListener("copy", () => {
    scheduleLocalClipboardSync("copy");
  });

  window.addEventListener("cut", () => {
    scheduleLocalClipboardSync("cut");
  });

  window.addEventListener("paste", () => {
    scheduleLocalClipboardSync("paste");
  });

  if (navigator.clipboard && typeof navigator.clipboard.addEventListener === "function") {
    navigator.clipboard.addEventListener("clipboardchange", () => {
      if (!rdWindowActive) {
        return;
      }

      void pushLocalClipboardToAgent("clipboardchange").catch(() => {});
    });
  }

  window.addEventListener("focus", () => {
    onRDWindowActivated();
  });

  window.addEventListener("blur", () => {
    onRDWindowDeactivated();
  });

  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      onRDWindowDeactivated();
      return;
    }

    if (document.hasFocus()) {
      onRDWindowActivated();
    }
  });

  window.__RD_ON_SIGNAL = async function(raw) {
    const msg = typeof raw === "string" ? JSON.parse(raw) : raw;

    if (!pc) {
      return;
    }

    if (msg.type === "rd_answer" && msg.sdp) {
      await pc.setRemoteDescription({ type: "answer", sdp: msg.sdp });
      setStatus("remote answer applied");
      return;
    }

    if (msg.type === "rd_ice" && msg.candidate) {
      await pc.addIceCandidate(msg.candidate);
      return;
    }
  };

  async function startPeer() {
    pc = new RTCPeerConnection();

    control = pc.createDataChannel("control", {
      ordered: true,
    });

    control.onopen = () => {
      setStatus("control open");

      sendControl({
        type: "focus_changed",
        focused: false,
      });

      rdWindowActive = !document.hidden && document.hasFocus();

      if (rdWindowActive) {
        void pushLocalClipboardToAgent("control-open").catch(() => {});
      }

      // На старых agent-реализациях это оставляет совместимость:
      // если watcher ещё не добавлен, agent хотя бы вернёт snapshot по запросу.
      sendControl({
        type: "clipboard_get",
      });
    };

    control.onmessage = async (event) => {
      let msg;

      try {
        msg = JSON.parse(event.data);
      } catch (_) {
        return;
      }

      if (msg.type === "rd_agent_ready") {
        setStatus("agent ready");
        return;
      }

      if (msg.type === "clipboard_sync") {
        await applyClipboardFromAgent(msg);
        return;
      }

      if (msg.type === "clipboard_error") {
        setStatus("clipboard error: " + (msg.error || "unknown"));
        return;
      }
    };

    control.onclose = () => {
      releasePressedKeys();
      setStatus("control closed");
    };

    control.onerror = () => {
      releasePressedKeys();
      setStatus("control error");
    };

    pc.addTransceiver("video", { direction: "recvonly" });

    pc.onicecandidate = (event) => {
      if (!event.candidate) {
        return;
      }

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

      if (
        pc.connectionState === "failed" ||
        pc.connectionState === "disconnected" ||
        pc.connectionState === "closed"
      ) {
        releasePressedKeys();
        setRDFocus(false);
      }
    };

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);

    window.rdSignalOut(JSON.stringify({
      type: "rd_offer",
      sdp: offer.sdp,
    }));

    setStatus("offer sent");
  }

  video.addEventListener("contextmenu", (event) => {
    event.preventDefault();
  });

  video.addEventListener("loadedmetadata", reportVideoSize);

  setInterval(reportVideoSize, 500);

  window.addEventListener("beforeunload", () => {
    releasePressedKeys();

    try {
      sendControl({
        type: "focus_changed",
        focused: false,
      });
    } catch (_) {}

    try {
      window.rdSignalOut(JSON.stringify({
        type: "rd_stop",
      }));
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
