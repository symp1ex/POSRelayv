package ws

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"posrelayd-viewer/internal/gui"
	"posrelayd-viewer/internal/logger"
)

func rdSessionID(msg Message, fallback string) string {
	if msg.SessionID != "" {
		return msg.SessionID
	}
	if msg.ID != "" {
		return msg.ID
	}
	return fallback
}

func StartServerReader(
	conn *websocket.Conn,
	sessionClosed chan struct{},
	sessionID string,
	clientID string,
	server string,
	apiKey string,
	autoReconnect bool,
) {
	logger.Posrelayv.Debugf("[ws] Starting server reader: sessionID=%s, clientID=%s, autoReconnect=%t", sessionID, clientID, autoReconnect)

	go func() {
		var rdViewer *RDViewer
		var closeSessionOnce sync.Once

		closeSessionByRDWindow := func(closedSessionID string) {
			closeSessionOnce.Do(func() {
				logger.Posrelayv.Infof("[ws] RD window closed by user, closing session connection: sessionID=%s, clientID=%s", closedSessionID, clientID)
				fmt.Printf("\n[RD] RD окно закрыто пользователем, завершаю сессию: session_id=%s\n",
					closedSessionID)

				_ = conn.Close()
			})
		}

		defer func() {
			logger.Posrelayv.Debugf("[ws] Server reader stopped: sessionID=%s, clientID=%s", sessionID, clientID)
			close(sessionClosed)
		}()

		for {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				logger.Posrelayv.Warnf("[ws] Server connection read failed: sessionID=%s, clientID=%s, error=%v", sessionID, clientID, err)
				gui.CloseRDWindow(sessionID)

				if autoReconnect {
					fmt.Println("\nСоединение разорвано, переподключение...")
				} else {
					fmt.Println("\nСоединение разорвано, нажмите Enter для продолжения")
				}

				return
			}

			switch msg.Type {

			case "result":
				if out, ok := msg.Result["output"].(string); ok {
					logger.Posrelayv.Debugf("[ws] Command result received: sessionID=%s, clientID=%s, outputLength=%d", sessionID, clientID, len(out))
					fmt.Print(out)
				} else {
					logger.Posrelayv.Debugf("[ws] Result message received without string output: sessionID=%s, clientID=%s", sessionID, clientID)
				}

			case "rd_start":
				receivedSessionID := rdSessionID(msg, sessionID)
				logger.Posrelayv.Infof("[ws] RD start acknowledged: sessionID=%s, clientID=%s, expiresAt=%s", receivedSessionID, clientID, msg.ExpiresAt)
				fmt.Printf("\n[RD] Принят ack на rd_start: session_id=%s expires_at=%s\n",
					receivedSessionID, msg.ExpiresAt)

			case "rd_ready":
				readySessionID := rdSessionID(msg, sessionID)
				logger.Posrelayv.Infof("[ws] RD agent ready: sessionID=%s, clientID=%s", readySessionID, msg.ClientID)
				fmt.Printf("\n[RD] rd-agent зарегистрирован: session_id=%s client_id=%s\n",
					readySessionID, msg.ClientID)

				if rdViewer != nil {
					logger.Posrelayv.Debugf("[ws] RD viewer already exists, duplicate rd_ready ignored: sessionID=%s, clientID=%s", readySessionID, clientID)
					continue
				}

				v, err := StartRDViewer(
					server,
					apiKey,
					readySessionID,
					clientID,
					closeSessionByRDWindow,
				)
				if err != nil {
					logger.Posrelayv.Errorf("[ws] Failed to start RD viewer: sessionID=%s, clientID=%s, error=%v", readySessionID, clientID, err)
					fmt.Printf("\n[RD] Не удалось запустить RD viewer: %v\n", err)
					continue
				}

				rdViewer = v
				logger.Posrelayv.Infof("[ws] RD viewer registered as rd_admin: sessionID=%s, clientID=%s", readySessionID, clientID)
				fmt.Printf("\n[RD] RD viewer зарегистрирован как rd_admin: session_id=%s\n", readySessionID)

			case "rd_closed":
				closedSessionID := rdSessionID(msg, sessionID)
				logger.Posrelayv.Infof("[ws] RD channel closed: sessionID=%s, clientID=%s, reason=%s", closedSessionID, clientID, msg.Error)

				if rdViewer != nil {
					rdViewer.Close()
					rdViewer = nil
				}

				gui.CloseRDWindow(closedSessionID)
				fmt.Printf("\n[RD] Канал закрыт: session_id=%s reason=%s\n",
					closedSessionID, msg.Error)

			case "rd_error":
				errSessionID := rdSessionID(msg, sessionID)
				logger.Posrelayv.Warnf("[ws] RD error received: sessionID=%s, clientID=%s, error=%s", errSessionID, clientID, msg.Error)

				if rdViewer != nil {
					rdViewer.Close()
					rdViewer = nil
				}

				gui.CloseRDWindow(errSessionID)
				fmt.Printf("\n[RD] Ошибка: session_id=%s error=%s\n",
					errSessionID, msg.Error)

			case "session_closed":
				logger.Posrelayv.Infof("[ws] Session closed by server: sessionID=%s, clientID=%s", sessionID, clientID)

				if rdViewer != nil {
					rdViewer.Close()
					rdViewer = nil
				}

				gui.CloseRDWindow(sessionID)

				if autoReconnect {
					fmt.Println("\nСессия клиента завершена, переподключение...")
				} else {
					fmt.Println("\nСессия клиента завершена, нажмите Enter для продолжения")
				}

				return

			default:
				logger.Posrelayv.Debugf("[ws] Unsupported server message type received: type=%s, sessionID=%s, clientID=%s", msg.Type, sessionID, clientID)
			}
		}
	}()
}

