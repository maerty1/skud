package gat

import (
	"encoding/binary"
	"fmt"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
)

// GAT protocol constants
const (
	GAT_MARKER = 0x2A // '*'

	GAT_CMD_REQ_MASTER     = 0xE5
	GAT_CMD_CARD_IDENT     = 0x80
	GAT_CMD_ACTION_STARTED = 0xA1
	GAT_CMD_CANCEL         = 0xC0
	GAT_CMD_HOST_CONTROL   = 0xCA

	GAT_TTYPE_ACCESS = 0x01
	GAT_TTYPE_TIME   = 0x02

	GAT_ARES_USED = 0x01
)

// Terminal types
var GATTerminalTypes = map[uint8]string{
	0x00: "INFO",
	0x01: "ACCESS",
	0x02: "TIME",
	0x03: "RETURN",
	0x04: "CASH",
}

// Reader types
var GATReaderTypes = map[uint8]string{
	0x00: "Unknown",
	0x01: "Card Reader",
	0x02: "Biometric",
	0x03: "PIN",
}

// DecodePacket decodes GAT packet
// GAT packet format (as in PHP):
// - len (1 byte): total packet length including len, adr, cmd, te_status (if present), data, lrc
// - adr (1 byte): terminal address
// - cmd (1 byte): command
// - te_status (1 byte, optional): terminal status (if cmd & 0x10)
// - data (variable): payload data
// - lrc (1 byte): Longitudinal Redundancy Check (XOR of all previous bytes)
func DecodePacket(data []byte) (*types.Packet, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("packet too short: need at least 4 bytes, got %d", len(data))
	}

	offset := 0
	soffset := offset

	// Read length
	pktLen := int(data[offset])
	offset++

	if pktLen < 3 {
		return nil, fmt.Errorf("invalid packet length: %d (minimum 3)", pktLen)
	}

	if len(data) < pktLen+1 {
		return nil, fmt.Errorf("packet incomplete: need %d bytes, got %d", pktLen+1, len(data))
	}

	// Read address
	adr := data[offset]
	offset++

	// Read command
	cmd := data[offset]
	offset++

	// Read terminal status if command has 0x10 bit set
	var teStatus uint8
	if cmd&0x10 != 0 {
		if offset >= len(data) {
			return nil, fmt.Errorf("packet too short for te_status")
		}
		teStatus = data[offset]
		offset++
	}

	// Read data (remaining bytes before LRC)
	dataLen := pktLen - (offset - soffset) - 1 // -1 for LRC
	var payload []byte
	if dataLen > 0 {
		if offset+dataLen > len(data) {
			return nil, fmt.Errorf("packet too short for data: need %d bytes, got %d", offset+dataLen, len(data))
		}
		payload = data[offset : offset+dataLen]
		offset += dataLen
	}

	// Read and verify LRC
	if offset >= len(data) {
		return nil, fmt.Errorf("packet too short for LRC")
	}
	receivedLRC := data[offset]

	// Calculate LRC (XOR of all bytes from len to data)
	calculatedLRC := calculateLRC(data[soffset:offset])
	if receivedLRC != calculatedLRC {
		// Try Big Endian CRC interpretation (for compatibility)
		crcBE := binary.BigEndian.Uint16(data[offset-2:offset])
		crcLE := binary.LittleEndian.Uint16(data[offset-2:offset])
		return nil, fmt.Errorf("LRC mismatch: expected %02X, got %02X (CRC16 BE: %04X, LE: %04X)", calculatedLRC, receivedLRC, crcBE, crcLE)
	}

	// Parse command data
	packetData := make(map[string]interface{})
	packetData["address"] = adr
	packetData["te_status"] = teStatus

	switch cmd {
	case GAT_CMD_REQ_MASTER:
		packetData["cmd_name"] = "GAT_CMD_REQ_MASTER"
		if len(payload) > 0 {
			packetData["terminal_type"] = payload[0]
			if terminalName, ok := GATTerminalTypes[payload[0]]; ok {
				packetData["terminal_name"] = terminalName
			}
		}

	case GAT_CMD_CARD_IDENT:
		packetData["cmd_name"] = "GAT_CMD_CARD_IDENT"
		if len(payload) >= 12 {
			offset := 0
			packetData["terminal_type"] = payload[offset]
			offset++
			packetData["reader_type"] = payload[offset]
			offset++
			packetData["data_valid"] = payload[offset]
			offset++

			// UID (10 bytes, padded with zeros)
			uid := payload[offset : offset+10]
			offset += 10

			// Remove trailing zeros
			for i := len(uid) - 1; i >= 0; i-- {
				if uid[i] != 0 {
					uid = uid[:i+1]
					break
				}
			}
			packetData["uid_raw"] = uid
			packetData["uid_hex"] = utils.NDs(uid, "", "")

			// Additional data based on terminal type
			if terminalType, ok := packetData["terminal_type"].(uint8); ok {
				if terminalType == GAT_TTYPE_TIME && len(payload) > offset+6 {
					packetData["time"] = binary.LittleEndian.Uint16(payload[offset : offset+2])
					offset += 2
					packetData["price"] = binary.LittleEndian.Uint32(payload[offset : offset+4])
					offset += 4
				}
			}
		}

	case GAT_CMD_ACTION_STARTED:
		packetData["cmd_name"] = "GAT_CMD_ACTION_STARTED"
		if len(payload) >= 12 {
			offset := 0
			packetData["terminal_type"] = payload[offset]
			offset++
			packetData["reader_type"] = payload[offset]
			offset++
			packetData["data_valid"] = payload[offset]
			offset++

			// UID (10 bytes)
			uid := payload[offset : offset+10]
			offset += 10

			// Remove trailing zeros
			for i := len(uid) - 1; i >= 0; i-- {
				if uid[i] != 0 {
					uid = uid[:i+1]
					break
				}
			}
			packetData["uid_raw"] = uid
			packetData["uid_hex"] = utils.NDs(uid, "", "")

			// Terminal-specific data
			if terminalType, ok := packetData["terminal_type"].(uint8); ok {
				if terminalType == GAT_TTYPE_ACCESS && len(payload) > offset {
					packetData["access_result"] = payload[offset]
				} else if terminalType == GAT_TTYPE_TIME && len(payload) > offset+10 {
					packetData["vendor"] = binary.LittleEndian.Uint32(payload[offset : offset+4])
					offset += 4
					packetData["price"] = binary.LittleEndian.Uint32(payload[offset : offset+4])
					offset += 4
					packetData["time"] = binary.LittleEndian.Uint16(payload[offset : offset+2])
				}
			}
		}

	case GAT_CMD_HOST_CONTROL:
		packetData["cmd_name"] = "GAT_CMD_HOST_CONTROL"
		if len(payload) >= 2 {
			packetData["control_data"] = binary.LittleEndian.Uint16(payload[0:2])
		}
	}

	return &types.Packet{
		Cmd:     cmd,
		Payload: utils.NDs(payload, " ", ""),
		Data:    packetData,
	}, nil
}

