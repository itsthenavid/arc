// Package main is the Arc Go server entrypoint binary.
//
// It intentionally delegates startup to the internal app package to keep main small,
// testable (via app), and lint-friendly.
package main

import (
	"log/slog"
	"os"

	"arc/cmd/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		slog.Error("arc.exit", "err", err)
		os.Exit(1)
	}
}
