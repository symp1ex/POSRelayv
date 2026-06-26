package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"posrelayd-viewer/internal/logger"
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
		logger.Posrelayv.Debug("[GUI] Starting local RD web server")

		distFS, err := fs.Sub(webDistFS, "web/dist")
		if err != nil {
			rdWebServerErr = fmt.Errorf("failed to open embedded React dist: %w", err)
			logger.Posrelayv.Errorf("[GUI] Failed to open embedded React dist: %v", err)
			return
		}

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			rdWebServerErr = fmt.Errorf("failed to start local RD web server: %w", err)
			logger.Posrelayv.Errorf("[GUI] Failed to start local RD web server: %v", err)
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
		logger.Posrelayv.Infof("[GUI] Local RD web server started: base_url=%s", rdWebServerBase)

		go func() {
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Posrelayv.Errorf("[GUI] Local RD web server stopped with error: %v", err)
			}
		}()
	})

	if rdWebServerErr != nil {
		logger.Posrelayv.Errorf("[GUI] Local RD web server is unavailable: %v", rdWebServerErr)
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
		logger.Posrelayv.Warnf("[GUI] Rejected main UI event with unsupported method: method=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var event mainUIEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to decode main UI event: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	event.Message = strings.TrimSpace(event.Message)
	if event.Message == "" {
		logger.Posrelayv.Warnf("[GUI] Rejected empty main UI event: type=%s", event.Type)
		http.Error(w, "message is empty", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "popup", "":
		logger.Posrelayv.Debugf("[GUI] Dispatching main window popup from web event: length=%d", len(event.Message))
		ShowMainWindowPopup(event.Message)
	default:
		logger.Posrelayv.Warnf("[GUI] Rejected unknown main UI event type: type=%s", event.Type)
		http.Error(w, "unknown event type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
