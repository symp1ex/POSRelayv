package gui

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"posrelayd-viewer/internal/logger"
)

var webview2DebugPort string

func IsWebView2DebugEnabled() bool {
	return os.Getenv("POSRELAY_WEBVIEW2_DEBUG") == "1"
}

func EnableWebView2Diagnostics() {
	if !IsWebView2DebugEnabled() {
		return
	}

	existing := strings.TrimSpace(os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"))
	if existing != "" && strings.Contains(existing, "--remote-debugging-port=") {
		port := extractWebView2DebugPort(existing)
		if port != "" {
			webview2DebugPort = port
			logWebView2DebugURLs(port)
		}

		logger.Posrelayv.Debug("[GUI] WebView2 diagnostics already configured in environment")
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to resolve executable path: %v", err)
		return
	}

	exeDir := filepath.Dir(exePath)

	logDir := filepath.Join(exeDir, "logs")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to create WebView2 log directory: %v", err)
		return
	}

	logFile := filepath.Join(logDir, "webview2-debug.log")

	port, err := allocateWebView2DebugPort()
	if err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to allocate WebView2 debug port: %v", err)
		return
	}

	webview2DebugPort = port

	args := []string{
		"--enable-logging",
		"--v=1",
		"--log-file=" + logFile,
		"--remote-debugging-port=" + port,
	}

	value := strings.Join(args, " ")

	if existing != "" {
		value = existing + " " + value
	}

	if err := os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", value); err != nil {
		logger.Posrelayv.Warnf("[GUI] Failed to set WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS: %v", err)
		return
	}

	logger.Posrelayv.Debug("[GUI] WebView2 diagnostics enabled")
	logger.Posrelayv.Debugf("[GUI] WebView2 Chromium log: %s", logFile)
	logWebView2DebugURLs(port)
}

func allocateWebView2DebugPort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected listener address type: %T", listener.Addr())
	}

	return fmt.Sprintf("%d", addr.Port), nil
}

func extractWebView2DebugPort(args string) string {
	re := regexp.MustCompile(`--remote-debugging-port=(\d+)`)
	match := re.FindStringSubmatch(args)
	if len(match) < 2 {
		return ""
	}

	return match[1]
}

func logWebView2DebugURLs(port string) {
	if port == "" {
		return
	}

	logger.Posrelayv.Debugf(
		"[GUI] WebView2 DevTools JSON list: http://127.0.0.1:%s/json/list",
		port,
	)
}
