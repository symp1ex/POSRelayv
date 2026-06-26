package gui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
)

var (
	rdWebServerOnce sync.Once
	rdWebServerBase string
	rdWebServerErr  error
)

type mainUIEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type rdWebHandler struct {
	distFS     fs.FS
	fileServer http.Handler
}

func rdWebURL(sessionID string) (string, error) {
	base, err := ensureRDWebServer()
	if err != nil {
		return "", err
	}

	if sessionID == "" {
		return base, nil
	}

	return base + "rd.html?session_id=" + url.QueryEscape(sessionID), nil
}

func ensureRDWebServer() (string, error) {
	rdWebServerOnce.Do(func() {
		distFS, err := fs.Sub(webDistFS, "web/dist")
		if err != nil {
			rdWebServerErr = fmt.Errorf("failed to open embedded React dist: %w", err)
			return
		}

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			rdWebServerErr = fmt.Errorf("failed to start local RD web server: %w", err)
			return
		}

		handler := &rdWebHandler{
			distFS:     distFS,
			fileServer: http.FileServer(http.FS(distFS)),
		}

		server := &http.Server{
			Handler: handler,
		}

		rdWebServerBase = "http://" + listener.Addr().String() + "/"

		go func() {
			_ = server.Serve(listener)
		}()
	})

	if rdWebServerErr != nil {
		return "", rdWebServerErr
	}

	return rdWebServerBase, nil
}

func (h *rdWebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := path.Clean("/" + r.URL.Path)

	if cleanPath == "/api/main-ui-event" {
		h.handleMainUIEvent(w, r)
		return
	}

	h.fileServer.ServeHTTP(w, r)
}

func (h *rdWebHandler) handleMainUIEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var event mainUIEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	event.Message = strings.TrimSpace(event.Message)
	if event.Message == "" {
		http.Error(w, "message is empty", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "popup", "":
		ShowMainWindowPopup(event.Message)
	default:
		http.Error(w, "unknown event type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
