package pocket

import (
	"encoding/binary"
	"fmt"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"strings"
)

// POCKET protocol constants
const (
	POCKET_MARKER = 0x2A // '*'

	// Configuration tags
	POCKET_CFG_TAG_MAC               = 0x01
	POCKET_CFG_TAG_IP                = 0x02
	POCKET_CFG_TAG_MASK              = 0x03
	POCKET_CFG_TAG_GATEWAY           = 0x04
	POCKET_CFG_TAG_UDP_PORT          = 0x05
	POCKET_CFG_TAG_TCP_PORT          = 0x06
	POCKET_CFG_TAG_DEBUG_PORT        = 0x07
	POCKET_CFG_TAG_PING_CONTROL      = 0x09
	POCKET_CFG_TAG_NET               = 0x08
	POCKET_CFG_TAG_BEHAVIOR          = 0x10
	POCKET_CFG_TAG_LANG              = 0x11
	POCKET_CFG_TAG_PROGRESS_MODE     = 0x12
	POCKET_CFG_TAG_WAIT_TMO          = 0x13
	POCKET_CFG_TAG_BLOCK_CAB         = 0x14
	POCKET_CFG_TAG_BLOCK_CELL        = 0x15
	POCKET_CFG_TAG_NO_FINGER         = 0x16
	POCKET_CFG_TAG_NO_FINGER_HW_TEST = 0x17
	POCKET_CFG_TAG_BUZZER_DUTY       = 0x18
	POCKET_CFG_TAG_LED_BRIGHTNESS    = 0x19
	POCKET_CFG_TAG_SECTOR_KEYS       = 0x20
	POCKET_CFG_TAG_LOCKERS_LIST      = 0x21

	// Interactive tags
	POCKET_INTERACTIVE_TAG_DELAY      = 0x00
	POCKET_INTERACTIVE_TAG_SOUND      = 0x01
	POCKET_INTERACTIVE_TAG_LIGHT      = 0x02
	POCKET_INTERACTIVE_TAG_TEXT       = 0x03
	POCKET_INTERACTIVE_TAG_WAITING    = 0x04
	POCKET_INTERACTIVE_TAG_HOURGLASS  = 0x05
	POCKET_INTERACTIVE_TAG_SYS_IDLE   = 0xE0
	POCKET_INTERACTIVE_TAG_SYS_LOCKS  = 0xE1
	POCKET_INTERACTIVE_TAG_SYS_DCOUNT = 0xE2

	// Sound types
	POCKET_INTERACTIVE_SOUND_BEEP  = 0x00
	POCKET_INTERACTIVE_SOUND_QUACK = 0x01

	// Relay flags
	POCKET_RELAY_FLAG_DOWNCOUNT     = 0x01
	POCKET_RELAY_FLAG_ZSECOND       = 0x02
	POCKET_RELAY_FLAG_TAKE_CARD     = 0x04
	POCKET_RELAY_FLAG_GATE_TRANSFER = 0x08

	// Signal types
	POCKET_SIGNAL_LOCKED     = 0x01
	POCKET_SIGNAL_UNLOCKED   = 0x02
	POCKET_SIGNAL_NFC_LOCK   = 0x03
	POCKET_SIGNAL_NFC_UNLOCK = 0x04

	POCKET_RELAY_UID_MAX_LEN = 32
)

