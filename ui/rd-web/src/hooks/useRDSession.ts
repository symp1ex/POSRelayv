import { useEffect, useRef, useState } from "react";
import { RDIncomingSignal, sendSignalOut } from "../lib/bridge";

const MAX_CLIPBOARD_TEXT_BYTES = 60 * 1024;

export function useRDSession(sessionID: string) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const controlRef = useRef<RTCDataChannel | null>(null);
  const pendingRemoteIceRef = useRef<RTCIceCandidateInit[]>([]);
  const lastSizeRef = useRef("");
  const msgSeqRef = useRef(0);
  const clipboardSeqRef = useRef(0);
  const lastClipboardRevisionRef = useRef("");
  const queuedRemoteClipboardRef = useRef<Record<string, unknown> | null>(null);
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

    function sendControl(msg: Record<string, unknown>) {
      const control = controlRef.current;
      if (!control || control.readyState !== "open") {
        return false;
      }

      if (control.bufferedAmount > 256 * 1024) {
        setStatus(`control backpressure: ${control.bufferedAmount}`);
        return false;
      }

      control.send(
        JSON.stringify({
          id: String(++msgSeqRef.current),
          session_id: sessionID,
          ts: now(),
          ...msg,
        }),
      );

      return true;
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

    async function pushLocalClipboardToAgent(reason = "local-event") {
      const control = controlRef.current;
      if (!control || control.readyState !== "open") {
        return false;
      }

      const snap = await readLocalClipboardSnapshot();
      if (snap.revision === lastClipboardRevisionRef.current) {
        return false;
      }

      lastClipboardRevisionRef.current = snap.revision;
      return sendControl({
        type: "clipboard_sync",
        origin: browserOriginRef.current,
        seq: ++clipboardSeqRef.current,
        revision: snap.revision.startsWith("sha256:") ? snap.revision : "",
        text: snap.text,
        reason,
      });
    }

    async function applyClipboardFromAgent(msg: Record<string, unknown>) {
      if (!msg || msg.origin === browserOriginRef.current) {
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

      if (revision === lastClipboardRevisionRef.current) {
        return;
      }

      if (!rdWindowActiveRef.current && !window.rdClipboardWrite) {
        queuedRemoteClipboardRef.current = { ...msg, text, revision };
        return;
      }

      try {
        const ok = await writeLocalClipboardText(text);
        if (!ok) {
          queuedRemoteClipboardRef.current = { ...msg, text, revision };
          setStatus("clipboard write unavailable");
          return;
        }

        lastClipboardRevisionRef.current = revision;
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        queuedRemoteClipboardRef.current = { ...msg, text, revision };
        setStatus(`clipboard write failed: ${message}`);
      }
    }

    function scheduleLocalClipboardSync(reason: string) {
      window.setTimeout(() => {
        void pushLocalClipboardToAgent(reason).catch((error) => {
          const message = error instanceof Error ? error.message : String(error);
          setStatus(`clipboard sync failed: ${message}`);
        });
      }, 25);
    }

    function onRDWindowActivated() {
      rdWindowActiveRef.current = true;

      if (queuedRemoteClipboardRef.current) {
        const queued = queuedRemoteClipboardRef.current;
        queuedRemoteClipboardRef.current = null;
        void applyClipboardFromAgent(queued);
      }

      const control = controlRef.current;
      if (control && control.readyState === "open") {
        void pushLocalClipboardToAgent("window-activated").catch(() => undefined);
      }
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
      sendControl({
        type: "mouse_move",
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

      if ((event.ctrlKey || event.metaKey) && event.code === "KeyV") {
        try {
          await pushLocalClipboardToAgent("paste-shortcut");
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          setStatus(`clipboard sync before paste failed: ${message}`);
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

    const onCopy = () => scheduleLocalClipboardSync("copy");
    const onCut = () => scheduleLocalClipboardSync("cut");
    const onPaste = () => scheduleLocalClipboardSync("paste");
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

    const clipboardWithEvents = navigator.clipboard as Clipboard & EventTarget;
    const onClipboardChange = () => {
      if (!rdWindowActiveRef.current) {
        return;
      }
      void pushLocalClipboardToAgent("clipboardchange").catch(() => undefined);
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

    if (typeof clipboardWithEvents?.addEventListener === "function") {
      clipboardWithEvents.addEventListener("clipboardchange", onClipboardChange as EventListener);
    }

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
        pc.close();
        setStatus(`pc closed by signal: ${msg.type}`);
      }
    };

    async function startPeer() {
      const pc = new RTCPeerConnection();
      pcRef.current = pc;

      const control = pc.createDataChannel("control", { ordered: true });
      controlRef.current = control;

      control.onopen = () => {
        setStatus("control open");
        sendControl({ type: "focus_changed", focused: false });
        rdWindowActiveRef.current = !document.hidden && document.hasFocus();

        if (rdWindowActiveRef.current) {
          void pushLocalClipboardToAgent("control-open").catch(() => undefined);
        }

        sendControl({ type: "clipboard_get" });
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
          await applyClipboardFromAgent(msg);
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

      if (typeof clipboardWithEvents?.removeEventListener === "function") {
        clipboardWithEvents.removeEventListener("clipboardchange", onClipboardChange as EventListener);
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
