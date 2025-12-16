package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Agrid-Dev/thermocktat/cmd/app"
	httpctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/http"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file (.yaml/.yml/.json)")
	flag.Parse()

	cfg, err := app.LoadConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}
	app.ApplyEnvOverrides(&cfg)
	snap, err := cfg.Snapshot()
	if err != nil {
		log.Fatal(err)
	}

	th, err := thermostat.New(snap)
	if err != nil {
		log.Fatal(err)
	}

	srv := httpctrl.New(th, cfg.HTTP.Addr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("thermockstat listening on %s", cfg.HTTP.Addr)
	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("server exited: %v", err)
	}
}
