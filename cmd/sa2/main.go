package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/victorx98/sa2-cli/internal/api"
	"github.com/victorx98/sa2-cli/internal/cli"
	"github.com/victorx98/sa2-cli/internal/session"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	app := cli.App{
		In:          os.Stdin,
		Out:         os.Stdout,
		Err:         os.Stderr,
		Version:     version,
		Origin:      os.Getenv("SA2_API_URL"),
		API:         api.Client{HTTP: api.NewHTTPClient()},
		OpenSession: session.Open,
	}
	code := app.Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
