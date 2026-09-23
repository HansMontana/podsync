package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HansMontana/podsync/internal/adapters/httpclient"
	"github.com/HansMontana/podsync/internal/adapters/logging"
)

var httpClient = httpclient.New(10 * time.Minute)

func main() {
	logger := commandLogger("cli")
	started := time.Now()
	command := "help"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	logger.Info("Starting podsync command " + command)
	if err := run(os.Args[1:]); err != nil {
		logger.Error("Command failed: " + err.Error())
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Completed command %s in %s", command, time.Since(started).Round(time.Millisecond)))
}

func commandLogger(component string) *logging.Logger {
	return logging.New(os.Stderr).WithComponent(component)
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runContext(ctx, args)
}
