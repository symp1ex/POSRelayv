//go:build windows

package ws

import (
	"bufio"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows"
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
	ctrlCHandlerMu.Lock()
	defer ctrlCHandlerMu.Unlock()

	ctrlCHandlerCurrentConn = conn
	ctrlCHandlerCurrentClient = clientID
	ctrlCHandlerCurrentSession = sessionID

	if ctrlCHandlerCallback == 0 {
		ctrlCHandlerCallback = syscall.NewCallback(consoleCtrlHandler)

		ret, _, err := procSetConsoleCtrlHandler.Call(ctrlCHandlerCallback, 1)
		if ret == 0 {
			fmt.Println("SetConsoleCtrlHandler failed:", err)
		}
	}

	return func() {
		ctrlCHandlerMu.Lock()
		defer ctrlCHandlerMu.Unlock()

		if ctrlCHandlerCurrentConn == conn {
			ctrlCHandlerCurrentConn = nil
			ctrlCHandlerCurrentClient = ""
			ctrlCHandlerCurrentSession = ""
		}
	}
}

func StartConsoleCommandReader(reader *bufio.Reader) <-chan string {
	cmdChan := make(chan string, 32)

	go func() {
		for {
			cmd, err := reader.ReadString('\n')
			if err != nil {
				continue
			}

			cmd = strings.TrimRight(cmd, "\r\n")
			if cmd == "" {
				continue
			}

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
	for {
		select {
		case <-sessionClosed:
			_ = conn.Close()
			return

		case cmd := <-cmdChan:
			if err := conn.WriteJSON(Message{
				Type:      "command",
				ClientID:  clientID,
				CommandID: uuid.NewString(),
				Command:   cmd,
				ID:        sessionID,
			}); err != nil {
				_ = conn.Close()
				fmt.Println("\nСоединение потеряно, переподключение...\n")
				return
			}
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
		_ = conn.WriteJSON(Message{
			Type:      "control",
			ClientID:  clientID,
			ID:        sessionID,
			SessionID: sessionID,
			Command:   "CTRL_C",
		})
	}

	// TRUE: событие обработано, default handler Windows не вызывается,
	// значит текущий процесс и окно консоли не закрываются.
	return 1
}
