package ws

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"posrelayd-viewer/internal/gui"
)

type RDViewer struct {
	conn      *websocket.Conn
	sessionID string
	clientID  string

	closeOnce sync.Once
	closed    chan struct{}
}

func StartRDViewer(server string, apiKey string, sessionID string, clientID string) (*RDViewer, error) {
	conn := ConnectWithRetry(server)

	if err := AdminHello(conn, apiKey); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rd admin_hello failed: %w", err)
	}

	viewerID := uuid.NewString()

	if err := conn.WriteJSON(Message{
		Type:      "rd_admin_register",
		ID:        viewerID,
		SessionID: sessionID,
		ClientID:  clientID,
		Target:    "agent",
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rd_admin_register failed: %w", err)
	}

	v := &RDViewer{
		conn:      conn,
		sessionID: sessionID,
		clientID:  clientID,
		closed:    make(chan struct{}),
	}

	go v.readLoop()

	return v, nil
}

func (v *RDViewer) readLoop() {
	defer close(v.closed)
	defer gui.CloseRDWindow(v.sessionID)
	defer v.conn.Close()

	for {
		var msg Message
		if err := v.conn.ReadJSON(&msg); err != nil {
			return
		}

		switch msg.Type {
		case "rd_ready":
			readySessionID := rdSessionID(msg, v.sessionID)

			if err := gui.OpenRDWindow(readySessionID, func(out gui.OutgoingSignal) error {
				return v.conn.WriteJSON(Message{
					Type:      out.Type,
					ID:        readySessionID,
					SessionID: readySessionID,
					ClientID:  v.clientID,
					Target:    "agent",
					SDP:       out.SDP,
					Candidate: out.Candidate,
				})
			}); err != nil {
				fmt.Printf("\n[RD] Не удалось открыть RD окно: %v\n", err)
			}

		case "rd_answer", "rd_ice":
			sigSessionID := rdSessionID(msg, v.sessionID)

			if err := gui.PushRDSignal(sigSessionID, msg); err != nil {
				fmt.Printf("\n[RD] Не удалось передать signaling в окно: %v\n", err)
			}

		case "rd_closed":
			closedSessionID := rdSessionID(msg, v.sessionID)

			fmt.Printf("\n[RD] Канал закрыт: session_id=%s reason=%s\n",
				closedSessionID, msg.Error)

			gui.CloseRDWindow(closedSessionID)
			return

		case "rd_error":
			errSessionID := rdSessionID(msg, v.sessionID)

			fmt.Printf("\n[RD] Ошибка: session_id=%s error=%s\n",
				errSessionID, msg.Error)

			gui.CloseRDWindow(errSessionID)
			return
		}
	}
}

func (v *RDViewer) Close() {
	v.closeOnce.Do(func() {
		_ = v.conn.WriteJSON(Message{
			Type:      "rd_stop",
			ID:        v.sessionID,
			SessionID: v.sessionID,
			ClientID:  v.clientID,
			Target:    "agent",
		})

		_ = v.conn.Close()
		gui.CloseRDWindow(v.sessionID)
	})
}
