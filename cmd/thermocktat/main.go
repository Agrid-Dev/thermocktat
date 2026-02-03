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
	modbusctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/modbus"
	mqttctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/mqtt"
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

	snap, err := cfg.Snapshot()
	if err != nil {
		log.Fatal(err)
	}
	regulatorParams, err := cfg.RegulatorParams()
	if err != nil {
		log.Fatal(err)
	}
	th, err := thermostat.New(snap, regulatorParams)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deviceID := cfg.DeviceID

	// start regulation
	if cfg.Regulator.Enabled {
		go func() {
			if err := th.Run(ctx, cfg.Regulator.Interval); err != nil && err != context.Canceled {
				log.Printf("thermostat exited: %v", err)
				cancel()
			}
		}()

	}

	// Start enabled controllers
	if cfg.Controllers.HTTP.Enabled {
		srv := httpctrl.New(th, cfg.Controllers.HTTP.Addr, deviceID)
		go func() {
			log.Printf("http controller listening on %s", cfg.Controllers.HTTP.Addr)
			if err := srv.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("http controller exited: %v", err)
				cancel()
			}
		}()
	}

	if cfg.Controllers.MQTT.Enabled {
		mc, err := mqttctrl.New(th, mqttctrl.Config{
			DeviceID:        deviceID,
			BrokerURL:       cfg.Controllers.MQTT.Addr,
			ClientID:        cfg.Controllers.MQTT.ClientID,
			BaseTopic:       cfg.Controllers.MQTT.BaseTopic,
			QoS:             cfg.Controllers.MQTT.QoS,
			RetainSnapshot:  cfg.Controllers.MQTT.RetainSnapshot,
			PublishInterval: cfg.Controllers.MQTT.PublishInterval,
			PublishMode:     cfg.Controllers.MQTT.PublishMode,
			Username:        cfg.Controllers.MQTT.Username,
			Password:        cfg.Controllers.MQTT.Password,
		})
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			log.Printf("mqtt controller broker=%s base_topic=%s", cfg.Controllers.MQTT.Addr, cfg.Controllers.MQTT.BaseTopic)
			if err := mc.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("mqtt controller exited: %v", err)
				cancel()
			}
		}()
	}
	if cfg.Controllers.MODBUS.Enabled {
		mc, err := modbusctrl.New(th, modbusctrl.Config{
			DeviceID:     deviceID,
			Addr:         cfg.Controllers.MODBUS.Addr,
			UnitID:       cfg.Controllers.MODBUS.UnitID,
			SyncInterval: cfg.Controllers.MODBUS.SyncInterval,
		})
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			log.Printf("modbus controller listening to %s with UnitID %d", cfg.Controllers.MODBUS.Addr, cfg.Controllers.MODBUS.UnitID)
			if err := mc.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("modbus controller exited: %v", err)
				cancel()
			}
		}()
	}

	// Block until shutdown.
	<-ctx.Done()
}
