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
	SyncInterval  time.Duration
	RegisterCount int // 1 = 16-bit (int16*100), 2 = 32-bit (IEEE 754 float32 across 2 registers)
}

// Holding register base addresses (spaced by 2 so each can hold up to 2 registers in 32-bit mode).
const (
	hrSetpoint    = 0
	hrSetpointMin = 2
	hrSetpointMax = 4
	hrMode        = 6
	hrFanSpeed    = 8
	hrTotal       = 10 // register space size

	irAmbient = 0
	irTotal   = 2
)

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
	if cfg.RegisterCount == 0 {
		cfg.RegisterCount = 1
	}
	if cfg.RegisterCount != 1 && cfg.RegisterCount != 2 {
		return nil, fmt.Errorf("modbus: RegisterCount must be 1 or 2, got %d", cfg.RegisterCount)
	}
	return &Controller{svc: svc, cfg: cfg}, nil
}

// encodeTempToRegs encodes a temperature float64 into a pair of uint16 registers.
// In 16-bit mode (RegisterCount=1), hi = int16*100, lo = 0.
// In 32-bit mode (RegisterCount=2), the pair is the big-endian IEEE 754 float32 split across two registers.
func (c *Controller) encodeTempToRegs(v float64) (hi, lo uint16) {
	if c.cfg.RegisterCount == 2 {
		u := math.Float32bits(float32(v))
		hi = uint16(u >> 16)
		lo = uint16(u & 0xFFFF)
		return hi, lo
	}
	return encodeTemp(v), 0
}

// decodeTempFromRegs decodes a temperature from a pair of uint16 registers.
func (c *Controller) decodeTempFromRegs(hi, lo uint16) float64 {
	if c.cfg.RegisterCount == 2 {
		u := uint32(hi)<<16 | uint32(lo)
		return float64(math.Float32frombits(u))
	}
	return decodeTemp(hi)
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

	// Read Holding Registers (function 3) - expose HR 0..hrTotal-1 from service snapshot.
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
		if start < 0 || start+qty > hrTotal {
			return []byte{}, &mbserver.IllegalDataAddress
		}
		snap := c.svc.Get()

		// Build full register map
		var regs [hrTotal]uint16
		regs[hrSetpoint], regs[hrSetpoint+1] = c.encodeTempToRegs(snap.TemperatureSetpoint)
		regs[hrSetpointMin], regs[hrSetpointMin+1] = c.encodeTempToRegs(snap.TemperatureSetpointMin)
		regs[hrSetpointMax], regs[hrSetpointMax+1] = c.encodeTempToRegs(snap.TemperatureSetpointMax)
		regs[hrMode] = uint16(snap.Mode)
		regs[hrFanSpeed] = uint16(snap.FanSpeed)

		// Serve the requested slice
		byteCount := qty * 2
		resp := make([]byte, 1+byteCount)
		resp[0] = byte(byteCount)
		for i := 0; i < qty; i++ {
			binary.BigEndian.PutUint16(resp[1+i*2:1+i*2+2], regs[start+i])
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
		if start < 0 || start+qty > irTotal {
			return []byte{}, &mbserver.IllegalDataAddress
		}
		snap := c.svc.Get()

		var regs [irTotal]uint16
		regs[irAmbient], regs[irAmbient+1] = c.encodeTempToRegs(snap.AmbientTemperature)

		byteCount := qty * 2
		resp := make([]byte, 1+byteCount)
		resp[0] = byte(byteCount)
		for i := 0; i < qty; i++ {
			binary.BigEndian.PutUint16(resp[1+i*2:1+i*2+2], regs[start+i])
		}
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
		case hrSetpoint:
			if c.cfg.RegisterCount == 2 {
				return []byte{}, &mbserver.IllegalDataAddress
			}
			if err := c.svc.SetSetpoint(decodeTemp(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case hrSetpointMin:
			if c.cfg.RegisterCount == 2 {
				return []byte{}, &mbserver.IllegalDataAddress
			}
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(decodeTemp(value), cur.TemperatureSetpointMax); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case hrSetpointMax:
			if c.cfg.RegisterCount == 2 {
				return []byte{}, &mbserver.IllegalDataAddress
			}
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, decodeTemp(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case hrMode:
			if err := c.svc.SetMode(thermostat.Mode(value)); err != nil {
				return []byte{}, &mbserver.IllegalDataValue
			}
		case hrFanSpeed:
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
		start := int(binary.BigEndian.Uint16(d[0:2]))
		quantity := int(binary.BigEndian.Uint16(d[2:4]))
		byteCount := int(d[4])
		if byteCount != quantity*2 || len(d) < 5+byteCount {
			return []byte{}, &mbserver.IllegalDataValue
		}

		// Helper to read register value at position in the data payload.
		regAt := func(pos int) uint16 {
			return binary.BigEndian.Uint16(d[5+pos*2 : 5+pos*2+2])
		}

		// Walk through the written range, consuming 1 or 2 registers per logical field.
		pos := 0
		for pos < quantity {
			addr := start + pos
			switch addr {
			case hrSetpoint, hrSetpointMin, hrSetpointMax:
				var temp float64
				if c.cfg.RegisterCount == 2 {
					if pos+2 > quantity {
						return []byte{}, &mbserver.IllegalDataValue
					}
					temp = c.decodeTempFromRegs(regAt(pos), regAt(pos+1))
					pos += 2
				} else {
					temp = decodeTemp(regAt(pos))
					pos++
				}
				switch addr {
				case hrSetpoint:
					if err := c.svc.SetSetpoint(temp); err != nil {
						return []byte{}, &mbserver.IllegalDataValue
					}
				case hrSetpointMin:
					cur := c.svc.Get()
					if err := c.svc.SetMinMax(temp, cur.TemperatureSetpointMax); err != nil {
						return []byte{}, &mbserver.IllegalDataValue
					}
				case hrSetpointMax:
					cur := c.svc.Get()
					if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, temp); err != nil {
						return []byte{}, &mbserver.IllegalDataValue
					}
				}
			case hrMode:
				if err := c.svc.SetMode(thermostat.Mode(regAt(pos))); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
				pos++
			case hrFanSpeed:
				if err := c.svc.SetFanSpeed(thermostat.FanSpeed(regAt(pos))); err != nil {
					return []byte{}, &mbserver.IllegalDataValue
				}
				pos++
			default:
				return []byte{}, &mbserver.IllegalDataAddress
			}
		}

		resp := make([]byte, 4)
		binary.BigEndian.PutUint16(resp[0:2], uint16(start))
		binary.BigEndian.PutUint16(resp[2:4], uint16(quantity))
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
