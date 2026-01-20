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
	// SyncInterval retained in config to preserve API but unused when reads are handled by custom handlers.
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
	// SyncInterval is optional; no polling is required because reads are handled directly.
	return &Controller{svc: svc, cfg: cfg}, nil
}

// Run starts the Modbus server and registers handlers that apply writes immediately and
// provide reads directly from the thermostat service. It blocks until ctx is canceled.
func (c *Controller) Run(ctx context.Context) error {
	serv := mbserver.NewServer()
	c.serv = serv

	// Register handlers BEFORE starting the TCP listener to avoid races inside mbserver
	// between handler registration and the server's goroutines.
	// Read Coils (function 1) - return current enabled state.
	serv.RegisterFunctionHandler(1, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		start := binary.BigEndian.Uint16(data[0:2])
		qty := binary.BigEndian.Uint16(data[2:4])
		if qty == 0 || qty > 2000 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		// We only expose coil 0 (enabled)
		if start != 0 || qty != 1 {
			return []byte{}, &mbserver.IllegalDataAddress
		}
		snap := c.svc.Get()
		coilByte := byte(0)
		if snap.Enabled {
			coilByte = 0x01
		}
		// response: byte count (1) + coil bytes
		return []byte{1, coilByte}, &mbserver.Success
	})

	// Read Holding Registers (function 3) - expose HR 0..4 from service snapshot.
	serv.RegisterFunctionHandler(3, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		start := int(binary.BigEndian.Uint16(data[0:2]))
		qty := int(binary.BigEndian.Uint16(data[2:4]))
		if qty == 0 || qty > 125 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		// We support addresses 0..4
		if start < 0 || start+qty > 5 {
			return []byte{}, &mbserver.IllegalDataAddress
		}
		snap := c.svc.Get()
		// Build response: byte count + register bytes
		regs := make([]uint16, 0, qty)
		for i := 0; i < qty; i++ {
			addr := start + i
			switch addr {
			case 0:
				regs = append(regs, encodeTemp(snap.TemperatureSetpoint))
			case 1:
				regs = append(regs, encodeTemp(snap.TemperatureSetpointMin))
			case 2:
				regs = append(regs, encodeTemp(snap.TemperatureSetpointMax))
			case 3:
				regs = append(regs, uint16(snap.Mode))
			case 4:
				regs = append(regs, uint16(snap.FanSpeed))
			default:
				return []byte{}, &mbserver.IllegalDataAddress
			}
		}
		byteCount := len(regs) * 2
		resp := make([]byte, 1+byteCount)
		resp[0] = byte(byteCount)
		for i, r := range regs {
			binary.BigEndian.PutUint16(resp[1+i*2:1+i*2+2], r)
		}
		return resp, &mbserver.Success
	})

	// Read Input Registers (function 4) - expose IR 0 (ambient temperature).
	serv.RegisterFunctionHandler(4, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		start := int(binary.BigEndian.Uint16(data[0:2]))
		qty := int(binary.BigEndian.Uint16(data[2:4]))
		if qty == 0 || qty > 125 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		if start < 0 || start+qty > 1 {
			return []byte{}, &mbserver.IllegalDataAddress
		}
		snap := c.svc.Get()
		// Only IR 0 is present
		if qty != 1 || start != 0 {
			return []byte{}, &mbserver.IllegalDataAddress
		}
		val := encodeTemp(snap.AmbientTemperature)
		resp := make([]byte, 1+2)
		resp[0] = 2
		binary.BigEndian.PutUint16(resp[1:3], val)
		return resp, &mbserver.Success
	})

	// Write Single Coil (function 5) - enabled
	serv.RegisterFunctionHandler(5, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		addr := binary.BigEndian.Uint16(data[0:2])
		value := binary.BigEndian.Uint16(data[2:4])

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

		c.svc.SetEnabled(enabled)

		// echo request (address + value)
		resp := make([]byte, 4)
		copy(resp, data[0:4])
		return resp, &mbserver.Success
	})

	// Write Single Register (function 6)
	serv.RegisterFunctionHandler(6, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		data := frame.GetData()
		if len(data) < 4 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		addr := binary.BigEndian.Uint16(data[0:2])
		value := binary.BigEndian.Uint16(data[2:4])

		switch addr {
		case 0:
			if err := c.svc.SetSetpoint(decodeTemp(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 1:
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(decodeTemp(value), cur.TemperatureSetpointMax); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 2:
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, decodeTemp(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 3:
			if err := c.svc.SetMode(thermostat.Mode(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case 4:
			if err := c.svc.SetFanSpeed(thermostat.FanSpeed(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		default:
			return []byte{}, &mbserver.IllegalDataAddress
		}

		resp := make([]byte, 4)
		copy(resp, data[0:4])
		return resp, &mbserver.Success
	})

	// Write Multiple Registers (function 16)
	serv.RegisterFunctionHandler(16, func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
		d := frame.GetData()
		if len(d) < 5 {
			return []byte{}, &mbserver.IllegalDataValue
		}
		start := binary.BigEndian.Uint16(d[0:2])
		quantity := binary.BigEndian.Uint16(d[2:4])
		byteCount := int(d[4])
		if byteCount != int(quantity)*2 || len(d) < 5+byteCount {
			return []byte{}, &mbserver.IllegalDataValue
		}
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
				return []byte{}, &mbserver.IllegalDataAddress
			}
		}

		resp := make([]byte, 4)
		binary.BigEndian.PutUint16(resp[0:2], start)
		binary.BigEndian.PutUint16(resp[2:4], quantity)
		return resp, &mbserver.Success
	})

	// Now start listening after all handlers are registered.
	if err := serv.ListenTCP(c.cfg.Addr); err != nil {
		return fmt.Errorf("mbserver listen tcp %s: %w", c.cfg.Addr, err)
	}

	// Block until ctx.Done()
	<-ctx.Done()
	serv.Close()
	return ctx.Err()
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
