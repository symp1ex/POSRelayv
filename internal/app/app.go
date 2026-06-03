package app

import (
	"bufio"
	"fmt"
	"github.com/google/uuid"
	"os"
	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/console"
	"posrelayd-viewer/internal/crypto"
	"posrelayd-viewer/internal/ws"
	"time"
)

type App struct {
	server string
	apiKey string
	reader *bufio.Reader
}

func LoadApp() (*App, error) {
	server, ok := crypto.Decrypt(config.Cfg.Connection.Url)
	if !ok {
		return nil, fmt.Errorf("не удалось расшифровать адрес сервера")
	}

	apiKey, ok := crypto.Decrypt(config.Cfg.Connection.APIKey)
	if !ok {
		return nil, fmt.Errorf("не удалось расшифровать API ключ")
	}

	return &App{
		server: server,
		apiKey: apiKey,
		reader: bufio.NewReader(os.Stdin),
	}, nil
}

func Run() {
	if HandleStartupOptions() {
		return
	}

	app, err := LoadApp()
	if err != nil {
		fmt.Println(err)
		return
	}

	for {
		// ---------- CONNECT ----------
		conn := ws.ConnectWithRetry(app.server)

		if err := ws.AdminHello(conn, app.apiKey); err != nil {
			fmt.Println(err)
			conn.Close()

			fmt.Println("Повторная попытка через 10 секунд...")
			time.Sleep(10 * time.Second)

			continue
		}

		console.DrainStdin(app.reader)

		adminID := uuid.NewString()

		// ---------- AUTH ----------
		clientID, err := ws.AuthLoop(conn, app.reader, adminID)
		if err != nil {
			fmt.Println("Соединение потеряно во время авторизации\n")
			conn.Close()
			continue
		}

		_ = conn.WriteJSON(ws.Message{
			Type: "register",
			Role: "admin",
			ID:   adminID,
		})

		sessionClosed := make(chan struct{})

		ws.StartCtrlCHandler(conn, clientID, adminID)

		ws.StartServerReader(conn, sessionClosed)

		ws.RunSessionLoop(conn, app.reader, sessionClosed, clientID, adminID)
		continue
	}
}