// DecodePacket decodes POCKET packet
func DecodePacket(data []byte) (*types.Packet, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("packet too short")
	}

	// Check marker
	if data[0] != POCKET_MARKER {
		return nil, fmt.Errorf("invalid marker: expected 0x2A, got 0x%02X", data[0])
	}

	// Parse header
	flags := data[1]
	code := data[2]
	payloadLen := int(binary.LittleEndian.Uint16(data[3:5]))
	crc := binary.LittleEndian.Uint16(data[5:7])

	// Validate payload length
	if len(data) < 7+payloadLen {
		return nil, fmt.Errorf("packet payload incomplete: expected %d bytes, got %d", payloadLen, len(data)-7)
	}

	// Extract payload
	payload := data[7 : 7+payloadLen]

	// Calculate and verify CRC
	calcCRC := utils.CRC8(data[:7+payloadLen])
	if calcCRC != uint8(crc&0xFF) {
		return nil, fmt.Errorf("CRC mismatch: calculated 0x%02X, received 0x%02X", calcCRC, uint8(crc&0xFF))
	}

	// Parse payload based on command
	packetData := make(map[string]interface{})

	switch code {
	case 0x02: // ReadTag
		if len(payload) >= 4 {
			readerType := payload[0]
			readerFlags := payload[1]
			uidLen := int(payload[2])
			if len(payload) >= 4+uidLen {
				uid := payload[3 : 3+uidLen]
				packetData["reader_type"] = readerType
				packetData["reader_flags"] = readerFlags
				packetData["uid"] = utils.BytesToHex(uid)
				packetData["auth"] = true
			}
		}

	case 0x03: // ReadTagExtended
		offset := 0
		if len(payload) < 5 {
			break
		}
		
		// Parse UID
		uidLen := int(payload[offset])
		offset++
		if offset+uidLen > len(payload) {
			break
		}
		
		uidRaw := payload[offset : offset+uidLen]
		offset += uidLen
		packetData["uid_raw"] = utils.BytesToHex(uidRaw)
		packetData["uid"] = utils.BytesToHex(uidRaw)
		
		// Parse Wiegand if uid_len > 2
		if uidLen > 2 {
			wiegand := fmt.Sprintf(",%02X", binary.LittleEndian.Uint16(uidRaw[:2]))
			if len(uidRaw) >= 3 {
				wiegand = fmt.Sprintf("%02X%s", uidRaw[2], wiegand)
			}
			packetData["wiegand"] = wiegand
		}
		
		// Parse finger_result (2 bytes, optional)
		if offset+2 <= len(payload) {
			fingerResultUint := binary.LittleEndian.Uint16(payload[offset : offset+2])
			var fingerResult int16
			if fingerResultUint&0x8000 != 0 {
				// Negative value: subtract 65536
				fingerResult = int16(int32(fingerResultUint) - 65536)
			} else {
				fingerResult = int16(fingerResultUint)
			}
			packetData["finger_result"] = fingerResult
			offset += 2
		}
		
		// Parse lockers_data (4 bytes per locker)
		lockersData := []types.LockerInfo{}
		for offset+4 <= len(payload) {
			// err byte: auth_err = (err >> 4) & 0x0F, read_err = err & 0x0F
			err := payload[offset]
			offset++
			authErr := (err >> 4) & 0x0F
			readErr := err & 0x0F
			
			// bno byte: is_passtech = (bno & 0x80) != 0, block_no = bno & 0x7F or bno
			bno := payload[offset]
			offset++
			isPasstech := (bno & 0x80) != 0
			blockNo := bno & 0x7F
			if !isPasstech {
				blockNo = bno
			}
			
			litera := "-"
			if isPasstech && blockNo > 0 && blockNo < 27 {
				litera = string(rune('A' + blockNo - 1))
			}
			
			// cab (2 bytes): locked = (cab & 0x8000) != 0, cab_no = cab & 0x7FFF
			cab := binary.LittleEndian.Uint16(payload[offset : offset+2])
			offset += 2
			locked := (cab & 0x8000) != 0
			cabNo := cab & 0x7FFF
			
			lockerInfo := types.LockerInfo{
				AuthErr:   authErr,
				ReadErr:   readErr,
				IsPasstech: isPasstech,
				BlockNo:   blockNo,
				Litera:    litera,
				Locked:    locked,
				CabNo:     cabNo,
			}
			lockersData = append(lockersData, lockerInfo)
		}
		
		if len(lockersData) > 0 {
			packetData["lockers_data"] = lockersData
			// Set auth based on first locker
			lauth := lockersData[0]
			packetData["auth"] = (lauth.AuthErr == 0 && lauth.ReadErr == 0)
		} else {
			packetData["auth"] = false
		}
		
		// Parse code flags (if code != 0)
		if code != 0 {
			if (code & 0x01) != 0 {
				packetData["last_sector_auth"] = true
			}
			if (code & 0x02) != 0 {
				packetData["passtech_auth"] = true
			}
			if (code & 0x04) != 0 {
				packetData["temp_card"] = true
			}
			if (code & 0x08) != 0 {
				packetData["fast_react"] = true
			}
		}

	case 0x15: // RelayControlEx
		if len(payload) >= 5 {
			onTime := binary.LittleEndian.Uint32(payload[0:4])
			flags := payload[4]
			uidLen := int(payload[5])
			uid := ""
			caption := ""

			offset := 6
			if offset+uidLen <= len(payload) {
				uid = string(payload[offset : offset+uidLen])
				offset += uidLen
			}

			if offset < len(payload) {
				caption = string(payload[offset:])
			}

			packetData["on_time"] = onTime
			packetData["flags"] = flags
			packetData["uid"] = uid
			packetData["caption"] = caption
		}

	case 0x16: // InputChanged
		if len(payload) >= 2 {
			inputState := binary.LittleEndian.Uint16(payload[0:2])
			packetData["input_state"] = inputState
			packetData["passed"] = (inputState & 0x01) != 0
		}
	}

	return &types.Packet{
		Cmd:     code,
		Code:    &flags,
		Flags:   &code,
		Payload: string(payload),
		Data:    packetData,
	}, nil
}

