import { useEffect, useRef, useState } from "react";
import { RDIncomingSignal, sendSignalOut } from "../lib/bridge";

const MAX_CLIPBOARD_TEXT_BYTES = 60 * 1024;

const CONTROL_BUFFERED_LOW_BYTES = 64 * 1024;
const CONTROL_BUFFERED_HIGH_BYTES = 256 * 1024;

const MOTION_BUFFERED_LOW_BYTES = 8 * 1024;
const MOTION_BUFFERED_HIGH_BYTES = 32 * 1024;

export function useRDSession(sessionID: string) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const controlRef = useRef<RTCDataChannel | null>(null);
  const motionRef = useRef<RTCDataChannel | null>(null);

  const pendingRemoteIceRef = useRef<RTCIceCandidateInit[]>([]);
  const lastSizeRef = useRef("");
  const msgSeqRef = useRef(0);
  const clipboardSeqRef = useRef(0);

  const controlQueueRef = useRef<string[]>([]);
  const pendingMouseMoveRef = useRef<{ x: number; y: number } | null>(null);
  const mouseMoveRAFRef = useRef<number | null>(null);
  const motionBackpressureRef = useRef(false);

// Последний revision, записанный/прочитанный на стороне viewer.
// Используем только для защиты от повторной записи в host clipboard,
// но НЕ для отмены paste: при paste host clipboard всегда отправляется на remote.
  const lastHostClipboardRevisionRef = useRef("");

