package knxctrl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

const (
	heartbeatTimeout         = 120 * time.Second
	heartbeatInterval        = 30 * time.Second
	statusOK          uint8  = 0x00
	statusNoMore      uint8  = 0x24   // E_NO_MORE_CONNECTIONS
	statusConnID      uint8  = 0x21   // E_CONNECTION_ID
	individualAddr    uint16 = 0x1101 // 1.1.1 — assigned to tunnel client
)

type clientState struct {
	channelID  uint8
	addr       *net.UDPAddr
	seqCounter uint8 // server → client sequence
	lastSeen   time.Time
}

// Controller implements a KNXnet/IP tunneling server.
type Controller struct {
	svc      ports.ThermostatService
	cfg      Config
	bindings map[uint16]Binding
	log      *slog.Logger

	mu     sync.Mutex
	conn   net.PacketConn
	client *clientState // nil = no active connection
}

// New creates a new KNX controller.
func New(logger *slog.Logger, svc ports.ThermostatService, cfg Config) (*Controller, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if cfg.Addr == "" {
		cfg.Addr = "0.0.0.0:3671"
	}
	if cfg.PublishInterval <= 0 {
		cfg.PublishInterval = 10 * time.Second
	}

	bindings, err := BuildBindingMap(cfg)
	if err != nil {
		return nil, fmt.Errorf("knx controller: %w", err)
	}

	return &Controller{
		svc:      svc,
		cfg:      cfg,
		bindings: bindings,
		log:      logger,
	}, nil
}

// Run starts the UDP server and blocks until ctx is cancelled.
func (c *Controller) Run(ctx context.Context) error {
	ln, err := net.ListenPacket("udp4", c.cfg.Addr)
	if err != nil {
		return fmt.Errorf("knx listen: %w", err)
	}

	c.mu.Lock()
	c.conn = ln
	c.mu.Unlock()

	defer ln.Close()

	// Heartbeat checker goroutine.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go c.heartbeatLoop(heartbeatCtx)

	// State push goroutine: polls thermostat and pushes changes to connected client.
	// Seed snapshot before starting goroutine to avoid racing with early writes.
	initialSnap := c.svc.Get()
	go c.stateLoop(heartbeatCtx, initialSnap)

	// Read loop in a goroutine so we can select on ctx.
	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					errCh <- err
				}
				return
			}
			c.handlePacket(buf[:n], addr)
		}
	}()

	select {
	case <-ctx.Done():
		ln.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// LocalAddr returns the bound address (useful for tests binding to :0).
func (c *Controller) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.LocalAddr()
}

func (c *Controller) handlePacket(data []byte, addr net.Addr) {
	hdr, err := ParseHeader(data)
	if err != nil {
		c.log.Warn("knx invalid header", "remote", addr.String(), "err", err)
		return
	}
	c.log.Debug("knx packet received",
		"remote", addr.String(),
		"service", fmt.Sprintf("0x%04X", hdr.ServiceType),
	)

	body := data[headerSize:]

	switch hdr.ServiceType {
	case ServiceConnectRequest:
		c.handleConnect(body, addr)
	case ServiceConnectionStateRequest:
		c.handleConnectionState(body, addr)
	case ServiceDisconnectRequest:
		c.handleDisconnect(body, addr)
	case ServiceTunnelingRequest:
		c.handleTunneling(body, addr)
	case ServiceTunnelingACK:
		// Client ACKs our pushed TUNNELING_REQUESTs — nothing to do.
	default:
		c.log.Warn("knx unsupported service",
			"remote", addr.String(),
			"service", fmt.Sprintf("0x%04X", hdr.ServiceType),
		)
	}
}

func (c *Controller) handleConnect(body []byte, addr net.Addr) {
	// CONNECT_REQUEST body: HPAI(control, 8) + HPAI(data, 8) + CRI(4)
	if len(body) < 20 {
		c.log.Warn("knx connect request too short", "remote", addr.String())
		return
	}

	controlHPAI, err := ParseHPAI(body[0:8])
	if err != nil {
		c.log.Warn("knx bad control HPAI", "err", err)
		return
	}

	clientAddr := c.resolveHPAI(controlHPAI, addr)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		// Already connected — reject.
		c.sendConnectResponse(statusNoMore, clientAddr)
		return
	}

	c.client = &clientState{
		channelID:  1,
		addr:       clientAddr,
		seqCounter: 0,
		lastSeen:   time.Now(),
	}

	c.sendConnectResponse(statusOK, clientAddr)
}

