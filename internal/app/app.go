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
	"posrelayd-viewer/internal/logger"
	"posrelayd-viewer/internal/ws"
)

type App struct {
	server string
	apiKey string
	reader *bufio.Reader
}

func getMachineGUID() (string, error) {
	logger.Posrelayv.Debug("Opening Windows registry key to read MachineGuid")

	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		logger.Posrelayv.Errorf("Failed to open registry key for MachineGuid: %v", err)
		return "", err
	}
	defer key.Close()

	guid, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		logger.Posrelayv.Errorf("Failed to read MachineGuid from registry: %v", err)
		return "", err
	}
	return guid, nil
}

func getHardwareID() (string, error) {
	logger.Posrelayv.Debug("Generating hardware ID")

	machineGUID, err := getMachineGUID()
	if err != nil {
		logger.Posrelayv.Errorf("Failed to get MachineGuid for hardware ID generation: %v", err)
		return "", err
	}

	machineGUID = strings.ToLower(strings.TrimSpace(machineGUID))
	hardwareUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(machineGUID))

	logger.Posrelayv.Debugf("Hardware ID successfully generated: %v", hardwareUUID)

	return hardwareUUID.String(), nil
}

func LoadApp() (*App, error) {
	logger.Posrelayv.Debug("Loading application configuration")

	server, ok := crypto.Decrypt(config.Cfg.Connection.Url)
	if !ok {
		logger.Posrelayv.Error("Failed to decrypt server URL")
		return nil, fmt.Errorf("Failed to decrypt server URL")
	}

	apiKey, ok := crypto.Decrypt(config.Cfg.Connection.APIKey)
	if !ok {
		logger.Posrelayv.Error("Failed to decrypt API key")
		return nil, fmt.Errorf("Failed to decrypt API key")
	}

	return &App{
		server: server,
		apiKey: apiKey,
		reader: bufio.NewReader(os.Stdin),
	}, nil
}

func Run() {
	logger.Posrelayv.Info("Starting interactive console application run loop")

	app, err := LoadApp()
	if err != nil {
		logger.Posrelayv.Errorf("Failed to load application: %v", err)
		fmt.Println(err)
		return
	}

	for {
		logger.Posrelayv.Info("Connecting to server")

		// ---------- CONNECT ----------
		conn := ws.ConnectWithRetry(app.server)
		logger.Posrelayv.Info("WebSocket connection established")

		logger.Posrelayv.Debug("Sending admin hello")
		if err := ws.AdminHello(conn, app.apiKey); err != nil {
			logger.Posrelayv.Errorf("Admin hello failed: %v", err)
			fmt.Println(err)
			conn.Close()

			logger.Posrelayv.Warn("Admin hello failed, retrying connection after delay")
			fmt.Println("Admin hello failed, retrying connection after 10s...")
			time.Sleep(10 * time.Second)

			continue
		}

		console.DrainStdin(app.reader)

		hardwareID, err := getHardwareID()
		if err != nil {
			logger.Posrelayv.Errorf("Failed to get hardware ID: %v", err)
			fmt.Println("Failed to get hardware ID:", err)
			conn.Close()
			continue
		}

		sessionID := uuid.NewString()
		logger.Posrelayv.Debugf("Session created: sessionID=%s", sessionID)

		stopKeepAlive := ws.StartKeepAlive(conn, 25*time.Second)
		logger.Posrelayv.Debug("Keep-alive started")

		// ---------- AUTH ----------
		clientID, err := ws.AuthLoop(conn, app.reader, sessionID)
		if err != nil {
			logger.Posrelayv.Warnf("Connection lost during authorization: %v", err)
			fmt.Println("Connection lost during authorization\n")
			conn.Close()
			continue
		}
		logger.Posrelayv.Debugf("Authorization completed for clientID=%s", clientID)

		logger.Posrelayv.Debug("Sending register message")
		if err := conn.WriteJSON(ws.Message{
			Type:       "register",
			Role:       "admin",
			ID:         sessionID,
			HardwareID: hardwareID,
		}); err != nil {
			logger.Posrelayv.Errorf("Failed to send register message: %v", err)
			fmt.Println("Не удалось отправить register:", err)
			conn.Close()
			continue
		}

		displayCfg := config.LoadDisplayConfig()

		logger.Posrelayv.Debugf(
			"Sending rd_start message: display_quality=%s display_codec=%s",
			displayCfg.Quality,
			displayCfg.Codec,
		)

		if err := conn.WriteJSON(ws.Message{
			Type:      "rd_start",
			ID:        sessionID,
			SessionID: sessionID,
			ClientID:  clientID,
			Display: ws.DisplayConfig{
				Quality: displayCfg.Quality.Active,
				Codec:   displayCfg.Codec.Active,
			},
		}); err != nil {
			logger.Posrelayv.Errorf("Failed to send rd_start message: %v", err)
			fmt.Println("Не удалось отправить rd_start:", err)
			conn.Close()
			continue
		}

		logger.Posrelayv.Infof("RD session started: sessionID=%s, clientID=%s", sessionID, clientID)
		fmt.Println("Отправлен rd_start")

		sessionClosed := make(chan struct{})

		stopCtrlC := ws.StartCtrlCHandler(conn, clientID, sessionID)

		logger.Posrelayv.Debug("Starting server reader")
		ws.StartServerReader(conn, sessionClosed, sessionID, clientID, app.server, app.apiKey, false)

		logger.Posrelayv.Info("Entering interactive session loop")
		ws.RunSessionLoop(conn, app.reader, sessionClosed, clientID, sessionID)

		logger.Posrelayv.Info("Interactive session loop finished")

		stopCtrlC()

		stopKeepAlive()
		logger.Posrelayv.Debug("Keep-alive stopped")

		logger.Posrelayv.Info("Session finished, reconnecting:")
		logger.Posrelayv.Debugf("sessionID=%s, clientID=%s\"", sessionID, clientID)
		continue
	}
}

