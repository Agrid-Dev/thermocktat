package knxctrl

import (
	"encoding/binary"
	"errors"
)

// CEMI message codes.
const (
	CEMIMsgCodeLDataReq uint8 = 0x11 // client → server
	CEMIMsgCodeLDataCon uint8 = 0x2E // server → client (data link layer confirmation)
	CEMIMsgCodeLDataInd uint8 = 0x29 // server → client (indication / unsolicited)
)

// APCI command masks (in the first 2 bytes of the APDU).
const (
	apciMask               uint16 = 0x03C0
	APCIGroupValueRead     uint16 = 0x0000
	APCIGroupValueResponse uint16 = 0x0040
	APCIGroupValueWrite    uint16 = 0x0080
)

// CEMI represents a parsed Common External Message Interface frame.
type CEMI struct {
	MsgCode   uint8
	SrcAddr   uint16
	DstAddr   uint16 // group address
	APCI      uint16
	IsCompact bool   // true if data is in the low 6 bits of the APCI byte (DPT 1.x)
	Data      []byte // payload data (may be empty for GroupValueRead)
}

// ParseCEMI parses a CEMI frame from raw bytes.
// Layout: [msgCode, addInfoLen, ctrl1, ctrl2, src(2), dst(2), dataLen, APDU...]
func ParseCEMI(data []byte) (CEMI, error) {
	if len(data) < 2 {
		return CEMI{}, errors.New("CEMI too short")
	}
	msgCode := data[0]
	addInfoLen := int(data[1])
	offset := 2 + addInfoLen
	if len(data) < offset+6 {
		return CEMI{}, errors.New("CEMI too short for control/address fields")
	}

	// ctrl1, ctrl2 at offset, offset+1 (skipped — we don't need them)
	srcAddr := binary.BigEndian.Uint16(data[offset+2 : offset+4])
	dstAddr := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	dataLen := int(data[offset+6])

	apduStart := offset + 7
	if len(data) < apduStart+2 {
		return CEMI{}, errors.New("CEMI too short for APDU")
	}

	apci := binary.BigEndian.Uint16(data[apduStart : apduStart+2])
	cmd := apci & apciMask

	c := CEMI{
		MsgCode: msgCode,
		SrcAddr: srcAddr,
		DstAddr: dstAddr,
		APCI:    cmd,
	}

	if cmd == APCIGroupValueRead {
		// No data payload.
		return c, nil
	}

	// Compact encoding: dataLen==1 means the data is in the low 6 bits of the second APCI byte.
	if dataLen == 1 {
		c.IsCompact = true
		c.Data = []byte{data[apduStart+1] & 0x3F}
	} else {
		c.IsCompact = false
		payloadStart := apduStart + 2
		payloadLen := dataLen - 1 // dataLen includes the APCI overhead byte
		if len(data) < payloadStart+payloadLen {
			return CEMI{}, errors.New("CEMI data truncated")
		}
		c.Data = make([]byte, payloadLen)
		copy(c.Data, data[payloadStart:payloadStart+payloadLen])
	}

	return c, nil
}

// BuildCEMIGroupValueResponse builds a CEMI L_Data.ind frame with a GroupValueResponse.
func BuildCEMIGroupValueResponse(srcAddr, dstGA uint16, data []byte, compact bool) []byte {
	var apdu []byte
	if compact {
		// Compact: data in low 6 bits of APCI second byte.
		apduHi := byte(APCIGroupValueResponse >> 8)
		apduLo := byte(APCIGroupValueResponse) | (data[0] & 0x3F)
		apdu = []byte{apduHi, apduLo}
	} else {
		apduHi := byte(APCIGroupValueResponse >> 8)
		apduLo := byte(APCIGroupValueResponse)
		apdu = append([]byte{apduHi, apduLo}, data...)
	}

	dataLen := len(apdu) - 1 // KNX dataLen = APDU length - 1
	if compact {
		dataLen = 1
	}

	// CEMI: msgCode, addInfoLen(0), ctrl1, ctrl2, src(2), dst(2), dataLen, APDU
	cemi := make([]byte, 0, 9+len(apdu))
	cemi = append(cemi, CEMIMsgCodeLDataInd, 0x00)
	cemi = append(cemi, 0xB0, 0xE0) // ctrl1=0xB0 (standard frame), ctrl2=0xE0 (group address, hop count 6)
	cemi = append(cemi, byte(srcAddr>>8), byte(srcAddr))
	cemi = append(cemi, byte(dstGA>>8), byte(dstGA))
	cemi = append(cemi, byte(dataLen))
	cemi = append(cemi, apdu...)
	return cemi
}

// BuildCEMIGroupValueWrite builds a CEMI L_Data.ind frame with a GroupValueWrite.
// Used for unsolicited state updates pushed to the client.
func BuildCEMIGroupValueWrite(srcAddr, dstGA uint16, data []byte, compact bool) []byte {
	var apdu []byte
	if compact {
		apduHi := byte(APCIGroupValueWrite >> 8)
		apduLo := byte(APCIGroupValueWrite) | (data[0] & 0x3F)
		apdu = []byte{apduHi, apduLo}
	} else {
		apduHi := byte(APCIGroupValueWrite >> 8)
		apduLo := byte(APCIGroupValueWrite)
		apdu = append([]byte{apduHi, apduLo}, data...)
	}

	dataLen := len(apdu) - 1
	if compact {
		dataLen = 1
	}

	cemi := make([]byte, 0, 9+len(apdu))
	cemi = append(cemi, CEMIMsgCodeLDataInd, 0x00)
	cemi = append(cemi, 0xB0, 0xE0)
	cemi = append(cemi, byte(srcAddr>>8), byte(srcAddr))
	cemi = append(cemi, byte(dstGA>>8), byte(dstGA))
	cemi = append(cemi, byte(dataLen))
	cemi = append(cemi, apdu...)
	return cemi
}

// BuildCEMILDataCon returns an L_Data.con frame confirming delivery of the
// given L_Data.req CEMI to the (virtual) KNX bus. Per KNX specification
// 03/06/03 §4.1.5, the server must send this confirmation so that the client
// knows it can proceed with the next telegram.
func BuildCEMILDataCon(rawReq []byte) []byte {
	con := make([]byte, len(rawReq))
	copy(con, rawReq)
	if len(con) > 0 {
		con[0] = CEMIMsgCodeLDataCon
	}
	return con
}

// Tunneling connection header (4 bytes): [0x04, channelID, seqCounter, status]

func ParseTunnelingHeader(data []byte) (channelID, seq uint8, err error) {
	if len(data) < 4 {
		return 0, 0, errors.New("tunneling header too short")
	}
	if data[0] != 0x04 {
		return 0, 0, errors.New("invalid tunneling header length")
	}
	return data[1], data[2], nil
}

func MarshalTunnelingACK(channelID, seq, status uint8) []byte {
	body := []byte{0x04, channelID, seq, status}
	hdr := MarshalHeader(ServiceTunnelingACK, headerSize+len(body))
	return append(hdr, body...)
}

func MarshalTunnelingRequest(channelID, seq uint8, cemi []byte) []byte {
	body := make([]byte, 0, 4+len(cemi))
	body = append(body, 0x04, channelID, seq, 0x00)
	body = append(body, cemi...)
	hdr := MarshalHeader(ServiceTunnelingRequest, headerSize+len(body))
	return append(hdr, body...)
}