// EncodePacket encodes packet to POCKET format
func EncodePacket(cmd uint8, flags uint8, payload string) []byte {
	payloadBytes := []byte(payload)
	payloadLen := len(payloadBytes)

	// Calculate CRC for header + payload
	header := []byte{
		POCKET_MARKER,
		flags,
		cmd,
		uint8(payloadLen & 0xFF),
		uint8((payloadLen >> 8) & 0xFF),
		0, // CRC placeholder
		0,
	}

	crcData := append(header[:5], payloadBytes...)
	crc := utils.CRC8(crcData)

	header[5] = uint8(crc & 0xFF)
	header[6] = uint8((crc >> 8) & 0xFF)

	return append(header, payloadBytes...)
}

// Interactive message functions

// InteractiveDelay creates delay TLV
func InteractiveDelay(delay int) []byte {
	delay = delay & 0xFFFF
	return utils.EncodeTLV(POCKET_INTERACTIVE_TAG_DELAY, utils.EncodeUint16(uint16(delay)))
}

// InteractiveWaiting creates waiting TLV
func InteractiveWaiting(delay int, displayHourglass bool, waitTillRemoved bool) []byte {
	val := utils.EncodeUint16(uint16(delay & 0xFFFF))

	dhg := uint8(0)
	if displayHourglass {
		dhg = 1
	}
	val = append(val, dhg)

	if dhg > 0 || waitTillRemoved {
		wtr := uint8(0)
		if waitTillRemoved {
			wtr = 1
		}
		val = append(val, wtr)
	}

	return utils.EncodeTLV(POCKET_INTERACTIVE_TAG_WAITING, val)
}

// InteractiveText creates text TLV
func InteractiveText(text string) []byte {
	return utils.EncodeTLV(POCKET_INTERACTIVE_TAG_TEXT, []byte(text))
}

// InteractiveSound creates sound TLV
func InteractiveSound(soundType uint8, freq uint16, length uint16, volume uint8, endDelay uint16) []byte {
	if endDelay > length {
		endDelay = 0
	}
	if endDelay > 0 {
		length = length - endDelay
	}

	val := []byte{
		soundType & 0xFF,
		uint8(freq & 0xFF),
		uint8((freq >> 8) & 0xFF),
		uint8(length & 0xFF),
		uint8((length >> 8) & 0xFF),
		volume & 0xFF,
	}

	result := utils.EncodeTLV(POCKET_INTERACTIVE_TAG_SOUND, val)

	if endDelay > 0 {
		result = append(result, InteractiveDelay(int(endDelay))...)
	}

	return result
}

// Interactive creates full interactive message
func Interactive(text string, displayTime int, sound int, tillRemoved bool) []byte {
	result := InteractiveText(text)

	volume := uint8(0xFF)

	switch sound {
	case 1: // Beep
		result = append(result, InteractiveSound(POCKET_INTERACTIVE_SOUND_BEEP, 4000, 150, volume, 50)...)
	case 2: // Quack
		result = append(result, InteractiveSound(POCKET_INTERACTIVE_SOUND_QUACK, 4000, 150, volume, 50)...)
	case 3: // BeepBeep
		result = append(result, InteractiveSound(POCKET_INTERACTIVE_SOUND_BEEP, 4000, 100, volume, 50)...)
		result = append(result, InteractiveSound(POCKET_INTERACTIVE_SOUND_BEEP, 4000, 100, volume, 50)...)
	case 4: // QuackQuack
		result = append(result, InteractiveSound(POCKET_INTERACTIVE_SOUND_QUACK, 4000, 100, volume, 50)...)
		result = append(result, InteractiveSound(POCKET_INTERACTIVE_SOUND_QUACK, 4000, 150, volume, 50)...)
	}

	if displayTime > 0 || tillRemoved {
		result = append(result, InteractiveWaiting(displayTime, false, tillRemoved)...)
	}

	return result
}

