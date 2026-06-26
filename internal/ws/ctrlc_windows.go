//go:build windows

package ws

import (
	"bufio"
	"fmt"
	"strings"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows"

	"posrelayd-viewer/internal/logger"
)

const (
	ctrlCEvent     = 0
	ctrlBreakEvent = 1
	ctrlCloseEvent = 2
)

var (
	kernel32CtrlC              = windows.NewLazySystemDLL("kernel32.dll")
	procSetConsoleCtrlHandler  = kernel32CtrlC.NewProc("SetConsoleCtrlHandler")
	ctrlCHandlerMu             sync.Mutex
	ctrlCHandlerCallback       uintptr
	ctrlCHandlerCurrentConn    *websocket.Conn
	ctrlCHandlerCurrentClient  string
	ctrlCHandlerCurrentSession string
)

func StartCtrlCHandler(conn *websocket.Conn, clientID, sessionID string) func() {
	logger.Posrelayv.Debugf("[WS] Starting Ctrl+C handler: sessionID=%s, clientID=%s", sessionID, clientID)

	ctrlCHandlerMu.Lock()
	defer ctrlCHandlerMu.Unlock()

	ctrlCHandlerCurrentConn = conn
	ctrlCHandlerCurrentClient = clientID
	ctrlCHandlerCurrentSession = sessionID

	if ctrlCHandlerCallback == 0 {
		ctrlCHandlerCallback = syscall.NewCallback(consoleCtrlHandler)

		ret, _, err := procSetConsoleCtrlHandler.Call(ctrlCHandlerCallback, 1)
		if ret == 0 {
			logger.Posrelayv.Errorf("[WS] SetConsoleCtrlHandler failed: %v", err)
			fmt.Println("SetConsoleCtrlHandler failed:", err)
		} else {
			logger.Posrelayv.Debug("[WS] Console control handler registered")
		}
	}

	return func() {
		ctrlCHandlerMu.Lock()
		defer ctrlCHandlerMu.Unlock()

		if ctrlCHandlerCurrentConn == conn {
			ctrlCHandlerCurrentConn = nil
			ctrlCHandlerCurrentClient = ""
			ctrlCHandlerCurrentSession = ""
			logger.Posrelayv.Debugf("[WS] Ctrl+C handler detached: sessionID=%s, clientID=%s", sessionID, clientID)
		}
	}
}

func StartConsoleCommandReader(reader *bufio.Reader) <-chan string {
	logger.Posrelayv.Debug("[WS] Starting console command reader")

	cmdChan := make(chan string, 32)

	go func() {
		for {
			cmd, err := reader.ReadString('\n')
			if err != nil {
				logger.Posrelayv.Warnf("[WS] Failed to read console command: %v", err)
				continue
			}

			cmd = strings.TrimRight(cmd, "\r\n")
			if cmd == "" {
				continue
			}

			logger.Posrelayv.Debugf("[WS] Console command queued: length=%d", len(cmd))
			cmdChan <- cmd
		}
	}()

	return cmdChan
}

func RunSessionCommandLoop(
	conn *websocket.Conn,
	cmdChan <-chan string,
	sessionClosed chan struct{},
	clientID string,
	sessionID string,
) {
	logger.Posrelayv.Debugf("[WS] Starting non-RD command loop: sessionID=%s, clientID=%s", sessionID, clientID)

	for {
		select {
		case <-sessionClosed:
			logger.Posrelayv.Infof("[WS] Session closed, stopping non-RD command loop: sessionID=%s, clientID=%s", sessionID, clientID)
			_ = conn.Close()
			return

		case cmd := <-cmdChan:
			commandID := uuid.NewString()
			if err := conn.WriteJSON(Message{
				Type:      "command",
				ClientID:  clientID,
				CommandID: commandID,
				Command:   cmd,
				ID:        sessionID,
			}); err != nil {
				logger.Posrelayv.Warnf("[WS] Failed to send non-RD command: sessionID=%s, clientID=%s, commandID=%s, error=%v", sessionID, clientID, commandID, err)
				_ = conn.Close()
				fmt.Println("\nСоединение потеряно, переподключение...\n")
				return
			}

			logger.Posrelayv.Debugf("[WS] Non-RD command sent: sessionID=%s, clientID=%s, commandID=%s, length=%d", sessionID, clientID, commandID, len(cmd))
		}
	}
}

func consoleCtrlHandler(ctrlType uint32) uintptr {
	if ctrlType != ctrlCEvent {
		// Важно: не перехватываем крестик окна, logout, shutdown и прочее.
		// Для них возвращаем FALSE, чтобы Windows обработала их штатно.
		return 0
	}

	ctrlCHandlerMu.Lock()
	conn := ctrlCHandlerCurrentConn
	clientID := ctrlCHandlerCurrentClient
	sessionID := ctrlCHandlerCurrentSession
	ctrlCHandlerMu.Unlock()

	if conn != nil {
		logger.Posrelayv.Debugf("[WS] Sending CTRL_C control message: sessionID=%s, clientID=%s", sessionID, clientID)
		if err := conn.WriteJSON(Message{
			Type:      "control",
			ClientID:  clientID,
			ID:        sessionID,
			SessionID: sessionID,
			Command:   "CTRL_C",
		}); err != nil {
			logger.Posrelayv.Warnf("[WS] Failed to send CTRL_C control message: sessionID=%s, clientID=%s, error=%v", sessionID, clientID, err)
		}
	} else {
		logger.Posrelayv.Debug("[WS] CTRL_C event ignored: active connection is not set")
	}

	// TRUE: событие обработано, default handler Windows не вызывается,
	// значит текущий процесс и окно консоли не закрываются.
	return 1
}
