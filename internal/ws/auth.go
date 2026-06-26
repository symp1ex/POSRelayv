package ws

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/gorilla/websocket"

	"posrelayd-viewer/internal/console"
	"posrelayd-viewer/internal/logger"
)

func AuthLoop(conn *websocket.Conn, reader *bufio.Reader, sessionID string) (string, error) {
	logger.Posrelayv.Debugf("[WS] Starting interactive authorization loop: sessionID=%s", sessionID)

	for {
		fmt.Print("Введите id-подключения: ")

		clientID, err := reader.ReadString('\n')
		if err != nil {
			logger.Posrelayv.Warnf("[WS] Failed to read client ID from console: %v", err)
			return "", err
		}
		clientID = strings.TrimSpace(clientID)

		password, err := console.ReadPassword("Введите пароль: ")
		if err != nil {
			logger.Posrelayv.Warnf("[WS] Failed to read password from console: %v", err)
			fmt.Println("Ошибка ввода пароля:", err)
			continue
		}

		logger.Posrelayv.Debugf("[WS] Sending interactive auth request: sessionID=%s, clientID=%s", sessionID, clientID)
		if err := conn.WriteJSON(Message{
			Type:     "auth",
			ID:       sessionID,
			ClientID: clientID,
			Password: password,
		}); err != nil {
			logger.Posrelayv.Errorf("[WS] Failed to send interactive auth request: %v", err)
			return "", err
		}

		var resp Message
		if err := conn.ReadJSON(&resp); err != nil {
			logger.Posrelayv.Warnf("[WS] Failed to read interactive auth response: %v", err)
			return "", err
		}

		if resp.Type == "auth_ok" {
			logger.Posrelayv.Infof("[WS] Interactive authorization succeeded: sessionID=%s, clientID=%s", sessionID, firstNotEmpty(resp.ClientID, clientID))

			fmt.Println("Авторизация успешна")

			if resp.ClientID != "" {
				return resp.ClientID, nil
			}

			return clientID, nil
		}

		logger.Posrelayv.Warnf("[WS] Interactive authorization rejected: sessionID=%s, clientID=%s, responseType=%s, error=%s", sessionID, clientID, resp.Type, resp.Error)
		fmt.Println("Ошибка авторизации:", resp.Error)
	}
}

func AuthWithCredentials(conn *websocket.Conn, sessionID string, clientID string, password string) (string, error) {
	clientID = strings.TrimSpace(clientID)

	logger.Posrelayv.Debugf("[WS] Starting credential authorization: sessionID=%s, clientID=%s", sessionID, clientID)

	if clientID == "" {
		logger.Posrelayv.Error("[WS] Credential authorization failed: client ID is empty")
		return "", fmt.Errorf("ID клиента не указан")
	}

	if password == "" {
		logger.Posrelayv.Error("[WS] Credential authorization failed: password is empty")
		return "", fmt.Errorf("Пароль не указан")
	}

	if err := conn.WriteJSON(Message{
		Type:     "auth",
		ID:       sessionID,
		ClientID: clientID,
		Password: password,
	}); err != nil {
		logger.Posrelayv.Errorf("[WS] Failed to send credential auth request: %v", err)
		return "", err
	}

	var resp Message
	if err := conn.ReadJSON(&resp); err != nil {
		logger.Posrelayv.Warnf("[WS] Failed to read credential auth response: %v", err)
		return "", err
	}

	if resp.Type == "auth_ok" {
		authorizedClientID := firstNotEmpty(resp.ClientID, clientID)
		logger.Posrelayv.Infof("[WS] Credential authorization succeeded: sessionID=%s, clientID=%s", sessionID, authorizedClientID)
		return authorizedClientID, nil
	}

	logger.Posrelayv.Warnf("[WS] Credential authorization rejected: sessionID=%s, clientID=%s, responseType=%s, error=%s", sessionID, clientID, resp.Type, resp.Error)

	if resp.Error != "" {
		return "", fmt.Errorf(resp.Error)
	}

	return "", fmt.Errorf("ошибка авторизации")
}

func AdminHello(conn *websocket.Conn, apiKey string) error {
	logger.Posrelayv.Debug("[WS] Sending admin_hello")

	if err := conn.WriteJSON(Message{
		Type:   "admin_hello",
		ApiKey: apiKey,
	}); err != nil {
		logger.Posrelayv.Errorf("[WS] Failed to send admin_hello: %v", err)
		fmt.Println("Ошибка отправки admin_hello:", err)
		return err
	}

	var helloResp Message
	if err := conn.ReadJSON(&helloResp); err != nil {
		logger.Posrelayv.Warnf("[WS] Failed to read admin_hello response: %v", err)
		fmt.Println("Соединение разорвано сервером")
		return err
	}

	if helloResp.Type == "error" {
		logger.Posrelayv.Warnf("[WS] admin_hello rejected by server: error=%s", helloResp.Error)
		return fmt.Errorf(helloResp.Error)
	}

	logger.Posrelayv.Info("[WS] admin_hello completed successfully")
	return nil
}

func firstNotEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
