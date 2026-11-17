// Command sciurus edits portable MySQL ER models and generates SQL locally.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mhemmings/sciurus/internal/app"
)

// main keeps process lifecycle concerns outside the testable CLI implementation.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exitCode := app.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(exitCode)
}
