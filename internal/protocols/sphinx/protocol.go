package sphinx

import (
	"fmt"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"strconv"
	"strings"
	"time"
)

// SPHINX protocol constants
const (
	SPHINX_DELIMITER = "\r\n"

	SPHINX_WAC_NONE             = 0x00
	SPHINX_WAC_AUTH             = 0x01
	SPHINX_WAC_DELEGATION_START = 0x02
	SPHINX_WAC_DELEGATION_END   = 0x03
	SPHINX_WAC_SUBSCRIBE        = 0x04
	SPHINX_WAC_UNSUBSCRIBE      = 0x05

	SPHINX_APRT_NORMAL = "NORMAL"
	SPHINX_APRT_ESCORT = "ESCORT"

	SPHINX_PING_INTERVAL = 5
	SPHINX_PING_TIMEOUT  = 10
)

// SphinxConnection represents SPHINX connection state
type SphinxConnection struct {
	WaitForAnswer int       `json:"wait_for_answer"`
	PingInterval  int       `json:"ping_interval"`
	PingTimeout   int       `json:"ping_timeout"`
	LastPingTime  time.Time `json:"last_ping_time"`
	PingSent      bool      `json:"ping_sent"`
	ConEvtTime    time.Time `json:"con_evt_time"`
}

// NewSphinxConnection creates new SPHINX connection
func NewSphinxConnection() *SphinxConnection {
	return &SphinxConnection{
		WaitForAnswer: SPHINX_WAC_NONE,
		PingInterval:  SPHINX_PING_INTERVAL,
		PingTimeout:   SPHINX_PING_TIMEOUT,
		LastPingTime:  time.Now(),
		PingSent:      false,
		ConEvtTime:    time.Now(),
	}
}

// DecodePacket decodes SPHINX packet
func DecodePacket(data []byte) (*types.Packet, error) {
	str := strings.TrimSpace(string(data))
	if str == "" {
		return nil, fmt.Errorf("empty packet")
	}

	// Split by spaces
	parts := strings.Fields(str)
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid packet format")
	}

	cmd := strings.ToUpper(parts[0])
	params := parts[1:]

	// Validate command
	if !isValidCommand(cmd) {
		return nil, fmt.Errorf("invalid command: %s", cmd)
	}

	return &types.Packet{
		Cmd:     0, // SPHINX uses text commands
		Payload: str,
		Data: map[string]interface{}{
			"command": cmd,
			"params":  params,
			"raw":     str,
		},
	}, nil
}

// EncodePacket encodes SPHINX packet
func EncodePacket(cmd string, params ...string) []byte {
	parts := append([]string{cmd}, params...)
	packet := strings.Join(parts, " ") + SPHINX_DELIMITER
	return []byte(packet)
}

// isValidCommand validates SPHINX command
func isValidCommand(cmd string) bool {
	validCommands := []string{
		"OK", "ERROR", "LOGIN", "LOGOUT", "SUBSCRIBE", "UNSUBSCRIBE",
		"DELEGATION_START", "DELEGATION_STOP", "DELEGATION_REQUEST",
		"DELEGATION_REPLY", "GETAPLIST", "GETZONEINFO", "PING", "PONG",
	}

	for _, validCmd := range validCommands {
		if cmd == validCmd {
			return true
		}
	}
	return false
}

// CreateLoginPacket creates login packet
func CreateLoginPacket(version, username, password string) []byte {
	return EncodePacket("LOGIN", version, fmt.Sprintf("\"%s\"", username), fmt.Sprintf("\"%s\"", password))
}

// CreateDelegationReply creates delegation reply
func CreateDelegationReply(ticket, accessType string, result int, flags ...string) []byte {
	params := []string{ticket, accessType, strconv.Itoa(result)}
	params = append(params, flags...)
	return EncodePacket("DELEGATION_REPLY", params...)
}

// CreateDelegationStartPacket creates DELEGATION_START packet for ping
func CreateDelegationStartPacket() []byte {
	return EncodePacket("DELEGATION_START")
}

