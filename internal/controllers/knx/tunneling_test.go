package knxctrl

import (
	"testing"
)

func TestParseCEMI_GroupValueRead(t *testing.T) {
	// L_Data.req, no additional info, ctrl1=0xB0, ctrl2=0xE0,
	// src=0x0000, dst=0x0801 (1/0/1), dataLen=1, APCI=0x0000 (read)
	data := []byte{
		0x11, 0x00, // msgCode=L_Data.req, addInfoLen=0
		0xB0, 0xE0, // ctrl1, ctrl2
		0x00, 0x00, // src addr
		0x08, 0x01, // dst addr (1/0/1)
		0x01,       // dataLen=1 (APCI only, compact)
		0x00, 0x00, // APCI: GroupValueRead
	}
	c, err := ParseCEMI(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.MsgCode != CEMIMsgCodeLDataReq {
		t.Fatalf("msgCode: got 0x%02X, want 0x11", c.MsgCode)
	}
	if c.DstAddr != 0x0801 {
		t.Fatalf("dst: got 0x%04X, want 0x0801", c.DstAddr)
	}
	if c.APCI != APCIGroupValueRead {
		t.Fatalf("APCI: got 0x%04X, want 0x0000", c.APCI)
	}
}

func TestParseCEMI_GroupValueWrite_Compact(t *testing.T) {
	// Write DPT 1.001: value=1 encoded in low bits of APCI.
	data := []byte{
		0x11, 0x00,
		0xB0, 0xE0,
		0x00, 0x00,
		0x08, 0x00, // dst=1/0/0
		0x01,       // dataLen=1 (compact)
		0x00, 0x81, // APCI: GroupValueWrite (0x0080) | data=1
	}
	c, err := ParseCEMI(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.APCI != APCIGroupValueWrite {
		t.Fatalf("APCI: got 0x%04X, want 0x0080", c.APCI)
	}
	if !c.IsCompact {
		t.Fatal("expected compact encoding")
	}
	if len(c.Data) != 1 || c.Data[0] != 1 {
		t.Fatalf("data: got %v, want [1]", c.Data)
	}
}

func TestParseCEMI_GroupValueWrite_Extended(t *testing.T) {
	// Write DPT 9.001: 2-byte data.
	data := []byte{
		0x11, 0x00,
		0xB0, 0xE0,
		0x00, 0x00,
		0x08, 0x01, // dst=1/0/1
		0x03,       // dataLen=3 (APCI byte + 2 data bytes)
		0x00, 0x80, // APCI: GroupValueWrite
		0x0C, 0x8C, // DPT 9.001 payload
	}
	c, err := ParseCEMI(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.APCI != APCIGroupValueWrite {
		t.Fatalf("APCI: got 0x%04X", c.APCI)
	}
	if c.IsCompact {
		t.Fatal("expected extended encoding")
	}
	if len(c.Data) != 2 || c.Data[0] != 0x0C || c.Data[1] != 0x8C {
		t.Fatalf("data: got %v, want [0x0C, 0x8C]", c.Data)
	}
}

func TestBuildCEMIGroupValueResponse_Compact(t *testing.T) {
	cemi := BuildCEMIGroupValueResponse(0x0000, 0x0800, []byte{1}, true)
	c, err := ParseCEMI(cemi)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if c.MsgCode != CEMIMsgCodeLDataInd {
		t.Fatalf("msgCode: got 0x%02X", c.MsgCode)
	}
	if c.DstAddr != 0x0800 {
		t.Fatalf("dst: got 0x%04X", c.DstAddr)
	}
	if c.APCI != APCIGroupValueResponse {
		t.Fatalf("APCI: got 0x%04X", c.APCI)
	}
	if !c.IsCompact || len(c.Data) != 1 || c.Data[0] != 1 {
		t.Fatalf("data: got compact=%v, data=%v", c.IsCompact, c.Data)
	}
}

func TestBuildCEMIGroupValueResponse_Extended(t *testing.T) {
	payload := []byte{0x0C, 0x8C}
	cemi := BuildCEMIGroupValueResponse(0x0000, 0x0801, payload, false)
	c, err := ParseCEMI(cemi)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if c.APCI != APCIGroupValueResponse {
		t.Fatalf("APCI: got 0x%04X", c.APCI)
	}
	if c.IsCompact {
		t.Fatal("expected extended")
	}
	if len(c.Data) != 2 || c.Data[0] != 0x0C || c.Data[1] != 0x8C {
		t.Fatalf("data: got %v", c.Data)
	}
}

func TestMarshalTunnelingACK(t *testing.T) {
	pkt := MarshalTunnelingACK(1, 5, 0)
	h, err := ParseHeader(pkt)
	if err != nil {
		t.Fatalf("header parse error: %v", err)
	}
	if h.ServiceType != ServiceTunnelingACK {
		t.Fatalf("service type: got 0x%04X", h.ServiceType)
	}
	ch, seq, err := ParseTunnelingHeader(pkt[headerSize:])
	if err != nil {
		t.Fatalf("tunneling header parse error: %v", err)
	}
	if ch != 1 || seq != 5 {
		t.Fatalf("got ch=%d seq=%d", ch, seq)
	}
}

func TestMarshalTunnelingRequest(t *testing.T) {
	cemi := []byte{0x29, 0x00, 0xB0, 0xE0, 0x00, 0x00, 0x08, 0x00, 0x01, 0x00, 0x41}
	pkt := MarshalTunnelingRequest(1, 3, cemi)
	h, err := ParseHeader(pkt)
	if err != nil {
		t.Fatalf("header parse error: %v", err)
	}
	if h.ServiceType != ServiceTunnelingRequest {
		t.Fatalf("service type: got 0x%04X", h.ServiceType)
	}
	if int(h.TotalLength) != len(pkt) {
		t.Fatalf("total length mismatch: header says %d, actual %d", h.TotalLength, len(pkt))
	}
}
