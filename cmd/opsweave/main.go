package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/PHPDevsr/opsweave/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return cli.Run(ctx, os.Args, os.Stdout, os.Stderr)
}
