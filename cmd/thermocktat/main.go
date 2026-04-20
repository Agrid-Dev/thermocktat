package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/Agrid-Dev/thermocktat/cmd/app"
	"github.com/Agrid-Dev/thermocktat/internal/buildinfo"
	bacnetctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/bacnet"
	httpctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/http"
	knxctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/knx"
	modbusctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/modbus"
	mqttctrl "github.com/Agrid-Dev/thermocktat/internal/controllers/mqtt"
	"github.com/Agrid-Dev/thermocktat/internal/logging"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file (.yaml/.yml/.json)")
	flag.Parse()

	// Bootstrap logger with zero-config defaults so early errors are not lost.
	// Replaced after config load with the configured one.
	root := logging.New(logging.Config{})

	cfg, err := app.LoadConfig(configPath)
	if err != nil {
		root.Error("config load failed", "err", err)
		os.Exit(1)
	}

	root = logging.New(cfg.Logging)
	root.Info("thermocktat starting",
		"version", buildinfo.Version,
		"commit", buildinfo.Commit,
		"built", buildinfo.Date,
	)
	root.Info("config loaded",
		"device_id", cfg.DeviceID,
		"log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
		"http", cfg.Controllers.HTTP.Enabled,
		"mqtt", cfg.Controllers.MQTT.Enabled,
		"modbus", cfg.Controllers.MODBUS.Enabled,
		"bacnet", cfg.Controllers.BACNET.Enabled,
		"knx", cfg.Controllers.KNX.Enabled,
	)

	snap, err := cfg.Snapshot()
	if err != nil {
		root.Error("config snapshot failed", "err", err)
		os.Exit(1)
	}
	regulatorParams, err := cfg.RegulatorParams()
	if err != nil {
		root.Error("regulator params invalid", "err", err)
		os.Exit(1)
	}
	heatLossParams, err := cfg.HeatLossParams()
	if err != nil {
		root.Error("heat-loss params invalid", "err", err)
		os.Exit(1)
	}

	thermoLog := root.With("component", "thermostat")
	th, err := thermostat.New(thermoLog, snap, regulatorParams, heatLossParams)
	if err != nil {
		root.Error("thermostat init failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deviceID := cfg.DeviceID

	// start regulation
	go func() {
		if err := th.Run(ctx, cfg.Regulator.Interval); err != nil && !errors.Is(err, context.Canceled) {
			thermoLog.Error("thermostat exited", "err", err)
			cancel()
		}
	}()

	if cfg.Controllers.HTTP.Enabled {
		log := root.With("controller", "http")
		srv := httpctrl.New(log, th, cfg.Controllers.HTTP.Addr, deviceID)
		go func() {
			log.Info("controller started", "addr", cfg.Controllers.HTTP.Addr)
			if err := srv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("controller exited", "err", err)
				cancel()
			}
		}()
	}

	if cfg.Controllers.MQTT.Enabled {
		log := root.With("controller", "mqtt")
		mc, err := mqttctrl.New(log, th, mqttctrl.Config{
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
			root.Error("mqtt init failed", "err", err)
			os.Exit(1)
		}

		go func() {
			log.Info("controller started",
				"broker", cfg.Controllers.MQTT.Addr,
				"base_topic", cfg.Controllers.MQTT.BaseTopic,
			)
			if err := mc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("controller exited", "err", err)
				cancel()
			}
		}()
	}

	if cfg.Controllers.MODBUS.Enabled {
		log := root.With("controller", "modbus")
		mc, err := modbusctrl.New(log, th, modbusctrl.Config{
			DeviceID:      deviceID,
			Addr:          cfg.Controllers.MODBUS.Addr,
			UnitID:        cfg.Controllers.MODBUS.UnitID,
			SyncInterval:  cfg.Controllers.MODBUS.SyncInterval,
			RegisterCount: cfg.Controllers.MODBUS.RegisterCount,
		})
		if err != nil {
			root.Error("modbus init failed", "err", err)
			os.Exit(1)
		}
		go func() {
			log.Info("controller started",
				"addr", cfg.Controllers.MODBUS.Addr,
				"unit_id", cfg.Controllers.MODBUS.UnitID,
			)
			if err := mc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("controller exited", "err", err)
				cancel()
			}
		}()
	}

	if cfg.Controllers.BACNET.Enabled {
		log := root.With("controller", "bacnet")
		bc, err := bacnetctrl.New(log, th, bacnetctrl.Config{
			DeviceID:       deviceID,
			Addr:           cfg.Controllers.BACNET.Addr,
			DeviceInstance: cfg.Controllers.BACNET.DeviceInstance,
			SyncInterval:   cfg.Controllers.BACNET.SyncInterval,
		})
		if err != nil {
			root.Error("bacnet init failed", "err", err)
			os.Exit(1)
		}
		go func() {
			log.Info("controller started",
				"addr", cfg.Controllers.BACNET.Addr,
				"device_instance", cfg.Controllers.BACNET.DeviceInstance,
			)
			if err := bc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("controller exited", "err", err)
				cancel()
			}
		}()
	}

	if cfg.Controllers.KNX.Enabled {
		log := root.With("controller", "knx")
		kc, err := knxctrl.New(log, th, knxctrl.Config{
			DeviceID:        deviceID,
			Addr:            cfg.Controllers.KNX.Addr,
			PublishInterval: cfg.Controllers.KNX.PublishInterval,
			GAMain:          cfg.Controllers.KNX.GAMain,
			GAMiddle:        cfg.Controllers.KNX.GAMiddle,
		})
		if err != nil {
			root.Error("knx init failed", "err", err)
			os.Exit(1)
		}
		go func() {
			log.Info("controller started", "addr", cfg.Controllers.KNX.Addr)
			if err := kc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("controller exited", "err", err)
				cancel()
			}
		}()
	}

	// Block until shutdown.
	<-ctx.Done()
	root.Info("shutting down")
}
