package ws

import (
	"bufio"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"os"
	"os/signal"
	"posrelayd-viewer/internal/gui"
	"strings"
	"sync"
	"syscall"
	"time"
)

func StartCtrlCHandler(conn *websocket.Conn, clientID, sessionID string) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT)

	go func() {
		for range sigChan {
			// НЕ завершаем админ
			// Отправляем спец-команду клиенту
			_ = conn.WriteJSON(Message{
				Type:     "control",
				ClientID: clientID,
				ID:       sessionID,
				Command:  "CTRL_C",
			})
		}
	}()
}

func rdSessionID(msg Message, fallback string) string {
	if msg.SessionID != "" {
		return msg.SessionID
	}
	if msg.ID != "" {
		return msg.ID
	}
	return fallback
}

func StartServerReader(conn *websocket.Conn, sessionClosed chan struct{}, sessionID string) {
	go func() {
		defer close(sessionClosed)

		for {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				gui.CloseVideoStub(sessionID)
				fmt.Println("\nСоединение разорвано, нажмите Enter для продолжения")
				return
			}

			switch msg.Type {

			case "result":
				if out, ok := msg.Result["output"].(string); ok {
					fmt.Print(out)
				}

			case "rd_start":
				fmt.Printf("\n[RD] Принят ack на rd_start: session_id=%s expires_at=%s\n",
					rdSessionID(msg, sessionID), msg.ExpiresAt)

			case "rd_ready":
				readySessionID := rdSessionID(msg, sessionID)
				fmt.Printf("\n[RD] rd-agent зарегистрирован: session_id=%s client_id=%s\n",
					readySessionID, msg.ClientID)

				if err := gui.OpenVideoStub(readySessionID); err != nil {
					fmt.Printf("\n[RD] Не удалось открыть нативное окно: %v\n", err)
				}

			case "rd_closed":
				closedSessionID := rdSessionID(msg, sessionID)
				gui.CloseVideoStub(closedSessionID)
				fmt.Printf("\n[RD] Канал закрыт: session_id=%s reason=%s\n",
					closedSessionID, msg.Error)

			case "rd_error":
				errSessionID := rdSessionID(msg, sessionID)
				gui.CloseVideoStub(errSessionID)
				fmt.Printf("\n[RD] Ошибка: session_id=%s error=%s\n",
					errSessionID, msg.Error)

			case "session_closed":
				gui.CloseVideoStub(sessionID)
				fmt.Println("\nСессия клиента завершена, нажмите Enter для продолжения")
				return
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
	for {
		select {
		case <-sessionClosed:
			conn.Close()
			fmt.Println("\nПереподключение к серверу...\n")
			return
		default:
		}

		cmd, err := reader.ReadString('\n')
		if err != nil {
			continue
		}

		cmd = strings.TrimRight(cmd, "\r\n")
		if cmd == "" {
			continue
		}

		if err := conn.WriteJSON(Message{
			Type:      "command",
			ClientID:  clientID,
			CommandID: uuid.NewString(),
			Command:   cmd,
			ID:        sessionID,
		}); err != nil {
			// соединение умерло во время сессии
			conn.Close()
			fmt.Println("\nСоединение потеряно, переподключение...\n")
			return
		}
	}
}

func ConnectWithRetry(server string) *websocket.Conn {
	for {
		conn, resp, err := websocket.DefaultDialer.Dial(server, nil)
		if err != nil {

			// ПРОВЕРЯЕМ HTTP-ОТВЕТ
			if resp != nil && resp.StatusCode == 403 {
				fmt.Println("Подключение отклонено сервером: IP заблокирован")
				time.Sleep(30 * time.Second)
				os.Exit(1)
			}

			fmt.Println("Сервер недоступен, повторная попытка через 10 секунд...")
			time.Sleep(10 * time.Second)
			continue
		}

		fmt.Println("Соединение с сервером установлено")
		return conn
	}
}

func StartKeepAlive(conn *websocket.Conn, interval time.Duration) func() {
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
					return
				}

			case <-done:
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
