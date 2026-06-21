package app

import (
	"bufio"
	"fmt"
	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
	"os"
	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/console"
	"posrelayd-viewer/internal/crypto"
	"posrelayd-viewer/internal/ws"
	"strings"
	"time"
)

type App struct {
	server string
	apiKey string
	reader *bufio.Reader
}

func getMachineGUID() (string, error) {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return "", err
	}
	defer key.Close()

	guid, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		return "", err
	}

	return guid, nil
}

func getHardwareID() (string, error) {
	machineGUID, err := getMachineGUID()
	if err != nil {
		return "", err
	}

	machineGUID = strings.ToLower(strings.TrimSpace(machineGUID))
	hardwareUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(machineGUID))

	return hardwareUUID.String(), nil
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

		hardwareID, err := getHardwareID()
		if err != nil {
			fmt.Println("Не удалось получить hardwareID:", err)
			conn.Close()
			continue
		}

		sessionID := uuid.NewString()

		stopKeepAlive := ws.StartKeepAlive(conn, 25*time.Second)

		// ---------- AUTH ----------
		clientID, err := ws.AuthLoop(conn, app.reader, sessionID)
		if err != nil {
			fmt.Println("Соединение потеряно во время авторизации\n")
			conn.Close()
			continue
		}

		if err := conn.WriteJSON(ws.Message{
			Type:       "register",
			Role:       "admin",
			ID:         sessionID,
			HardwareID: hardwareID,
		}); err != nil {
			fmt.Println("Не удалось отправить register:", err)
			conn.Close()
			continue
		}

		if err := conn.WriteJSON(ws.Message{
			Type:      "rd_start",
			ID:        sessionID,
			SessionID: sessionID,
			ClientID:  clientID,
		}); err != nil {
			fmt.Println("Не удалось отправить rd_start:", err)
			conn.Close()
			continue
		}

		fmt.Println("Отправлен rd_start")

		sessionClosed := make(chan struct{})

		ws.StartCtrlCHandler(conn, clientID, sessionID)

		ws.StartServerReader(conn, sessionClosed, sessionID)

		stopKeepAlive()

		ws.RunSessionLoop(conn, app.reader, sessionClosed, clientID, sessionID)
		continue
	}
}