func (c *Controller) sendConnectResponse(status uint8, addr *net.UDPAddr) {
	// CONNECT_RESPONSE: status(1), channelID(1), HPAI(data endpoint, 8), CRD(4)
	var channelID uint8
	if status == statusOK {
		channelID = c.client.channelID
	}

	// Data endpoint HPAI: use 0.0.0.0:0 (route-back / NAT mode) so the client
	// sends tunneling data to the same address it connected to. This is required
	// for Docker port-mapped deployments where the container's internal IP is
	// not reachable from the host.
	dataHPAI := MarshalHPAINAT()

	// CRD: Connection Response Data block — tunnel connection.
	crd := []byte{0x04, ConnTypeTunnel, byte(individualAddr >> 8), byte(individualAddr & 0xFF)}

	bodyLen := 1 + 1 + len(dataHPAI) + len(crd)
	resp := MarshalHeader(ServiceConnectResponse, headerSize+bodyLen)
	resp = append(resp, channelID, status)
	resp = append(resp, dataHPAI...)
	resp = append(resp, crd...)

	c.send(resp, addr)
}

func (c *Controller) handleConnectionState(body []byte, addr net.Addr) {
	if len(body) < 2 {
		return
	}
	channelID := body[0]
	// body[1] is reserved

	c.mu.Lock()
	defer c.mu.Unlock()

	status := statusOK
	if c.client == nil || c.client.channelID != channelID {
		status = statusConnID
	} else {
		c.client.lastSeen = time.Now()
	}

	// CONNECTIONSTATE_RESPONSE: channelID(1) + status(1)
	resp := MarshalHeader(ServiceConnectionStateResponse, headerSize+2)
	resp = append(resp, channelID, status)

	udpAddr := c.resolveHPAI(HPAI{}, addr) // use source addr
	c.send(resp, udpAddr)
}

func (c *Controller) handleDisconnect(body []byte, addr net.Addr) {
	if len(body) < 2 {
		return
	}
	channelID := body[0]

	c.mu.Lock()
	defer c.mu.Unlock()

	status := statusOK
	if c.client == nil || c.client.channelID != channelID {
		status = statusConnID
	} else {
		c.client = nil
	}

	// DISCONNECT_RESPONSE: channelID(1) + status(1)
	resp := MarshalHeader(ServiceDisconnectResponse, headerSize+2)
	resp = append(resp, channelID, status)

	udpAddr := c.resolveHPAI(HPAI{}, addr)
	c.send(resp, udpAddr)
}

func (c *Controller) handleTunneling(body []byte, _ net.Addr) {
	channelID, seq, err := ParseTunnelingHeader(body)
	if err != nil {
		c.log.Warn("knx bad tunneling header", "err", err)
		return
	}

	c.mu.Lock()
	if c.client == nil || c.client.channelID != channelID {
		c.mu.Unlock()
		return
	}
	c.client.lastSeen = time.Now()
	clientAddr := c.client.addr
	c.mu.Unlock()

	// Always ACK first.
	ack := MarshalTunnelingACK(channelID, seq, statusOK)
	c.send(ack, clientAddr)

	// Parse CEMI from body after the 4-byte tunneling header.
	cemiData := body[4:]
	cemi, err := ParseCEMI(cemiData)
	if err != nil {
		c.log.Warn("knx bad cemi", "err", err)
		return
	}

	// Send L_Data.con to confirm delivery to the virtual KNX bus (KNX spec 03/06/03 §4.1.5).
	if cemi.MsgCode == CEMIMsgCodeLDataReq {
		conCEMI := BuildCEMILDataCon(cemiData)
		c.sendTunnelingRequest(conCEMI, clientAddr)
	}

	c.dispatchCEMI(cemi, clientAddr)
}

func (c *Controller) dispatchCEMI(cemi CEMI, clientAddr *net.UDPAddr) {
	ga := fmt.Sprintf("0x%04X", cemi.DstAddr)
	apci := fmt.Sprintf("0x%04X", cemi.APCI)
	c.log.Debug("knx cemi", "ga", ga, "apci", apci)

	binding, ok := c.bindings[cemi.DstAddr]
	if !ok {
		c.log.Warn("knx unknown group address", "ga", ga)
		return
	}

	switch cemi.APCI {
	case APCIGroupValueRead:
		snap := c.svc.Get()
		data := binding.Read(snap)
		compact := binding.DPTSize == 0
		responseCEMI := BuildCEMIGroupValueResponse(0x0000, cemi.DstAddr, data, compact)
		c.sendTunnelingRequest(responseCEMI, clientAddr)

	case APCIGroupValueWrite:
		if binding.Write == nil {
			c.log.Warn("knx write to read-only GA", "ga", ga)
			return
		}
		if err := binding.Write(c.svc, cemi.Data); err != nil {
			c.log.Warn("knx write failed", "ga", ga, "err", err)
		}

	default:
		c.log.Warn("knx unsupported APCI", "ga", ga, "apci", apci)
	}
}