// CreateInteractivePacket creates interactive message packet (wrapper for Interactive)
func CreateInteractivePacket(text string, displayTime int, sound int, tillRemoved bool) []byte {
	return Interactive(text, displayTime, sound, tillRemoved)
}

// CreatePacket creates POCKET packet with command, flags and payload
func CreatePacket(cmd uint8, flags uint8, payload []byte) []byte {
	return EncodePacket(cmd, flags, string(payload))
}

// RelayOnEx creates relay control packet
func RelayOnEx(onTime uint32, flags uint8, caption string, uid string) []byte {
	var val []byte

	if onTime > 0x0FFFFFFF {
		val = []byte{0xFF, 0xFF, 0xFF, 0xFF}
	} else {
		val = utils.EncodeUint32(onTime)
	}

	val = append(val, flags)

	uidLen := len(uid)
	if uidLen > POCKET_RELAY_UID_MAX_LEN {
		uidLen = POCKET_RELAY_UID_MAX_LEN
	}
	val = append(val, uint8(uidLen))

	if uidLen > 0 {
		val = append(val, []byte(uid)[:uidLen]...)
	}

	if caption != "" {
		maxLen := 96
		if (flags & POCKET_RELAY_FLAG_DOWNCOUNT) != 0 {
			maxLen = 16
		}
		if len(caption) > maxLen {
			caption = caption[:maxLen]
		}
		val = append(val, []byte(caption)...)
	}

	return val
}

// ParseUID extracts UID from various formats
func ParseUID(data interface{}) string {
	if data == nil {
		return ""
	}

	switch v := data.(type) {
	case string:
		// Remove spaces and convert to uppercase
		uid := strings.ReplaceAll(v, " ", "")
		uid = strings.ToUpper(uid)

		// Validate hex format
		if len(uid) >= 4 && len(uid) <= 16 {
			return uid
		}
	case []byte:
		return utils.BytesToHex(v)
	}

	return ""
}

// ValidateUID validates UID format
func ValidateUID(uid string) bool {
	if len(uid) == 0 {
		return false
	}

	// Check if it's hex
	for _, r := range uid {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}

	return len(uid) >= 4 && len(uid) <= 16
}

// GetReaderTypeString converts reader type to string
func GetReaderTypeString(readerType uint8) string {
	switch readerType {
	case 0x00:
		return "Unknown"
	case 0x01:
		return "Card Reader"
	case 0x02:
		return "Biometric"
	case 0x03:
		return "PIN"
	default:
		return fmt.Sprintf("Type_%d", readerType)
	}
}

// POCKET command codes
const (
	POCKET_CMD_ENQUIRE = 0x06
	POCKET_RESP_ENQUIRE = 0x86 // 0x06 | 0x80
)

// CreateEnquirePacket creates Enquire packet for ping
func CreateEnquirePacket() []byte {
	// Enquire packet has empty payload
	return EncodePacket(POCKET_CMD_ENQUIRE, 0x00, "")
}

// ProcessPacket processes incoming POCKET packet and returns response
func ProcessPacket(packet *types.Packet, config *types.Config) (*types.Packet, error) {
	switch packet.Cmd {
	case 0x02: // ReadTag response
		// This is a response to our read request, process the data
		return nil, nil // No response needed

	case 0x15: // RelayControlEx response
		// Relay operation completed
		return nil, nil // No response needed

	case 0x16: // InputChanged
		// Input state changed (person passed)
		return nil, nil // No response needed

	case POCKET_RESP_ENQUIRE: // Enquire response (pong)
		// This is a response to our Enquire (ping)
		// Parse ping interval and timeout if present
		if len(packet.Payload) >= 4 {
			// Payload contains: ping_interval (2 bytes) + ping_timeout (2 bytes)
			// We don't need to parse it, just acknowledge
		}
		return nil, nil // No response needed

	default:
		// Unknown command
		return nil, fmt.Errorf("unknown POCKET command: 0x%02X", packet.Cmd)
	}
}
