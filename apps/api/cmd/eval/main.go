package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"api/internal/app"
	"api/internal/eval"
)

const (
	exitFailed = 1
	exitBroken = 2
)

func main() {
	failures, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(exitBroken)
	}
	if failures > 0 {
		os.Exit(exitFailed)
	}
}

func run() (int, error) {
	config, err := app.LoadConfig()
	if err != nil {
		return 0, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.Build(ctx, config)
	if err != nil {
		return 0, err
	}
	defer func() { _ = runtime.Close() }()

	fmt.Printf("store                  %s\n", config.StorePath())
	report, err := eval.Run(ctx, runtime, eval.Options{
		ResumeDir: os.Getenv("CV_DIR"),
		Out:       os.Stdout,
	})
	if err != nil {
		return 0, err
	}
	return len(report.Failures), nil
}
