package main

import (
	"log"

	"arc/cmd/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
