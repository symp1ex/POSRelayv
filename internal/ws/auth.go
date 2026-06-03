package ws

import (
	"bufio"
	"fmt"
	"github.com/gorilla/websocket"
	"posrelayd-viewer/internal/console"
	"strings"
)

func AuthLoop(conn *websocket.Conn, reader *bufio.Reader, adminID string) (string, error) {
	for {
		fmt.Print("Введите id-подключения: ")

		clientID, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		clientID = strings.TrimSpace(clientID)

		password, err := console.ReadPassword("Введите пароль: ")
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

			if resp.ClientID != "" {
				return resp.ClientID, nil
			}

			return clientID, nil
		}

		fmt.Println("Ошибка авторизации:", resp.Error)
	}
}

func AdminHello(conn *websocket.Conn, apiKey string) error {
	if err := conn.WriteJSON(Message{
		Type:   "admin_hello",
		ApiKey: apiKey,
	}); err != nil {
		fmt.Println("Ошибка отправки admin_hello:", err)
		return err
	}

	var helloResp Message
	if err := conn.ReadJSON(&helloResp); err != nil {
		fmt.Println("Соединение разорвано сервером")
		return err
	}

	if helloResp.Type == "error" {
		return fmt.Errorf(helloResp.Error)
	}

	return nil
}
