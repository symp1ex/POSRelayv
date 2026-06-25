package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"

	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/console"
	"posrelayd-viewer/internal/crypto"
	"posrelayd-viewer/internal/ws"
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

		stopCtrlC := ws.StartCtrlCHandler(conn, clientID, sessionID)

		ws.StartServerReader(conn, sessionClosed, sessionID, clientID, app.server, app.apiKey, false)

		ws.RunSessionLoop(conn, app.reader, sessionClosed, clientID, sessionID)

		stopCtrlC()
		stopKeepAlive()
		continue
	}
}

func sendMainUIPopup(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	eventURL := strings.TrimSpace(os.Getenv("POSRELAY_MAIN_UI_EVENT_URL"))
	if eventURL == "" {
		return
	}

	body, err := json.Marshal(map[string]string{
		"type":    "popup",
		"message": message,
	})
	if err != nil {
		return
	}

	client := http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Post(eventURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func RunConnectionSession(clientID string, password string, startRD bool, showConsole bool) error {
	clientID = strings.TrimSpace(clientID)

	if clientID == "" {
		return fmt.Errorf("ID клиента не указан")
	}

	if password == "" {
		return fmt.Errorf("Пароль не указан")
	}

	if showConsole {
		if err := console.EnsureRuntimeConsole(); err != nil {
			return err
		}
	}

	loadedApp, err := LoadApp()
	if err != nil {
		return err
	}

	if showConsole {
		loadedApp.reader = bufio.NewReader(os.Stdin)
	}

	var commandInput <-chan string

	if showConsole && !startRD {
		commandInput = ws.StartConsoleCommandReader(loadedApp.reader)
	}

	for {
		conn := ws.ConnectWithRetry(loadedApp.server)

		if err := ws.AdminHello(conn, loadedApp.apiKey); err != nil {
			fmt.Println(err)
			conn.Close()

			fmt.Println("Повторная попытка через 10 секунд...")
			time.Sleep(10 * time.Second)

			continue
		}

		hardwareID, err := getHardwareID()
		if err != nil {
			fmt.Println("Не удалось получить hardwareID:", err)
			conn.Close()
			time.Sleep(10 * time.Second)
			continue
		}

		sessionID := uuid.NewString()

		stopKeepAlive := ws.StartKeepAlive(conn, 25*time.Second)

		authorizedClientID, err := ws.AuthWithCredentials(conn, sessionID, clientID, password)
		if err != nil {
			message := fmt.Sprintf("Error: %v", err)

			fmt.Println("Error:", err)
			sendMainUIPopup(message)

			conn.Close()
			stopKeepAlive()
			return nil
		}

		if err := conn.WriteJSON(ws.Message{
			Type:       "register",
			Role:       "admin",
			ID:         sessionID,
			HardwareID: hardwareID,
		}); err != nil {
			fmt.Println("Не удалось отправить register:", err)
			conn.Close()
			stopKeepAlive()
			continue
		}

		if startRD {
			if err := conn.WriteJSON(ws.Message{
				Type:      "rd_start",
				ID:        sessionID,
				SessionID: sessionID,
				ClientID:  authorizedClientID,
			}); err != nil {
				fmt.Println("Не удалось отправить rd_start:", err)
				conn.Close()
				stopKeepAlive()
				continue
			}

			fmt.Println("RD session starting")
		} else {
			fmt.Println("nonRD-session starting")
		}

		sessionClosed := make(chan struct{})

		var stopCtrlC func()

		if showConsole {
			stopCtrlC = ws.StartCtrlCHandler(conn, authorizedClientID, sessionID)
		}

		autoReconnect := showConsole && !startRD

		ws.StartServerReader(
			conn,
			sessionClosed,
			sessionID,
			authorizedClientID,
			loadedApp.server,
			loadedApp.apiKey,
			autoReconnect,
		)

		if showConsole {
			if startRD {
				ws.RunSessionLoop(
					conn,
					loadedApp.reader,
					sessionClosed,
					authorizedClientID,
					sessionID,
				)
			} else {
				ws.RunSessionCommandLoop(
					conn,
					commandInput,
					sessionClosed,
					authorizedClientID,
					sessionID,
				)
			}
		} else {
			ws.WaitSessionClosed(conn, sessionClosed)
		}

		if stopCtrlC != nil {
			stopCtrlC()
		}

		stopKeepAlive()

		if showConsole && !startRD {
			time.Sleep(1 * time.Second)
			continue
		}

		return nil
	}
}

func StartHiddenSession(clientID string, password string, startRD bool, showConsole bool) error {
	go func() {
		if err := RunConnectionSession(clientID, password, startRD, showConsole); err != nil {
			fmt.Println("Ошибка сессии:", err)
		}
	}()

	return nil
}