// EncodePacket encodes GAT packet
// GAT packet format: len, adr, cmd, [te_status], data, lrc
func EncodePacket(cmd uint8, address uint8, teStatus uint8, data []byte) []byte {
	// Build packet without LRC
	packet := []byte{}
	
	// Add address
	packet = append(packet, address)
	
	// Add command
	packet = append(packet, cmd)
	
	// Add terminal status if command has 0x10 bit set
	if cmd&0x10 != 0 {
		packet = append(packet, teStatus)
	}
	
	// Add data
	if len(data) > 0 {
		packet = append(packet, data...)
	}
	
	// Calculate length (including len byte itself, but excluding LRC)
	pktLen := len(packet) + 1 // +1 for len byte itself
	
	// Prepend length
	packet = append([]byte{uint8(pktLen)}, packet...)
	
	// Calculate and append LRC
	lrc := calculateLRC(packet)
	packet = append(packet, lrc)
	
	return packet
}

// calculateLRC calculates GAT LRC (Longitudinal Redundancy Check)
// LRC is XOR of all bytes in the packet (from len to data, excluding LRC itself)
func calculateLRC(data []byte) uint8 {
	var lrc uint8
	for _, b := range data {
		lrc ^= b
	}
	return lrc
}

// GetResponseCommand returns response command for given command
func GetResponseCommand(cmd uint8) uint8 {
	switch cmd {
	case GAT_CMD_REQ_MASTER:
		return GAT_CMD_REQ_MASTER
	case GAT_CMD_CARD_IDENT:
		return GAT_CMD_CARD_IDENT
	case GAT_CMD_ACTION_STARTED:
		return GAT_CMD_ACTION_STARTED
	case GAT_CMD_HOST_CONTROL:
		return GAT_CMD_HOST_CONTROL + 1 // Response command
	case GAT_CMD_CANCEL:
		return GAT_CMD_CANCEL
	default:
		return cmd
	}
}

// CreateResponse creates response packet
func CreateResponse(cmd uint8, address uint8, teStatus uint8, data []byte) []byte {
	respCmd := GetResponseCommand(cmd)
	return EncodePacket(respCmd, address, teStatus, data)
}

// ValidateUID validates UID format
func ValidateUID(uid []byte) bool {
	return len(uid) > 0 && len(uid) <= 10
}

// FormatUID formats UID for logging
func FormatUID(uid []byte) string {
	if len(uid) == 0 {
		return ""
	}

	// Convert to hex string
	hexStr := ""
	for _, b := range uid {
		hexStr += fmt.Sprintf("%02X", b)
	}

	return hexStr
}

// CreateReqMasterPacket creates REQ_MASTER packet for ping
// address: terminal address (usually 0x00 for broadcast or specific terminal)
// terminalType: terminal type (0x01 for ACCESS, 0x02 for TIME, etc.)
func CreateReqMasterPacket(address uint8, terminalType uint8) []byte {
	// REQ_MASTER payload contains terminal type
	payload := []byte{terminalType}
	return EncodePacket(GAT_CMD_REQ_MASTER, address, 0, payload)
}
