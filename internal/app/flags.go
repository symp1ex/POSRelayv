package app

import (
	"flag"
	"fmt"
	"os"
	"posrelayd-viewer/internal/config"
)

type Flags struct {
	Setup bool
}

func ParseFlags() Flags {
	var flags Flags

	flag.BoolVar(&flags.Setup, "setup", false, "configure application")
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
	}

	return false
}
