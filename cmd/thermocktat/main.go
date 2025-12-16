package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	httpctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/http"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

func main() {
	addr := getenv("THERMOCKSTAT_HTTP_ADDR", ":8080")

	th, err := thermostat.New(thermostat.Snapshot{
		Enabled:                true,
		TemperatureSetpoint:    22,
		TemperatureSetpointMin: 16,
		TemperatureSetpointMax: 28,
		Mode:                   thermostat.ModeAuto,
		FanSpeed:               thermostat.FanAuto,
		AmbientTemperature:     21,
	})
	if err != nil {
		log.Fatal(err)
	}

	srv := httpctrl.New(th, addr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("thermockstat listening on %s", addr)
	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("server exited: %v", err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
