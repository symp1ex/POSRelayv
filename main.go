package main

import (
	"bufio"
	"fmt"
	"os"

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
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	// Если stdin — терминал, читаем скрыто
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// Иначе (IDE, debug, pipe) — обычный ввод
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return text[:len(text)-1], nil
}

func main() {
	server := "ws://10.127.33.42:22233/ws"

	var clientID string

	fmt.Print("Введите id-подключения: ")
	fmt.Scan(&clientID)

	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		panic(err)
	}

	adminID := uuid.NewString()

	// === AUTH LOOP ===
	for {
		password, err := readPassword("Введите пароль: ")
		if err != nil {
			fmt.Println("Ошибка ввода пароля:", err)
			continue
		}

		conn.WriteJSON(Message{
			Type:     "auth",
			ClientID: clientID,
			Password: password,
		})

		var authResp Message
		if err := conn.ReadJSON(&authResp); err != nil {
			fmt.Println("Ошибка ответа сервера:", err)
			continue
		}

		if authResp.Type == "auth_ok" {
			fmt.Println("Авторизация успешна")
			break
		}

		fmt.Println("Ошибка авторизации:", authResp.Error)
	}

	// === REGISTER ===
	conn.WriteJSON(Message{
		Type: "register",
		Role: "admin",
		ID:   adminID,
	})

	go func() {
		for {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				fmt.Println("Ошибка соединения:", err)
				return
			}

			switch msg.Type {

			case "interactive_prompt":
				fmt.Print(msg.Prompt + " ")
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')

				conn.WriteJSON(Message{
					Type:      "interactive_response",
					CommandID: msg.CommandID,
					Command:   answer[:len(answer)-1],
				})

			case "result":
				fmt.Println("\n=== OUTPUT ===")
				fmt.Println(msg.Result["output"])
				fmt.Println(msg.Result["prompt"])
			}
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		cmd, _ := reader.ReadString('\n')
		cmd = cmd[:len(cmd)-1]

		conn.WriteJSON(Message{
			Type:      "command",
			ClientID:  clientID,
			CommandID: uuid.NewString(),
			Command:   cmd,
		})
	}
}
