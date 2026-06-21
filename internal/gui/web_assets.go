package gui

import "embed"

// webDistFS содержит production-сборку React UI.
//
// Важно:
// перед `go build` должен быть выполнен:
//
//	cd ui/rd-web
//	npm run build
//
// Иначе go:embed упадет, если internal/gui/web/dist отсутствует
// или не содержит файлов.
//
//go:embed web/dist
var webDistFS embed.FS