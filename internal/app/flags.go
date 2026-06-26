package app

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"posrelayd-viewer/internal/config"
	"posrelayd-viewer/internal/console"
	"posrelayd-viewer/internal/logger"
)

type Flags struct {
	Setup   bool
	Session bool
	Console bool
}

func ParseFlags() Flags {
	var flags Flags

	flag.BoolVar(&flags.Setup, "setup", false, "configure application")
	flag.BoolVar(&flags.Session, "session", false, "start standalone connection session")
	flag.BoolVar(&flags.Console, "console", false, "start in console mode without UI")

	flag.Parse()

	logger.Posrelayv.Debugf(
		"Startup flags parsed: setup=%t, session=%t, console=%t",
		flags.Setup,
		flags.Session,
		flags.Console,
	)

	return flags
}

func HandleStartupOptions() bool {
	logger.Posrelayv.Debug("Handling startup options")

	flags := ParseFlags()

	switch {
	case flags.Session:
		logger.Posrelayv.Info("Startup option selected: standalone connection session")

		clientID := os.Getenv("POSRELAY_CLIENT_ID")
		password := os.Getenv("POSRELAY_PASSWORD")
		startRD := os.Getenv("POSRELAY_START_RD") == "1"
		showConsole := os.Getenv("POSRELAY_SHOW_CONSOLE") == "1"

		logger.Posrelayv.Debugf(
			"Session environment options loaded: clientIDProvided=%t, passwordProvided=%t, startRD=%t, showConsole=%t",
			strings.TrimSpace(clientID) != "",
			password != "",
			startRD,
			showConsole,
		)

		if err := RunConnectionSession(clientID, password, startRD, showConsole); err != nil {
			logger.Posrelayv.Errorf("Failed to start connection session: %v", err)
			fmt.Println("Ошибка запуска сессии:", err)
			os.Exit(1)
		}

		logger.Posrelayv.Info("Standalone connection session startup option completed")
		return true

	case flags.Setup:
		logger.Posrelayv.Info("Startup option selected: setup")

		logger.Posrelayv.Debug("Ensuring runtime console for setup")
		if err := console.EnsureRuntimeConsole(); err != nil {
			logger.Posrelayv.Errorf("Failed to ensure runtime console for setup: %v", err)
			os.Exit(1)
		}

		logger.Posrelayv.Info("Starting application setup")
		if err := config.Setup(); err != nil {
			logger.Posrelayv.Errorf("Application setup failed: %v", err)
			fmt.Println("Ошибка настройки:", err)
			os.Exit(1)
		}

		logger.Posrelayv.Info("Application configuration successfully saved")
		fmt.Println("Конфигурация успешно сохранена")
		return true

	case flags.Console:
		logger.Posrelayv.Info("Startup option selected: console mode")

		logger.Posrelayv.Debug("Ensuring runtime console for console mode")
		if err := console.EnsureRuntimeConsole(); err != nil {
			logger.Posrelayv.Errorf("Failed to ensure runtime console for console mode: %v", err)
			os.Exit(1)
		}

		Run()
		return true
	}

	logger.Posrelayv.Debug("No startup option selected, continuing normal GUI startup")
	return false
}
