package modbusctrl

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	mbserver "github.com/tbrandon/mbserver"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

// Config for the Modbus controller.
type Config struct {
	DeviceID string
	Addr     string
	UnitID   byte // UnitID (Modbus slave/unit ID). Use an integer 1..247.
	// How frequently to copy the thermostat snapshot into mbserver memory for reads.
	// If your service never changes except via Modbus, you can set this to 0 to disable.
	SyncInterval time.Duration
}

type Controller struct {
	svc ports.ThermostatService
	cfg Config

	serv *mbserver.Server
}

func New(svc ports.ThermostatService, cfg Config) (*Controller, error) {
	if cfg.UnitID == 0 {
		return nil, errors.New("modbus: UnitID is required (non-zero)")
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:1502"
	}
	if cfg.SyncInterval == 0 {
		cfg.SyncInterval = 1 * time.Second
	}
	return &Controller{svc: svc, cfg: cfg}, nil
}

// Run starts the Modbus server and registers handlers that apply writes immediately.
// It blocks until ctx is canceled.
func (c *Controller) Run(ctx context.Context) error {
	serv := mbserver.NewServer()
	c.serv = serv

	if err := serv.ListenTCP(c.cfg.Addr); err != nil {
		return fmt.Errorf("mbserver listen tcp %s: %w", c.cfg.Addr, err)
	}
	serv.RegisterFunctionHandler(5, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		addr := binary.BigEndian.Uint16(data[0:2])
		value := binary.BigEndian.Uint16(data[2:4])

		// Only accept coil 0 for enabled
		if addr != 0 {
			return []byte{}, &mbserver.IllegalDataAddress
		}

		var enabled bool
		switch value {
		case 0x0000:
			enabled = false
		case 0xFF00:
			enabled = true
		default:
			return []byte{}, &mbserver.IllegalDataValue
		}

		// Apply to service synchronously
		c.svc.SetEnabled(enabled)

		// Update server memory to reflect final service state (svc may have side-effects)
		c.writeSnapshotToServer(c.svc.Get())

		// echo request (address + value)
		resp := make([]byte, 4)
		copy(resp, data[0:4])
		return resp, &mbserver.Success
	})

	// Write Single Holding Register (function 6)
	serv.RegisterFunctionHandler(6, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		addr := binary.BigEndian.Uint16(data[0:2])
		value := binary.BigEndian.Uint16(data[2:4])

		switch addr {
		case 0:
			// setpoint
			v := decodeTemp(value)
			if err := c.svc.SetSetpoint(v); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 1:
			// setpoint min
			cur := c.svc.Get()
			v := decodeTemp(value)
			if err := c.svc.SetMinMax(v, cur.TemperatureSetpointMax); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 2:
			// setpoint max
			cur := c.svc.Get()
			v := decodeTemp(value)
			if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, v); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 3:
			// mode (enum)
			m := thermostat.Mode(value)
			if err := c.svc.SetMode(m); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 4:
			// fan_speed
			f := thermostat.FanSpeed(value)
			if err := c.svc.SetFanSpeed(f); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		default:
			return []byte{}, &mbserver.IllegalDataAddress
		}

		// success: update server memory and echo the request
		c.writeSnapshotToServer(c.svc.Get())
		resp := make([]byte, 4)
		copy(resp, data[0:4])
		return resp, &mbserver.Success
	})

	// Write Multiple Registers (function 16) - support writing a block of HRs
	serv.RegisterFunctionHandler(16, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		d := frame.GetData()
		// data layout: start addr (2) | quantity (2) | bytecount (1) | bytes...
		if len(d) < 5 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		start := binary.BigEndian.Uint16(d[0:2])
		quantity := binary.BigEndian.Uint16(d[2:4])
		byteCount := int(d[4])
		if byteCount != int(quantity)*2 || len(d) < 5+byteCount {
			return []byte{}, &mbserver.IllegalDataValue
		}
		// iterate registers and apply
		for i := 0; i < int(quantity); i++ {
			addr := int(start) + i
			val := binary.BigEndian.Uint16(d[5+i*2 : 5+i*2+2])
			switch addr {
			case 0:
				if err := c.svc.SetSetpoint(decodeTemp(val)); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
			case 1:
				cur := c.svc.Get()
				if err := c.svc.SetMinMax(decodeTemp(val), cur.TemperatureSetpointMax); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
			case 2:
				cur := c.svc.Get()
				if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, decodeTemp(val)); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
			case 3:
				if err := c.svc.SetMode(thermostat.Mode(val)); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
			case 4:
				if err := c.svc.SetFanSpeed(thermostat.FanSpeed(val)); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
			default:
				// illegal address
				return []byte{}, &mbserver.IllegalDataAddress
			}
		}

		// success: update server memory
		c.writeSnapshotToServer(c.svc.Get())

		// Response for Write Multiple Registers is start + quantity
		resp := make([]byte, 4)
		binary.BigEndian.PutUint16(resp[0:2], start)
		binary.BigEndian.PutUint16(resp[2:4], quantity)
		return resp, &mbserver.Success
	})

	// Periodically copy service snapshot to server memory so read requests reflect latest state.
	ticker := time.NewTicker(c.cfg.SyncInterval)
	defer func() {
		ticker.Stop()
		serv.Close()
	}()

	// initial snapshot
	c.writeSnapshotToServer(c.svc.Get())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.writeSnapshotToServer(c.svc.Get())
		}
	}
}

