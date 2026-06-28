package main

import (
	"log"

	"posrelayd-viewer/internal/app"
	"posrelayd-viewer/internal/gui"
	"posrelayd-viewer/internal/logger"
)

func main() {
	version := "0.4.9.0"
	gui.EnableWebView2Diagnostics()

	if app.HandleStartupOptions() {
		return
	}

	if err := gui.OpenMainWindow(app.StartHiddenSession, version); err != nil {
		logger.Posrelayv.Errorf(
			"The main thread terminated with the error: \n%s", err)
		log.Fatal(err)
	}
}
