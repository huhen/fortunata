// Пакет version: версия сборки, прошиваемая при сборке через
// -ldflags "-X fortunata/internal/version.Version=...".
package version

// Version — версия сборки; "dev" для локального go run и go test.
var Version = "dev"
