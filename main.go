package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"posrelayd-viewer/config"
	"posrelayd-viewer/crypto"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type Message struct {
	Type      string                 `json:"type"`
	ClientID  string                 `json:"client_id,omitempty"`
	CommandID string                 `json:"command_id,omitempty"`
	Command   string                 `json:"command,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	Role      string                 `json:"role,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Password  string                 `json:"password,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ApiKey    string                 `json:"api_key,omitempty"`
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// fallback (не-TTY)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(text, "\r\n"), nil
}

// ===== АВТОРИЗАЦИЯ =====

func authLoop(conn *websocket.Conn, reader *bufio.Reader, adminID string) (string, error) {
	for {
		fmt.Print("Введите id-подключения: ")

		clientID, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		clientID = strings.TrimSpace(clientID)

		password, err := readPassword("Введите пароль: ")
		if err != nil {
			fmt.Println("Ошибка ввода пароля:", err)
			continue
		}

		if err := conn.WriteJSON(Message{
			Type:     "auth",
			ID:       adminID,
			ClientID: clientID,
			Password: password,
		}); err != nil {
			return "", err
		}

		var resp Message
		if err := conn.ReadJSON(&resp); err != nil {
			return "", err
		}

		if resp.Type == "auth_ok" {
			fmt.Println("Авторизация успешна")
			return clientID, nil
		}

		fmt.Println("Ошибка авторизации:", resp.Error)
	}
}

// ПОДКЛЮЧЕНИЕ С RETRY

func connectWithRetry(server string) *websocket.Conn {
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

func drainStdin(reader *bufio.Reader) {
	for reader.Buffered() > 0 {
		_, _ = reader.ReadString('\n')
	}
}

// ===== MAIN =====

func main() {
	setup := flag.Bool("setup", false, "configure application")
	flag.Parse()

	if *setup {
		if err := config.Setup(); err != nil {
			fmt.Println("Ошибка настройки:", err)
			os.Exit(1)
		}

		fmt.Println("Конфигурация успешно сохранена")
		return
	}

	server, ok := crypto.Decrypt(config.Cfg.Connection.Url)
	if !ok {
		fmt.Println("Не удалось расшифровать адрес сервера")
		return
	}

	apikey, ok := crypto.Decrypt(config.Cfg.Connection.APIKey)
	if !ok {
		fmt.Println("Не удалось расшифровать API ключ")
		return
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		// ---------- CONNECT ----------
		conn := connectWithRetry(server)

		if err := conn.WriteJSON(Message{
			Type:   "admin_hello",
			ApiKey: apikey,
		}); err != nil {
			fmt.Println("Ошибка отправки admin_hello:", err)
			conn.Close()
			continue
		}

		var helloResp Message
		if err := conn.ReadJSON(&helloResp); err != nil {
			fmt.Println("Соединение разорвано сервером")
			conn.Close()
			continue
		}

		if helloResp.Type == "error" {
			fmt.Println(helloResp.Error)
			conn.Close()

			fmt.Println("Повторная попытка через 10 секунд...")
			time.Sleep(10 * time.Second)

			continue
		}

		drainStdin(reader)

		adminID := uuid.NewString()

		// ---------- AUTH ----------
		clientID, err := authLoop(conn, reader, adminID)
		if err != nil {
			fmt.Println("Соединение потеряно во время авторизации\n")
			conn.Close()
			continue
		}

		_ = conn.WriteJSON(Message{
			Type: "register",
			Role: "admin",
			ID:   adminID,
		})

		sessionClosed := make(chan struct{})

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

		// ---------- SERVER READER ----------
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

		// ---------- SESSION LOOP ----------
		for {
			select {
			case <-sessionClosed:
				conn.Close()
				fmt.Println("\nПереподключение к серверу...\n")
				goto RECONNECT
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
				goto RECONNECT
			}
		}

	RECONNECT:
		continue
	}
}