// writeSnapshotToServer writes the thermostat snapshot into mbserver memory.
func (c *Controller) writeSnapshotToServer(snap thermostat.Snapshot) {
	s := c.serv
	if s == nil {
		return
	}
	// coils: coil 0 stored in byte 0 LSB bit 0; mbserver stores raw coil bytes.
	if len(s.Coils) > 0 {
		// not used here
	}
	// set coil 0 (enabled)
	if len(s.Coils) > 0 {
		if snap.Enabled {
			s.Coils[0] = 0x01
		} else {
			s.Coils[0] = 0x00
		}
	}

	// Holding registers
	if len(s.HoldingRegisters) >= 5 {
		s.HoldingRegisters[0] = encodeTemp(snap.TemperatureSetpoint)
		s.HoldingRegisters[1] = encodeTemp(snap.TemperatureSetpointMin)
		s.HoldingRegisters[2] = encodeTemp(snap.TemperatureSetpointMax)
		s.HoldingRegisters[3] = uint16(snap.Mode)
		s.HoldingRegisters[4] = uint16(snap.FanSpeed)
	} else {
		// be defensive in case mbserver layout changed; try to set what is present
		if len(s.HoldingRegisters) > 0 {
			s.HoldingRegisters[0] = encodeTemp(snap.TemperatureSetpoint)
		}
		if len(s.HoldingRegisters) > 1 {
			s.HoldingRegisters[1] = encodeTemp(snap.TemperatureSetpointMin)
		}
		if len(s.HoldingRegisters) > 2 {
			s.HoldingRegisters[2] = encodeTemp(snap.TemperatureSetpointMax)
		}
		if len(s.HoldingRegisters) > 3 {
			s.HoldingRegisters[3] = uint16(snap.Mode)
		}
		if len(s.HoldingRegisters) > 4 {
			s.HoldingRegisters[4] = uint16(snap.FanSpeed)
		}
	}

	// Input registers: ambient temperature at IR 0
	if len(s.InputRegisters) > 0 {
		s.InputRegisters[0] = encodeTemp(snap.AmbientTemperature)
	}
}

const TemperatureScale int = 100

func encodeTemp(v float64) uint16 {
	r := min(max(int(math.Round(v*float64(TemperatureScale))), math.MinInt16), math.MaxInt16)
	return uint16(int16(r))
}

func decodeTemp(u uint16) float64 {
	i := int16(u)
	return float64(i) / float64(TemperatureScale)
}
