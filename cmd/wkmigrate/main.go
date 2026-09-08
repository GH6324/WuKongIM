package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	migrationapp "github.com/WuKongIM/WuKongIM/internal/app/migration"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := migrationapp.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	cancel()
	os.Exit(code)
}
