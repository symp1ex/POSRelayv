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
		sessionID = "rd-session"
	}

	return base + "?session_id=" + url.QueryEscape(sessionID), nil
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

	if cleanPath == "/" || cleanPath == "/index.html" {
		h.serveIndex(w, r)
		return
	}

	h.fileServer.ServeHTTP(w, r)
}

func (h *rdWebHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	htmlBytes, err := fs.ReadFile(h.distFS, "index.html")
	if err != nil {
		http.Error(w, "React RD build is missing", http.StatusInternalServerError)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = "rd-session"
	}

	bootstrap, err := json.Marshal(map[string]string{
		"sessionID": sessionID,
	})
	if err != nil {
		http.Error(w, "Failed to prepare RD bootstrap", http.StatusInternalServerError)
		return
	}

	bootstrapTag := `<script>window.__RD_BOOTSTRAP__ = ` + string(bootstrap) + `;</script>`
	htmlText := string(htmlBytes)

	if strings.Contains(htmlText, "</head>") {
		htmlText = strings.Replace(htmlText, "</head>", bootstrapTag+"\n</head>", 1)
	} else {
		htmlText = bootstrapTag + htmlText
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(htmlText))
}