// ParseDelegationRequest parses delegation request
func ParseDelegationRequest(params []string) (map[string]interface{}, error) {
	if len(params) < 4 {
		return nil, fmt.Errorf("insufficient parameters for delegation request")
	}

	result := map[string]interface{}{
		"ticket":              params[0],
		"access_request_type": strings.ToUpper(params[1]),
	}

	// Parse key data
	if len(params) > 2 {
		keyData := params[2]
		if strings.HasPrefix(keyData, "W26") && len(params) > 4 {
			// W26 format: facility_code + card_number
			facility, err1 := strconv.Atoi(params[3])
			card, err2 := strconv.Atoi(params[4])
			if err1 == nil && err2 == nil {
				uid := utils.GenID() // Generate UID from facility and card
				result["key_type"] = "W26"
				result["facility_code"] = facility
				result["card_number"] = card
				result["uid"] = uid
			}
		} else if strings.HasPrefix(keyData, "W34") && len(params) > 3 {
			// W34 format: hex data
			hexData := params[3]
			if uid, err := utils.CryptHexToStr(hexData); err == nil {
				result["key_type"] = "W34"
				result["uid_raw"] = uid
				result["uid_hex"] = hexData
			}
		} else if keyData == "ID" && len(params) > 3 {
			// ID format
			result["key_type"] = "ID"
			result["person_id"] = params[3]
		}
	}

	// Parse additional parameters
	if len(params) > 5 {
		result["direction"] = params[5]
	}
	if len(params) > 6 {
		result["access_point_id"] = params[6]
	}
	if len(params) > 7 {
		result["extra_data"] = params[7]
	}

	return result, nil
}

// ValidateTicket validates ticket format
func ValidateTicket(ticket string) bool {
	return len(ticket) > 0 && len(ticket) <= 32
}

// FormatUID formats UID for SPHINX protocol
func FormatUID(uid []byte) string {
	if len(uid) == 0 {
		return ""
	}

	// Convert to hex string without spaces
	result := ""
	for _, b := range uid {
		result += fmt.Sprintf("%02X", b)
	}

	return strings.ToUpper(result)
}

// ProcessAuth processes authentication response
func ProcessAuth(response string, conn *SphinxConnection) error {
	parts := strings.Fields(strings.TrimSpace(response))
	if len(parts) == 0 {
		return fmt.Errorf("empty response")
	}

	cmd := strings.ToUpper(parts[0])

	switch conn.WaitForAnswer {
	case SPHINX_WAC_AUTH:
		if cmd == "OK" {
			conn.WaitForAnswer = SPHINX_WAC_NONE
			return nil
		} else {
			return fmt.Errorf("authentication failed: %s", response)
		}

	case SPHINX_WAC_DELEGATION_START:
		if cmd == "OK" {
			conn.WaitForAnswer = SPHINX_WAC_NONE
			return nil
		} else {
			return fmt.Errorf("delegation start failed: %s", response)
		}

	case SPHINX_WAC_SUBSCRIBE:
		if cmd == "OK" {
			conn.WaitForAnswer = SPHINX_WAC_NONE
			return nil
		} else {
			return fmt.Errorf("subscription failed: %s", response)
		}
	}

	return nil
}

// ShouldSendPing checks if ping should be sent
func (sc *SphinxConnection) ShouldSendPing() bool {
	if sc.PingInterval <= 0 {
		return false
	}

	return time.Since(sc.LastPingTime).Seconds() >= float64(sc.PingInterval)
}

// MarkPingSent marks ping as sent
func (sc *SphinxConnection) MarkPingSent() {
	sc.LastPingTime = time.Now()
	sc.PingSent = true
}

// CheckPingTimeout checks if ping timeout occurred
func (sc *SphinxConnection) CheckPingTimeout() bool {
	if !sc.PingSent || sc.PingTimeout <= 0 {
		return false
	}

	return time.Since(sc.LastPingTime).Seconds() >= float64(sc.PingTimeout)
}

// ResetPing resets ping state
func (sc *SphinxConnection) ResetPing() {
	sc.PingSent = false
	sc.LastPingTime = time.Now()
}

// GetPingPacket returns ping packet
func GetPingPacket() []byte {
	return EncodePacket("DELEGATION_START")
}

// GetPongPacket returns pong packet
func GetPongPacket() []byte {
	return EncodePacket("OK")
}
