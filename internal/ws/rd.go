package ws

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"posrelayd-viewer/internal/gui"
	"posrelayd-viewer/internal/logger"
)

type RDViewer struct {
	conn      *websocket.Conn
	sessionID string
	clientID  string

	onWindowClosed func(sessionID string)

	closeOnce sync.Once
	closed    chan struct{}
}

func StartRDViewer(
	server string,
	apiKey string,
	sessionID string,
	clientID string,
	onWindowClosed func(sessionID string),
) (*RDViewer, error) {
	logger.Posrelayv.Debugf("[WS] Starting RD viewer: sessionID=%s, clientID=%s", sessionID, clientID)

	conn := ConnectWithRetry(server)

	if err := AdminHello(conn, apiKey); err != nil {
		logger.Posrelayv.Errorf("[WS] RD viewer admin_hello failed: sessionID=%s, clientID=%s, error=%v", sessionID, clientID, err)
		_ = conn.Close()
		return nil, fmt.Errorf("rd admin_hello failed: %w", err)
	}

	viewerID := uuid.NewString()
	logger.Posrelayv.Debugf("[WS] Registering RD viewer: viewerID=%s, sessionID=%s, clientID=%s", viewerID, sessionID, clientID)

	if err := conn.WriteJSON(Message{
		Type:      "rd_admin_register",
		ID:        viewerID,
		SessionID: sessionID,
		ClientID:  clientID,
		Target:    "agent",
	}); err != nil {
		logger.Posrelayv.Errorf("[WS] RD viewer registration failed: viewerID=%s, sessionID=%s, clientID=%s, error=%v", viewerID, sessionID, clientID, err)
		_ = conn.Close()
		return nil, fmt.Errorf("rd_admin_register failed: %w", err)
	}

	v := &RDViewer{
		conn:           conn,
		sessionID:      sessionID,
		clientID:       clientID,
		onWindowClosed: onWindowClosed,
		closed:         make(chan struct{}),
	}

	go v.readLoop()

	logger.Posrelayv.Infof("[WS] RD viewer started: viewerID=%s, sessionID=%s, clientID=%s", viewerID, sessionID, clientID)
	return v, nil
}

func (v *RDViewer) readLoop() {
	logger.Posrelayv.Debugf("[WS] RD viewer read loop started: sessionID=%s, clientID=%s", v.sessionID, v.clientID)

	defer func() {
		logger.Posrelayv.Debugf("[WS] RD viewer read loop stopped: sessionID=%s, clientID=%s", v.sessionID, v.clientID)
	}()
	defer close(v.closed)
	defer gui.CloseRDWindow(v.sessionID)
	defer v.conn.Close()

	for {
		var msg Message
		if err := v.conn.ReadJSON(&msg); err != nil {
			logger.Posrelayv.Warnf("[WS] RD viewer connection read failed: sessionID=%s, clientID=%s, error=%v", v.sessionID, v.clientID, err)
			return
		}

		switch msg.Type {
		case "rd_ready":
			readySessionID := rdSessionID(msg, v.sessionID)
			logger.Posrelayv.Infof("[WS] RD viewer received rd_ready: sessionID=%s, clientID=%s", readySessionID, v.clientID)

			if err := gui.OpenRDWindow(
				readySessionID,
				func(out gui.OutgoingSignal) error {
					err := v.conn.WriteJSON(Message{
						Type:      out.Type,
						ID:        readySessionID,
						SessionID: readySessionID,
						ClientID:  v.clientID,
						Target:    "agent",
						SDP:       out.SDP,
						Candidate: out.Candidate,
					})
					if err != nil {
						logger.Posrelayv.Warnf("[WS] Failed to send RD outgoing signal: type=%s, sessionID=%s, clientID=%s, error=%v", out.Type, readySessionID, v.clientID, err)
					}
					return err
				},
				v.onWindowClosed,
			); err != nil {
				logger.Posrelayv.Errorf("[WS] Failed to open RD window: sessionID=%s, clientID=%s, error=%v", readySessionID, v.clientID, err)
				fmt.Printf("\n[RD] Не удалось открыть RD окно: %v\n", err)
			}

		case "rd_answer", "rd_ice":
			sigSessionID := rdSessionID(msg, v.sessionID)
			logger.Posrelayv.Debugf("[WS] RD viewer received signal: type=%s, sessionID=%s, clientID=%s", msg.Type, sigSessionID, v.clientID)

			if err := gui.PushRDSignal(sigSessionID, msg); err != nil {
				logger.Posrelayv.Warnf("[WS] Failed to push RD signal to window: type=%s, sessionID=%s, clientID=%s, error=%v", msg.Type, sigSessionID, v.clientID, err)
				fmt.Printf("\n[RD] Не удалось передать signaling в окно: %v\n", err)
			}

		case "rd_closed":
			closedSessionID := rdSessionID(msg, v.sessionID)
			logger.Posrelayv.Infof("[WS] RD channel closed by server: sessionID=%s, clientID=%s, reason=%s", closedSessionID, v.clientID, msg.Error)

			fmt.Printf("\n[RD] Канал закрыт: session_id=%s reason=%s\n",
				closedSessionID, msg.Error)

			gui.CloseRDWindow(closedSessionID)
			return

		case "rd_error":
			errSessionID := rdSessionID(msg, v.sessionID)
			logger.Posrelayv.Warnf("[WS] RD error received: sessionID=%s, clientID=%s, error=%s", errSessionID, v.clientID, msg.Error)

			fmt.Printf("\n[RD] Ошибка: session_id=%s error=%s\n",
				errSessionID, msg.Error)

			gui.CloseRDWindow(errSessionID)
			return

		default:
			logger.Posrelayv.Debugf("[WS] RD viewer received unsupported message type: type=%s, sessionID=%s, clientID=%s", msg.Type, v.sessionID, v.clientID)
		}
	}
}

func (v *RDViewer) Close() {
	v.closeOnce.Do(func() {
		logger.Posrelayv.Debugf("[WS] Closing RD viewer: sessionID=%s, clientID=%s", v.sessionID, v.clientID)

		if err := v.conn.WriteJSON(Message{
			Type:      "rd_stop",
			ID:        v.sessionID,
			SessionID: v.sessionID,
			ClientID:  v.clientID,
			Target:    "agent",
		}); err != nil {
			logger.Posrelayv.Warnf("[WS] Failed to send rd_stop while closing RD viewer: sessionID=%s, clientID=%s, error=%v", v.sessionID, v.clientID, err)
		}

		_ = v.conn.Close()
		gui.CloseRDWindow(v.sessionID)
		logger.Posrelayv.Debugf("[WS] RD viewer closed: sessionID=%s, clientID=%s", v.sessionID, v.clientID)
	})
}