func sendMainUIPopup(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		logger.Posrelayv.Debug("Main UI popup skipped: empty message")
		return
	}

	eventURL := strings.TrimSpace(os.Getenv("POSRELAY_MAIN_UI_EVENT_URL"))
	if eventURL == "" {
		logger.Posrelayv.Debug("Main UI popup skipped: POSRELAY_MAIN_UI_EVENT_URL is empty")
		return
	}

	body, err := json.Marshal(map[string]string{
		"type":    "popup",
		"message": message,
	})
	if err != nil {
		logger.Posrelayv.Errorf("Failed to marshal main UI popup payload: %v", err)
		return
	}

	client := http.Client{
		Timeout: 2 * time.Second,
	}

	logger.Posrelayv.Debug("Sending popup event to main UI")

	resp, err := client.Post(eventURL, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Posrelayv.Warnf("Failed to send popup event to main UI: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Posrelayv.Warnf("Main UI popup endpoint returned non-success status: %s", resp.Status)
		return
	}

	logger.Posrelayv.Debug("Popup event successfully sent to main UI")
}

func RunConnectionSession(clientID string, password string, startRD bool, showConsole bool) error {
	logger.Posrelayv.Infof(
		"Starting connection session: startRD=%t, showConsole=%t",
		startRD,
		showConsole,
	)

	clientID = strings.TrimSpace(clientID)

	if clientID == "" {
		logger.Posrelayv.Error("Connection session start failed: client ID is empty")
		return fmt.Errorf("Connection session start failed: client ID is empty")
	}

	if password == "" {
		logger.Posrelayv.Error("Connection session start failed: password is empty")
		return fmt.Errorf("Connection session start failed: password is empty")
	}

	if showConsole {
		logger.Posrelayv.Debug("Ensuring runtime console for connection session")
		if err := console.EnsureRuntimeConsole(); err != nil {
			logger.Posrelayv.Errorf("Failed to ensure runtime console: %v", err)
			return err
		}
	}

	loadedApp, err := LoadApp()
	if err != nil {
		logger.Posrelayv.Errorf("Failed to load application for connection session: %v", err)
		return err
	}

	if showConsole {
		loadedApp.reader = bufio.NewReader(os.Stdin)
		logger.Posrelayv.Debug("Console reader initialized for connection session")
	}

	var commandInput <-chan string

	if showConsole && !startRD {
		commandInput = ws.StartConsoleCommandReader(loadedApp.reader)
		logger.Posrelayv.Debug("Console command reader started for non-RD session")
	}

	for {
		logger.Posrelayv.Info("Connecting to server for connection session")

		conn := ws.ConnectWithRetry(loadedApp.server)
		logger.Posrelayv.Info("WebSocket connection established for connection session")

		logger.Posrelayv.Debug("Sending admin hello for connection session")
		if err := ws.AdminHello(conn, loadedApp.apiKey); err != nil {
			logger.Posrelayv.Errorf("Admin hello failed during connection session: %v", err)
			fmt.Println(err)
			conn.Close()

			logger.Posrelayv.Warn("Admin hello failed during connection session, retrying after delay")
			fmt.Println("Admin hello failed during connection session, retrying after delay 10s...")
			time.Sleep(10 * time.Second)

			continue
		}
		logger.Posrelayv.Info("Admin hello completed successfully for connection session")

		hardwareID, err := getHardwareID()
		if err != nil {
			logger.Posrelayv.Errorf("Failed to get hardware ID during connection session: %v", err)
			fmt.Println("Failed to get hardware ID during connection session:", err)
			conn.Close()
			time.Sleep(10 * time.Second)
			continue
		}

		sessionID := uuid.NewString()
		logger.Posrelayv.Infof("Connection session created: sessionID=%s, clientID=%s", sessionID, clientID)

		stopKeepAlive := ws.StartKeepAlive(conn, 25*time.Second)
		logger.Posrelayv.Debug("Keep-alive started for connection session")

		logger.Posrelayv.Debug("Authorizing with provided credentials")
		authorizedClientID, err := ws.AuthWithCredentials(conn, sessionID, clientID, password)
		if err != nil {
			message := fmt.Sprintf("Error: %v", err)

			logger.Posrelayv.Warnf("Authorization with credentials failed: %v", err)
			fmt.Println("Error:", err)
			sendMainUIPopup(message)

			conn.Close()
			stopKeepAlive()
			logger.Posrelayv.Debug("Connection session stopped after authorization failure")
			return nil
		}
		logger.Posrelayv.Infof("Authorization with credentials completed: authorizedClientID=%s", authorizedClientID)

		logger.Posrelayv.Debug("Sending register message for connection session")
		if err := conn.WriteJSON(ws.Message{
			Type:       "register",
			Role:       "admin",
			ID:         sessionID,
			HardwareID: hardwareID,
		}); err != nil {
			logger.Posrelayv.Errorf("Failed to send register message during connection session: %v", err)
			fmt.Println("Failed to send register message during connection session:", err)
			conn.Close()
			stopKeepAlive()
			continue
		}
		logger.Posrelayv.Info("Register message sent for connection session")

		if startRD {
			displayCfg := config.LoadDisplayConfig()

			logger.Posrelayv.Debugf(
				"Sending rd_start message for connection session: display_quality=%s display_codec=%s",
				displayCfg.Quality,
				displayCfg.Codec,
			)

			if err := conn.WriteJSON(ws.Message{
				Type:      "rd_start",
				ID:        sessionID,
				SessionID: sessionID,
				ClientID:  authorizedClientID,
				Display: ws.DisplayConfig{
					Quality: displayCfg.Quality.Active,
					Codec:   displayCfg.Codec.Active,
				},
			}); err != nil {
				logger.Posrelayv.Errorf("Failed to send rd_start message during connection session: %v", err)
				fmt.Println("Failed to send rd_start message during connection session:", err)
				conn.Close()
				stopKeepAlive()
				continue
			}

			logger.Posrelayv.Infof(
				"RD connection session starting: sessionID=%s, clientID=%s",
				sessionID,
				authorizedClientID,
			)
			fmt.Println("RD session starting")
		} else {
			logger.Posrelayv.Infof(
				"Non-RD connection session starting: sessionID=%s, clientID=%s",
				sessionID,
				authorizedClientID,
			)
			fmt.Println("nonRD-session starting")
		}

		sessionClosed := make(chan struct{})

		var stopCtrlC func()

		if showConsole {
			stopCtrlC = ws.StartCtrlCHandler(conn, authorizedClientID, sessionID)
			logger.Posrelayv.Debug("Ctrl+C handler started for connection session")
		}

		autoReconnect := showConsole && !startRD
		logger.Posrelayv.Debugf("Starting server reader: autoReconnect=%t", autoReconnect)

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
				logger.Posrelayv.Info("Entering RD session loop")
				ws.RunSessionLoop(
					conn,
					loadedApp.reader,
					sessionClosed,
					authorizedClientID,
					sessionID,
				)
			} else {
				logger.Posrelayv.Info("Entering non-RD command session loop")
				ws.RunSessionCommandLoop(
					conn,
					commandInput,
					sessionClosed,
					authorizedClientID,
					sessionID,
				)
			}
		} else {
			logger.Posrelayv.Debug("Waiting for hidden session close")
			ws.WaitSessionClosed(conn, sessionClosed)
		}

		logger.Posrelayv.Infof("Connection session closed: sessionID=%s, clientID=%s", sessionID, authorizedClientID)

		if stopCtrlC != nil {
			stopCtrlC()
			logger.Posrelayv.Debug("Ctrl+C handler stopped for connection session")
		}

		stopKeepAlive()
		logger.Posrelayv.Debug("Keep-alive stopped for connection session")

		if showConsole && !startRD {
			logger.Posrelayv.Info("Non-RD console session finished, reconnecting")
			time.Sleep(1 * time.Second)
			continue
		}

		logger.Posrelayv.Info("Connection session finished")
		return nil
	}
}

func StartHiddenSession(clientID string, password string, startRD bool, showConsole bool) error {
	logger.Posrelayv.Infof(
		"Starting hidden session goroutine: startRD=%t, showConsole=%t",
		startRD,
		showConsole,
	)

	go func() {
		if err := RunConnectionSession(clientID, password, startRD, showConsole); err != nil {
			logger.Posrelayv.Errorf("Hidden session finished with error: %v", err)
			fmt.Println("Hidden session finished with error:", err)
		}
	}()

	return nil
}
