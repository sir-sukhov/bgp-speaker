package main

import (
	"fmt"
	"os"

	"github.com/sir-sukhov/bgp-speaker/internal/speaker"
)

func main() {
	app, err := speaker.NewApp()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error in application initialization: %s\n", err)
		os.Exit(1)
	}
	if err := app.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Exiting: %s\n", err)
		os.Exit(1)
	}
}
