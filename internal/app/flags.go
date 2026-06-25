package app

import (
	"flag"
	"fmt"
	"os"
	"posrelayd-viewer/internal/config"
)

type Flags struct {
	Setup   bool
	Session bool
}

func ParseFlags() Flags {
	var flags Flags

	flag.BoolVar(&flags.Setup, "setup", false, "configure application")
	flag.BoolVar(&flags.Session, "session", false, "start standalone connection session")

	flag.Parse()
	return flags
}

func HandleStartupOptions() bool {
	flags := ParseFlags()

	switch {
	case flags.Setup:
		if err := config.Setup(); err != nil {
			fmt.Println("Ошибка настройки:", err)
			os.Exit(1)
		}

		fmt.Println("Конфигурация успешно сохранена")
		return true

	case flags.Session:
		clientID := os.Getenv("POSRELAY_CLIENT_ID")
		password := os.Getenv("POSRELAY_PASSWORD")
		startRD := os.Getenv("POSRELAY_START_RD") == "1"
		showConsole := os.Getenv("POSRELAY_SHOW_CONSOLE") == "1"

		if err := RunConnectionSession(clientID, password, startRD, showConsole); err != nil {
			fmt.Println("Ошибка запуска сессии:", err)
			os.Exit(1)
		}

		return true
	}

	return false
}
