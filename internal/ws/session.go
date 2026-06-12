package ws

import (
	"bufio"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func StartCtrlCHandler(conn *websocket.Conn, clientID, adminID string) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT)

	go func() {
		for range sigChan {
			// НЕ завершаем админ
			// Отправляем спец-команду клиенту
			_ = conn.WriteJSON(Message{
				Type:     "control",
				ClientID: clientID,
				ID:       adminID,
				Command:  "CTRL_C",
			})
		}
	}()
}

func StartServerReader(conn *websocket.Conn, sessionClosed chan struct{}) {
	go func() {
		defer close(sessionClosed)

		for {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				fmt.Println("\nСоединение разорвано, нажмите Enter для продолжения")
				return
			}

			switch msg.Type {

			case "result":
				if out, ok := msg.Result["output"].(string); ok {
					fmt.Print(out)
				}

			case "session_closed":
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
	adminID string,
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
			ID:        adminID,
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