func (c *Controller) sendTunnelingRequest(cemi []byte, addr *net.UDPAddr) {
	c.mu.Lock()
	if c.client == nil {
		c.mu.Unlock()
		return
	}
	seq := c.client.seqCounter
	c.client.seqCounter++
	c.mu.Unlock()

	pkt := MarshalTunnelingRequest(1, seq, cemi)
	c.send(pkt, addr)
}

func (c *Controller) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.client != nil && time.Since(c.client.lastSeen) > heartbeatTimeout {
				c.log.Warn("knx heartbeat timeout, dropping client")
				c.client = nil
			}
			c.mu.Unlock()
		}
	}
}

// stateLoop polls the thermostat state and pushes GroupValueWrite telegrams
// to the connected client when values change.
func (c *Controller) stateLoop(ctx context.Context, lastSnap thermostat.Snapshot) {
	ticker := time.NewTicker(c.cfg.PublishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			client := c.client
			c.mu.Unlock()
			if client == nil {
				continue
			}

			snap := c.svc.Get()
			c.pushChanges(lastSnap, snap, client.addr)
			lastSnap = snap
		}
	}
}

func (c *Controller) pushChanges(prev, cur thermostat.Snapshot, addr *net.UDPAddr) {
	for ga, binding := range c.bindings {
		prevData := binding.Read(prev)
		curData := binding.Read(cur)
		if !bytesEqual(prevData, curData) {
			compact := binding.DPTSize == 0
			cemi := BuildCEMIGroupValueWrite(0x0000, ga, curData, compact)
			c.sendTunnelingRequest(cemi, addr)
		}
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *Controller) resolveHPAI(hpai HPAI, fallback net.Addr) *net.UDPAddr {
	if !hpai.IsNAT() && hpai.Port != 0 {
		return &net.UDPAddr{IP: hpai.IP, Port: int(hpai.Port)}
	}
	if ua, ok := fallback.(*net.UDPAddr); ok {
		return ua
	}
	return nil
}

// send writes data to addr. conn is set once in Run() before the read loop
// starts, so it is safe to read without the mutex (which may already be held
// by the caller).
func (c *Controller) send(data []byte, addr *net.UDPAddr) {
	if c.conn == nil || addr == nil {
		return
	}
	if _, err := c.conn.WriteTo(data, addr); err != nil {
		c.log.Error("knx send failed", "remote", addr.String(), "err", err)
	}
}

// --- Helper for building connect request in tests ---

// BuildConnectRequest creates a CONNECT_REQUEST packet for testing.
func BuildConnectRequest(controlIP net.IP, controlPort uint16) []byte {
	controlHPAI := MarshalHPAI(controlIP, controlPort)
	dataHPAI := MarshalHPAI(controlIP, controlPort)
	// CRI: Connection Request Information — tunnel linklayer.
	cri := []byte{0x04, ConnTypeTunnel, 0x02, 0x00}

	bodyLen := len(controlHPAI) + len(dataHPAI) + len(cri)
	pkt := MarshalHeader(ServiceConnectRequest, headerSize+bodyLen)
	pkt = append(pkt, controlHPAI...)
	pkt = append(pkt, dataHPAI...)
	pkt = append(pkt, cri...)
	return pkt
}

// BuildConnectionStateRequest creates a CONNECTIONSTATE_REQUEST packet.
func BuildConnectionStateRequest(channelID uint8, controlIP net.IP, controlPort uint16) []byte {
	hpai := MarshalHPAI(controlIP, controlPort)
	pkt := MarshalHeader(ServiceConnectionStateRequest, headerSize+2+len(hpai))
	pkt = append(pkt, channelID, 0x00)
	pkt = append(pkt, hpai...)
	return pkt
}

// BuildDisconnectRequest creates a DISCONNECT_REQUEST packet.
func BuildDisconnectRequest(channelID uint8, controlIP net.IP, controlPort uint16) []byte {
	hpai := MarshalHPAI(controlIP, controlPort)
	pkt := MarshalHeader(ServiceDisconnectRequest, headerSize+2+len(hpai))
	pkt = append(pkt, channelID, 0x00)
	pkt = append(pkt, hpai...)
	return pkt
}

// BuildTunnelingGroupValueRead creates a TUNNELING_REQUEST with GroupValueRead CEMI.
func BuildTunnelingGroupValueRead(channelID, seq uint8, ga uint16) []byte {
	// CEMI: L_Data.req, no add info, ctrl1=0xB0, ctrl2=0xE0, src=0x0000, dst=ga, dataLen=1, APCI=0x0000
	cemi := []byte{
		CEMIMsgCodeLDataReq, 0x00,
		0xB0, 0xE0,
		0x00, 0x00,
		byte(ga >> 8), byte(ga),
		0x01,       // dataLen=1
		0x00, 0x00, // APCI: GroupValueRead
	}
	return MarshalTunnelingRequest(channelID, seq, cemi)
}

// BuildTunnelingGroupValueWrite creates a TUNNELING_REQUEST with GroupValueWrite CEMI.
func BuildTunnelingGroupValueWrite(channelID, seq uint8, ga uint16, data []byte, compact bool) []byte {
	var apdu []byte
	var dataLen byte
	if compact {
		apduHi := byte(APCIGroupValueWrite >> 8)
		apduLo := byte(APCIGroupValueWrite) | (data[0] & 0x3F)
		apdu = []byte{apduHi, apduLo}
		dataLen = 1
	} else {
		apdu = append([]byte{byte(APCIGroupValueWrite >> 8), byte(APCIGroupValueWrite)}, data...)
		dataLen = byte(len(apdu) - 1)
	}

	cemi := []byte{
		CEMIMsgCodeLDataReq, 0x00,
		0xB0, 0xE0,
		0x00, 0x00,
		byte(ga >> 8), byte(ga),
		dataLen,
	}
	cemi = append(cemi, apdu...)

	return MarshalTunnelingRequest(channelID, seq, cemi)
}

// ParseConnectResponse extracts channelID and status from a CONNECT_RESPONSE.
func ParseConnectResponse(data []byte) (channelID, status uint8, err error) {
	if len(data) < headerSize+2 {
		return 0, 0, errors.New("connect response too short")
	}
	return data[headerSize], data[headerSize+1], nil
}

// ParseTunnelingResponse extracts the CEMI from a TUNNELING_REQUEST (server→client).
func ParseTunnelingResponse(data []byte) (channelID, seq uint8, cemi []byte, err error) {
	if len(data) < headerSize+4 {
		return 0, 0, nil, errors.New("tunneling response too short")
	}
	ch, s, err := ParseTunnelingHeader(data[headerSize:])
	if err != nil {
		return 0, 0, nil, err
	}
	return ch, s, data[headerSize+4:], nil
}

// Helper to extract a GroupValueResponse value from a TUNNELING_REQUEST (server→client).
func ExtractGroupValueResponseData(data []byte) ([]byte, bool, error) {
	_, _, cemiData, err := ParseTunnelingResponse(data)
	if err != nil {
		return nil, false, err
	}
	cemi, err := ParseCEMI(cemiData)
	if err != nil {
		return nil, false, err
	}
	if cemi.APCI != APCIGroupValueResponse {
		return nil, false, fmt.Errorf("expected GroupValueResponse, got APCI 0x%04X", cemi.APCI)
	}
	return cemi.Data, cemi.IsCompact, nil
}

// DecodeResponseFloat reads a DPT 9.001 value from a tunneling response packet.
func DecodeResponseFloat(data []byte) (float64, error) {
	raw, _, err := ExtractGroupValueResponseData(data)
	if err != nil {
		return 0, err
	}
	if len(raw) != 2 {
		return 0, fmt.Errorf("expected 2 bytes, got %d", len(raw))
	}
	return DecodeDPT9([2]byte{raw[0], raw[1]}), nil
}

// DecodeResponseByte reads a 1-byte DPT value from a tunneling response packet.
func DecodeResponseByte(data []byte) (byte, error) {
	raw, _, err := ExtractGroupValueResponseData(data)
	if err != nil {
		return 0, err
	}
	if len(raw) < 1 {
		return 0, errors.New("empty response data")
	}
	return raw[0], nil
}

// DecodeResponseBool reads a DPT 1.001 compact value from a tunneling response packet.
func DecodeResponseBool(data []byte) (bool, error) {
	raw, compact, err := ExtractGroupValueResponseData(data)
	if err != nil {
		return false, err
	}
	if !compact || len(raw) < 1 {
		return false, errors.New("expected compact DPT 1.001")
	}
	return DecodeDPT1(raw[0]), nil
}
