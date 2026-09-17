// Package web содержит статические файлы фронтенда, зашитые в бинарник.
package web

import "embed"

//go:embed index.html admin.html app.js admin.js common.js footer.js parse.js style.css
var Files embed.FS
