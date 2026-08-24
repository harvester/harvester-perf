package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/harvester/hvperf/cmd"
	_ "github.com/harvester/hvperf/internal/suites" // register built-in suites
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
