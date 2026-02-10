package bacnetctrl

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/ulbios/bacnet"
	"github.com/ulbios/bacnet/plumbing"
	"github.com/ulbios/bacnet/services"
)

// Config for the BACnet controller.
type Config struct {
	// Identity
	DeviceID       string
	DeviceInstance int // BACnet device instance number (0..4194303)

	// Network
	Addr string // bind address, e.g. "0.0.0.0:47808" or "127.0.0.1:0" for ephemeral port in tests

	// Behavior
	SyncInterval time.Duration // retained for parity; unused by WhoIs handling
}

type Controller struct {
	svc ports.ThermostatService
	cfg Config

	mu   sync.Mutex
	conn net.PacketConn
}

// New creates a BACnet controller. DeviceInstance must be in the BACnet range 0..4194303.
func New(svc ports.ThermostatService, cfg Config) (*Controller, error) {
	if cfg.DeviceID == "" {
		return nil, errors.New("bacnet: DeviceID is required")
	}
	if cfg.DeviceInstance < 0 || cfg.DeviceInstance > 4194303 {
		return nil, errors.New("bacnet: DeviceInstance must be in range 0..4194303")
	}
	if cfg.Addr == "" {
		cfg.Addr = "0.0.0.0:47808"
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 1 * time.Second
	}
	return &Controller{svc: svc, cfg: cfg}, nil
}

// Run starts a BACnet/IP listener and responds to Who-Is messages with I-Am.
// It blocks until ctx is canceled.
func (c *Controller) Run(ctx context.Context) error {
	ln, err := net.ListenPacket("udp4", c.cfg.Addr)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = ln
	c.mu.Unlock()

	// ensure connection is closed when function returns
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	// Read loop goroutine
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 2048)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				// If the listener closed due to context cancel, exit loop.
				select {
				case <-ctx.Done():
					return
				default:
					// transient read error: log and continue.
					log.Printf("bacnet read error: %v", err)
					continue
				}
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			peek := data
			if len(peek) > 32 {
				peek = peek[:32]
			}
			log.Printf("bacnet: received %d bytes from %s: % x", n, addr.String(), peek)
			// Parse the incoming BACnet message. Ignore parse errors (malformed packets).
			pkt, err := bacnet.Parse(data)
			if err != nil {
				// ignore non-BACnet or malformed packets
				log.Printf("bacnet: parse error from %s: %v (raw: % x)", addr.String(), err, peek)
				continue
			}

			// respond to WhoIs
			switch pkt.(type) {
			case *services.UnconfirmedWhoIs:
				log.Printf("bacnet: Who-Is received from %s — replying I-Am (device instance %d)", addr.String(), c.cfg.DeviceInstance)
				if err := c.respondIAm(addr); err != nil {
					log.Printf("bacnet: failed to send I-Am to %s: %v", addr.String(), err)
				}
			default:
				// other services not handled here
				log.Printf("bacnet: received unsupported service type from %s: %T", addr.String(), pkt)

			}
		}
	}()

	// Wait until context done, then close listener and wait for read loop to finish.
	<-ctx.Done()
	_ = ln.Close()
	<-readDone
	return ctx.Err()
}

// respondIAm builds an Unconfirmed I-Am and sends it to the given addr.
// Uses DeviceInstance from config. Sends as BVLC unicast to the provided addr.
func (c *Controller) respondIAm(addr net.Addr) error {
	fmt.Println("responding to IAm")

	// Build APDU objects with configured device instance.
	objs := services.IAmObjects(uint32(c.cfg.DeviceInstance), 1024, 0, 0) // maxAPDU=1024, segmentation=0, vendor=0
	apdu := plumbing.NewAPDU(plumbing.UnConfirmedReq, services.ServiceUnconfirmedIAm, objs)

	// BVLC: use unicast for reply to sender.
	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	// NPDU: minimal NPDU (no extra specifiers)
	npdu := plumbing.NewNPDU(false, false, false, false)

	iam := &services.UnconfirmedIAm{
		BVLC: bvlc,
		NPDU: npdu,
		APDU: apdu,
	}
	iam.SetLength()

	b, err := iam.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("bacnet: connection closed")
	}
	_, err = c.conn.WriteTo(b, addr)
	return err
}

// LocalAddr returns the controller's bound address (useful for tests).
// Returns nil if not running.
func (c *Controller) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.LocalAddr()
}
