package main

import (
	"fmt"

	"posrelayd-viewer/internal/app"
	"posrelayd-viewer/internal/gui"
)

func main() {
	if app.HandleStartupOptions() {
		return
	}

	if err := gui.OpenMainWindow(app.StartHiddenSession); err != nil {
		fmt.Println(err)
	}
}