// Разрешаем принимать clipboard_sync от агента только как ответ
// на пользовательский copy/cut внутри RD-окна.
  const pendingRemoteClipboardPullRef = useRef(0);

  const rdFocusedRef = useRef(false);
  const rdWindowActiveRef = useRef(!document.hidden && document.hasFocus());
  const pressedKeysRef = useRef(new Set<string>());
  const browserOriginRef = useRef<string>(
    globalThis.crypto && typeof globalThis.crypto.randomUUID === "function"
      ? globalThis.crypto.randomUUID()
      : `browser-${Math.random().toString(16).slice(2)}-${Date.now()}`,
  );

  const [statusText, setStatusText] = useState<string>(`session_id: ${sessionID} | connecting...`);

  useEffect(() => {
    const videoElement = videoRef.current;
    if (videoElement === null) {
      return;
    }

    const video: HTMLVideoElement = videoElement;

    let disposed = false;

    function setStatus(text: string) {
      if (!disposed) {
        setStatusText(`session_id: ${sessionID} | ${text}`);
      }
    }

    function now() {
      return Date.now();
    }

    async function sha256Text(text: string) {
      const input = String(text ?? "");
      const data = new TextEncoder().encode(input);
      const browserCrypto = globalThis.crypto || null;

      if (
        browserCrypto &&
        browserCrypto.subtle &&
        typeof browserCrypto.subtle.digest === "function"
      ) {
        const digest = await browserCrypto.subtle.digest("SHA-256", data);
        return (
          "sha256:" +
          Array.from(new Uint8Array(digest))
            .map((value) => value.toString(16).padStart(2, "0"))
            .join("")
        );
      }

      let h = 2166136261;
      for (let index = 0; index < data.length; index += 1) {
        h ^= data[index];
        h = Math.imul(h, 16777619);
      }

      return `local-fnv1a:${data.length}:${(h >>> 0).toString(16).padStart(8, "0")}`;
    }

    function utf8Bytes(text: string) {
      return new TextEncoder().encode(String(text ?? "")).length;
    }

    function normalizeClipboardText(text: unknown) {
      const safe = String(text ?? "").replace(/\u0000/g, "");
      const size = utf8Bytes(safe);
      if (size > MAX_CLIPBOARD_TEXT_BYTES) {
        throw new Error(`clipboard payload too large: ${size} bytes`);
      }
      return safe;
    }

    function encodeControlMessage(msg: Record<string, unknown>) {
      return JSON.stringify({
        id: String(++msgSeqRef.current),
        session_id: sessionID,
        ts: now(),
        ...msg,
      });
    }

    function flushControlQueue() {
      const control = controlRef.current;
      if (!control || control.readyState !== "open") {
        return;
      }

      while (
          controlQueueRef.current.length > 0 &&
          control.bufferedAmount < CONTROL_BUFFERED_HIGH_BYTES
          ) {
        const raw = controlQueueRef.current.shift();
        if (!raw) {
          continue;
        }

        control.send(raw);
      }

      if (controlQueueRef.current.length > 0) {
        setStatus(
            `control backpressure: queued=${controlQueueRef.current.length} buffered=${control.bufferedAmount}`,
        );
      }
    }

    function sendControl(msg: Record<string, unknown>) {
      const control = controlRef.current;
      if (!control || control.readyState !== "open") {
        return false;
      }

      const raw = encodeControlMessage(msg);

      if (
          controlQueueRef.current.length > 0 ||
          control.bufferedAmount >= CONTROL_BUFFERED_HIGH_BYTES
      ) {
        controlQueueRef.current.push(raw);
        setStatus(
            `control queued: queued=${controlQueueRef.current.length} buffered=${control.bufferedAmount}`,
        );
        return true;
      }

      control.send(raw);
      return true;
    }

    function encodeMotionMessage(msg: Record<string, unknown>) {
      return JSON.stringify({
        id: String(++msgSeqRef.current),
        session_id: sessionID,
        ts: now(),
        ...msg,
      });
    }

    function sendMotion(msg: Record<string, unknown>) {
      const motion = motionRef.current;
      if (!motion || motion.readyState !== "open") {
        return false;
      }

      if (motion.bufferedAmount >= MOTION_BUFFERED_HIGH_BYTES) {
        motionBackpressureRef.current = true;
        return false;
      }

      motion.send(encodeMotionMessage(msg));
      return true;
    }

    function flushLatestMouseMove() {
      mouseMoveRAFRef.current = null;

      const point = pendingMouseMoveRef.current;
      if (!point) {
        return;
      }

      pendingMouseMoveRef.current = null;

      const sent = sendMotion({
        type: "mouse_move",
        x: point.x,
        y: point.y,
      });

      if (!sent) {
        // Не копим старую очередь move-событий.
        // Оставляем только последнюю позицию и попробуем отправить её позже.
        pendingMouseMoveRef.current = point;
      }
    }

    function scheduleMouseMove(point: { x: number; y: number }) {
      pendingMouseMoveRef.current = point;

      if (mouseMoveRAFRef.current !== null) {
        return;
      }

      mouseMoveRAFRef.current = window.requestAnimationFrame(flushLatestMouseMove);
    }

    async function addRemoteIceCandidate(candidate: RTCIceCandidateInit) {
      const pc = pcRef.current;
      if (!pc) {
        setStatus("remote ICE ignored: pc is not ready");
        return;
      }

      if (!candidate || !candidate.candidate) {
        return;
      }

      if (!pc.remoteDescription) {
        pendingRemoteIceRef.current.push(candidate);
        setStatus(`remote ICE queued: ${pendingRemoteIceRef.current.length}`);
        return;
      }

      try {
        await pc.addIceCandidate(candidate);
        setStatus("remote ICE added");
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setStatus(`addIceCandidate failed: ${message}`);
        console.error("[RD] addIceCandidate failed", {
          error,
          candidate,
          signalingState: pc.signalingState,
          iceConnectionState: pc.iceConnectionState,
          connectionState: pc.connectionState,
          hasRemoteDescription: Boolean(pc.remoteDescription),
        });
      }
    }

    async function flushPendingRemoteIce() {
      const pending = pendingRemoteIceRef.current.splice(0);

      for (const candidate of pending) {
        await addRemoteIceCandidate(candidate);
      }
    }

    function releasePressedKeys() {
      for (const code of Array.from(pressedKeysRef.current)) {
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

      pressedKeysRef.current.clear();
    }

    function setRDFocus(focused: boolean) {
      if (rdFocusedRef.current === focused) {
        return;
      }

      rdFocusedRef.current = focused;
      sendControl({ type: "focus_changed", focused });

      if (!focused) {
        releasePressedKeys();
      }
    }

    function reportVideoSize() {
      if (!video.videoWidth || !video.videoHeight) {
        return;
      }

      const key = `${video.videoWidth}x${video.videoHeight}`;
      if (key === lastSizeRef.current) {
        return;
      }

      lastSizeRef.current = key;
      const ratio = video.videoWidth / video.videoHeight;
      document.documentElement.style.setProperty("--stream-ratio", String(ratio));
      window.rdVideoMeta?.(video.videoWidth, video.videoHeight);
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

    function normalizedPoint(event: MouseEvent | PointerEvent | WheelEvent) {
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

    function buttonName(button: number) {
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

      if (navigator.clipboard?.readText) {
        return await navigator.clipboard.readText();
      }

      return "";
    }

    async function writeLocalClipboardText(text: string) {
      if (window.rdClipboardWrite) {
        return await window.rdClipboardWrite(text);
      }

      if (navigator.clipboard?.writeText) {
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

    async function sendHostClipboardToRemoteForPaste(reason: string) {
      const control = controlRef.current;
      if (!control || control.readyState !== "open") {
        return false;
      }

      const snap = await readLocalClipboardSnapshot();

      // ВАЖНО: для paste не делаем early return по revision.
      // Требование: при каждом paste в RD-окне буфер хоста должен быть отправлен на remote.
      lastHostClipboardRevisionRef.current = snap.revision;

      return sendControl({
        type: "clipboard_set",
        origin: browserOriginRef.current,
        seq: ++clipboardSeqRef.current,
        revision: snap.revision.startsWith("sha256:") ? snap.revision : "",
        text: snap.text,
        reason,
      });
    }

    function requestRemoteClipboardToHost(reason: string) {
      const control = controlRef.current;
      if (!control || control.readyState !== "open") {
        return false;
      }

      pendingRemoteClipboardPullRef.current += 1;

      const sent = sendControl({
        type: "clipboard_get",
        reason,
      });

      if (!sent) {
        pendingRemoteClipboardPullRef.current = Math.max(0, pendingRemoteClipboardPullRef.current - 1);
      }

      return sent;
    }

    function scheduleRemoteClipboardPull(reason: string) {
      window.setTimeout(() => {
        requestRemoteClipboardToHost(reason);
      }, 80);
    }

    async function applyRemoteClipboardToHostAfterCopyCut(msg: Record<string, unknown>) {
      if (!msg || msg.origin === browserOriginRef.current) {
        return;
      }

      const fromWatcher = isRemoteClipboardWatcherSync(msg);

      // Есть два легитимных пути remote -> host:
      // 1. Ответ на наш clipboard_get после Ctrl+C / Ctrl+X / browser copy/cut event.
      // 2. Самостоятельное событие от agent ClipboardWatcher после copy/cut через remote context menu.
      if (!fromWatcher) {
        if (pendingRemoteClipboardPullRef.current <= 0) {
          return;
        }

        pendingRemoteClipboardPullRef.current -= 1;
      }

      // Watcher-событие принимаем только когда RD реально активен в viewer.
      // Это дополнительная защита на стороне viewer; основная проверка focus уже есть на agent.
      if (fromWatcher && (!rdFocusedRef.current || !rdWindowActiveRef.current)) {
        return;
      }

      if (!("text" in msg)) {
        return;
      }

      let text: string;
      let revision: string;

      try {
        text = normalizeClipboardText(msg.text);
        revision = String(msg.revision || (await sha256Text(text)));
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setStatus(`clipboard payload rejected: ${message}`);
        return;
      }

      if (revision === lastHostClipboardRevisionRef.current) {
        return;
      }

      try {
        const ok = await writeLocalClipboardText(text);
        if (!ok) {
          setStatus("host clipboard write unavailable");
          return;
        }

        lastHostClipboardRevisionRef.current = revision;
        setStatus(`clipboard pulled from remote: ${reasonFromMessage(msg)}`);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setStatus(`host clipboard write failed: ${message}`);
      }
    }

    function reasonFromMessage(msg: Record<string, unknown>) {
      return typeof msg.reason === "string" ? msg.reason : "copy/cut";
    }

    function isRemoteClipboardWatcherSync(msg: Record<string, unknown>) {
      return msg.reason === "remote-clipboard-update";
    }

    function onRDWindowActivated() {
      rdWindowActiveRef.current = true;
    }

    function onRDWindowDeactivated() {
      rdWindowActiveRef.current = false;
      setRDFocus(false);
    }

    const onPointerEnter = () => {
      setRDFocus(true);
    };

    const onPointerLeave = () => {
      setRDFocus(false);
    };

    const onPointerDown = (event: PointerEvent) => {
      const point = normalizedPoint(event);
      if (!point.inside) {
        return;
      }

      video.setPointerCapture?.(event.pointerId);
      setRDFocus(true);
      event.preventDefault();

      if (event.button === 2) {
        void sendHostClipboardToRemoteForPaste("remote-context-menu-open").catch((error: unknown) => {
          const message = error instanceof Error ? error.message : String(error);
          setStatus(`clipboard preload before context menu failed: ${message}`);
        });
      }

      sendControl({
        type: "mouse_down",
        x: point.x,
        y: point.y,
        button: buttonName(event.button),
      });
    };

    const onPointerUp = (event: PointerEvent) => {
      const point = normalizedPoint(event);
      event.preventDefault();
      sendControl({
        type: "mouse_up",
        x: point.x,
        y: point.y,
        button: buttonName(event.button),
      });
    };

    const onPointerMove = (event: PointerEvent) => {
      if (!rdFocusedRef.current) {
        return;
      }

      const point = normalizedPoint(event);
      if (!point.inside) {
        return;
      }

      event.preventDefault();
      scheduleMouseMove({
        x: point.x,
        y: point.y,
      });
    };

    const onWheel = (event: WheelEvent) => {
      if (!rdFocusedRef.current) {
        return;
      }

      const point = normalizedPoint(event);
      if (!point.inside) {
        return;
      }

      event.preventDefault();
      sendControl({
        type: "mouse_wheel",
        x: point.x,
        y: point.y,
        delta_x: Math.trunc(event.deltaX),
        delta_y: Math.trunc(event.deltaY),
      });
    };

    const onKeyDown = async (event: KeyboardEvent) => {
      if (!rdFocusedRef.current) {
        return;
      }

      event.preventDefault();

      if (event.repeat) {
        return;
      }

      const isClipboardShortcut = event.ctrlKey || event.metaKey;
      const isPasteShortcut = isClipboardShortcut && event.code === "KeyV";
      const isCopyShortcut = isClipboardShortcut && event.code === "KeyC";
      const isCutShortcut = isClipboardShortcut && event.code === "KeyX";

      if (isPasteShortcut) {
        try {
          await sendHostClipboardToRemoteForPaste("paste-shortcut");
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          setStatus(`clipboard send before paste failed: ${message}`);
        }
      }

      pressedKeysRef.current.add(event.code);

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

      if (isCopyShortcut) {
        scheduleRemoteClipboardPull("copy-shortcut");
      }

      if (isCutShortcut) {
        scheduleRemoteClipboardPull("cut-shortcut");
      }
    };

    const onKeyUp = (event: KeyboardEvent) => {
      const wasPressed = pressedKeysRef.current.has(event.code);
      if (!rdFocusedRef.current && !wasPressed) {
        return;
      }

      event.preventDefault();
      pressedKeysRef.current.delete(event.code);
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
    };

    const onCopy = () => {
      if (!rdFocusedRef.current) {
        return;
      }

      scheduleRemoteClipboardPull("copy-event");
    };

    const onCut = () => {
      if (!rdFocusedRef.current) {
        return;
      }

      scheduleRemoteClipboardPull("cut-event");
    };

    const onPaste = () => {
      if (!rdFocusedRef.current) {
        return;
      }

      void sendHostClipboardToRemoteForPaste("paste-event").catch((error) => {
        const message = error instanceof Error ? error.message : String(error);
        setStatus(`clipboard send on paste failed: ${message}`);
      });
    };

    const onFocus = () => onRDWindowActivated();
    const onBlur = () => onRDWindowDeactivated();
    const onVisibilityChange = () => {
      if (document.hidden) {
        onRDWindowDeactivated();
        return;
      }
      if (document.hasFocus()) {
        onRDWindowActivated();
      }
    };

    const onContextMenu = (event: Event) => {
      event.preventDefault();

      if (!rdFocusedRef.current) {
        return;
      }

      void sendHostClipboardToRemoteForPaste("remote-context-menu").catch((error: unknown) => {
        const message = error instanceof Error ? error.message : String(error);
        setStatus(`clipboard preload on context menu failed: ${message}`);
      });
    };

    const onBeforeUnload = () => {
      releasePressedKeys();
      try {
        sendControl({ type: "focus_changed", focused: false });
      } catch {
        // ignore
      }
      try {
        sendSignalOut({ type: "rd_stop" });
      } catch {
        // ignore
      }
    };

    video.addEventListener("pointerenter", onPointerEnter);
    video.addEventListener("pointerleave", onPointerLeave);
    video.addEventListener("pointerdown", onPointerDown);
    video.addEventListener("pointerup", onPointerUp);
    video.addEventListener("pointermove", onPointerMove);
    video.addEventListener("wheel", onWheel, { passive: false });
    video.addEventListener("contextmenu", onContextMenu);
    video.addEventListener("loadedmetadata", reportVideoSize);

    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
    window.addEventListener("copy", onCopy);
    window.addEventListener("cut", onCut);
    window.addEventListener("paste", onPaste);
    window.addEventListener("focus", onFocus);
    window.addEventListener("blur", onBlur);
    window.addEventListener("beforeunload", onBeforeUnload);
    document.addEventListener("visibilitychange", onVisibilityChange);

    window.__RD_ON_SIGNAL = async (raw: string | RDIncomingSignal) => {
      const msg = typeof raw === "string" ? (JSON.parse(raw) as RDIncomingSignal) : raw;
      const pc = pcRef.current;

      if (!pc) {
        setStatus(`signal ignored: pc is not ready, type=${msg.type}`);
        return;
      }

      if (msg.type === "rd_answer" && msg.sdp) {
        try {
          await pc.setRemoteDescription({ type: "answer", sdp: msg.sdp });
          setStatus("remote answer applied");

          await flushPendingRemoteIce();
          return;
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          setStatus(`setRemoteDescription failed: ${message}`);
          console.error("[RD] setRemoteDescription failed", {
            error,
            signalingState: pc.signalingState,
            iceConnectionState: pc.iceConnectionState,
            connectionState: pc.connectionState,
          });
          return;
        }
      }

      if (msg.type === "rd_ice" && msg.candidate) {
        await addRemoteIceCandidate(msg.candidate);
        return;
      }

      if (msg.type === "rd_closed") {
        releasePressedKeys();
        setRDFocus(false);
        controlRef.current = null;
        motionRef.current = null;
        pc.close();
        setStatus(`pc closed by signal: ${msg.type}`);
      }
    };

    async function startPeer() {
      const pc = new RTCPeerConnection();
      pcRef.current = pc;

      const control = pc.createDataChannel("control", {
        ordered: true,
      });

      control.bufferedAmountLowThreshold = CONTROL_BUFFERED_LOW_BYTES;
      controlRef.current = control;

      const motion = pc.createDataChannel("motion", {
        ordered: false,
        maxRetransmits: 0,
      });

      motion.bufferedAmountLowThreshold = MOTION_BUFFERED_LOW_BYTES;
      motionRef.current = motion;

      control.onopen = () => {
        setStatus("control open");
        flushControlQueue();
        sendControl({ type: "focus_changed", focused: false });
        rdWindowActiveRef.current = !document.hidden && document.hasFocus();
      };

      control.onbufferedamountlow = () => {
        flushControlQueue();
      };

      control.onmessage = async (event) => {
        let msg: Record<string, unknown>;
        try {
          msg = JSON.parse(event.data) as Record<string, unknown>;
        } catch {
          return;
        }

        if (msg.type === "rd_agent_ready") {
          setStatus("agent ready");
          return;
        }

        if (msg.type === "clipboard_sync") {
          await applyRemoteClipboardToHostAfterCopyCut(msg);
          return;
        }

        if (msg.type === "clipboard_error") {
          setStatus(`clipboard error: ${String(msg.error || "unknown")}`);
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

      motion.onopen = () => {
        setStatus("motion open");
      };

      motion.onbufferedamountlow = () => {
        motionBackpressureRef.current = false;

        if (pendingMouseMoveRef.current && mouseMoveRAFRef.current === null) {
          mouseMoveRAFRef.current = window.requestAnimationFrame(flushLatestMouseMove);
        }
      };

      motion.onclose = () => {
        motionBackpressureRef.current = false;
      };

      motion.onerror = () => {
        motionBackpressureRef.current = false;
      };

      pc.addTransceiver("video", { direction: "recvonly" });

      pc.onicecandidate = (event) => {
        if (!event.candidate) {
          console.debug("[RD] local ICE gathering complete");
          return;
        }

        const candidate = event.candidate.toJSON();

        console.debug("[RD] local ICE candidate", {
          candidate: candidate.candidate,
          sdpMid: candidate.sdpMid,
          sdpMLineIndex: candidate.sdpMLineIndex,
        });

        sendSignalOut({
          type: "rd_ice",
          candidate,
        });
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
        setStatus(`pc=${pc.connectionState}`);
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
      sendSignalOut({
        type: "rd_offer",
        sdp: offer.sdp || "",
      });
      setStatus("offer sent");
    }

    const reportVideoSizeInterval = window.setInterval(reportVideoSize, 500);

    void startPeer().catch((error) => {
      const message = error instanceof Error ? error.message : String(error);
      setStatus(`error: ${message}`);
    });

    return () => {
      disposed = true;
      pendingRemoteIceRef.current = [];
      controlQueueRef.current = [];
      pendingMouseMoveRef.current = null;

      if (mouseMoveRAFRef.current !== null) {
        window.cancelAnimationFrame(mouseMoveRAFRef.current);
        mouseMoveRAFRef.current = null;
      }

      window.clearInterval(reportVideoSizeInterval);
      releasePressedKeys();

      try {
        sendControl({ type: "focus_changed", focused: false });
      } catch {
        // ignore
      }

      try {
        sendSignalOut({ type: "rd_stop" });
      } catch {
        // ignore
      }

      delete window.__RD_ON_SIGNAL;

      video.removeEventListener("pointerenter", onPointerEnter);
      video.removeEventListener("pointerleave", onPointerLeave);
      video.removeEventListener("pointerdown", onPointerDown);
      video.removeEventListener("pointerup", onPointerUp);
      video.removeEventListener("pointermove", onPointerMove);
      video.removeEventListener("wheel", onWheel);
      video.removeEventListener("contextmenu", onContextMenu);
      video.removeEventListener("loadedmetadata", reportVideoSize);
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("keyup", onKeyUp);
      window.removeEventListener("copy", onCopy);
      window.removeEventListener("cut", onCut);
      window.removeEventListener("paste", onPaste);
      window.removeEventListener("focus", onFocus);
      window.removeEventListener("blur", onBlur);
      window.removeEventListener("beforeunload", onBeforeUnload);
      document.removeEventListener("visibilitychange", onVisibilityChange);

      controlRef.current?.close();
      controlRef.current = null;
      pcRef.current?.close();
      pcRef.current = null;
    };
  }, [sessionID]);

  return {
    statusText,
    videoRef,
  };
}