func RunSessionLoop(
	conn *websocket.Conn,
	reader *bufio.Reader,
	sessionClosed chan struct{},
	clientID string,
	sessionID string,
) {
	logger.Posrelayv.Debugf("[ws] Starting interactive command loop: sessionID=%s, clientID=%s", sessionID, clientID)

	for {
		select {
		case <-sessionClosed:
			logger.Posrelayv.Infof("[ws] Session closed, stopping interactive command loop: sessionID=%s, clientID=%s", sessionID, clientID)
			conn.Close()
			fmt.Println("\nПереподключение к серверу...\n")
			return
		default:
		}

		cmd, err := reader.ReadString('\n')
		if err != nil {
			logger.Posrelayv.Warnf("[ws] Failed to read interactive command: sessionID=%s, clientID=%s, error=%v", sessionID, clientID, err)
			continue
		}

		cmd = strings.TrimRight(cmd, "\r\n")
		if cmd == "" {
			continue
		}

		commandID := uuid.NewString()
		if err := conn.WriteJSON(Message{
			Type:      "command",
			ClientID:  clientID,
			CommandID: commandID,
			Command:   cmd,
			ID:        sessionID,
		}); err != nil {
			logger.Posrelayv.Warnf("[ws] Failed to send interactive command: sessionID=%s, clientID=%s, commandID=%s, error=%v", sessionID, clientID, commandID, err)
			// соединение умерло во время сессии
			conn.Close()
			fmt.Println("\nСоединение потеряно, переподключение...\n")
			return
		}

		logger.Posrelayv.Debugf("[ws] Interactive command sent: sessionID=%s, clientID=%s, commandID=%s, length=%d", sessionID, clientID, commandID, len(cmd))
	}
}

func ConnectWithRetry(server string) *websocket.Conn {
	attempt := 0

	for {
		attempt++
		logger.Posrelayv.Infof("[ws] Connecting to WebSocket server: attempt=%d", attempt)

		conn, resp, err := websocket.DefaultDialer.Dial(server, nil)
		if err != nil {

			// ПРОВЕРЯЕМ HTTP-ОТВЕТ
			if resp != nil && resp.StatusCode == 403 {
				logger.Posrelayv.Error("[ws] WebSocket connection rejected: status=403, reason=IP blocked")
				fmt.Println("Подключение отклонено сервером: IP заблокирован")
				time.Sleep(30 * time.Second)
				os.Exit(1)
			}

			if resp != nil {
				logger.Posrelayv.Warnf("[ws] WebSocket connection failed: attempt=%d, status=%d, error=%v", attempt, resp.StatusCode, err)
			} else {
				logger.Posrelayv.Warnf("[ws] WebSocket connection failed: attempt=%d, error=%v", attempt, err)
			}

			fmt.Println("Сервер недоступен, повторная попытка через 10 секунд...")
			time.Sleep(10 * time.Second)
			continue
		}

		logger.Posrelayv.Info("[ws] WebSocket connection established")
		fmt.Println("Соединение с сервером установлено")
		return conn
	}
}

func StartKeepAlive(conn *websocket.Conn, interval time.Duration) func() {
	logger.Posrelayv.Debugf("[ws] Starting keep-alive: interval=%s", interval)

	done := make(chan struct{})
	var once sync.Once

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := conn.WriteControl(
					websocket.PingMessage,
					[]byte("admin-keepalive"),
					time.Now().Add(5*time.Second),
				)
				if err != nil {
					logger.Posrelayv.Warnf("[ws] Keep-alive ping failed: %v", err)
					return
				}

			case <-done:
				logger.Posrelayv.Debug("[ws] Keep-alive stopped")
				return
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
		})
	}
}

func WaitSessionClosed(conn *websocket.Conn, sessionClosed chan struct{}) {
	logger.Posrelayv.Debug("[ws] Waiting for session close")
	<-sessionClosed
	logger.Posrelayv.Debug("[ws] Session closed, closing connection")
	conn.Close()
}
