package jsp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// JSP protocol constants
const (
	JSP_SOF = 0x03 // Start of Frame
	JSP_EOF = 0x02 // End of Frame
	JSP_LENGTH_SIZE = 4 // Length field size in hex characters
)

// JSPConnection represents JSP connection state
type JSPConnection struct {
	Buffer          []byte
	PingInterval    int
	PingTimeout     int
	PingTimer       int
	PingSinceLast   int
	PingSent        bool
	LastSentTime    float64
	ConEvtTime      float64
	Requests        map[string]*JSPRequest
}

// JSPRequest represents JSP request
type JSPRequest struct {
	ID      string
	RKey    string
	Cmd     string
	Time    float64
	Params  map[string]interface{}
	CParams map[string]interface{}
	RParams map[string]interface{}
}

// NewJSPConnection creates new JSP connection
func NewJSPConnection() *JSPConnection {
	return &JSPConnection{
		Buffer:   make([]byte, 0),
		Requests: make(map[string]*JSPRequest),
	}
}

// EncodePacket encodes packet to JSP format
func EncodePacket(data map[string]interface{}) ([]byte, error) {
	// Convert to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}

	// Build packet: SOF + LENGTH_HEX + JSON + EOF
	lengthHex := fmt.Sprintf("%04X", len(jsonData))
	
	packet := make([]byte, 0, 1+4+len(jsonData)+1)
	packet = append(packet, JSP_SOF)
	packet = append(packet, []byte(lengthHex)...)
	packet = append(packet, jsonData...)
	packet = append(packet, JSP_EOF)

	return packet, nil
}

// DecodeHex decodes hex string to integer
func DecodeHex(hstr string) (int, error) {
	hstr = strings.ToUpper(strings.TrimSpace(hstr))
	length := len(hstr)
	
	if length%2 != 0 {
		return 0, fmt.Errorf("invalid hex length: %d", length)
	}
	if length < 2 || length > 6 {
		return 0, fmt.Errorf("hex length out of range: %d", length)
	}
	
	// Check if all characters are hex
	for _, r := range hstr {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			return 0, fmt.Errorf("invalid hex character: %c", r)
		}
	}
	
	val, err := strconv.ParseInt(hstr, 16, 32)
	if err != nil {
		return 0, err
	}
	
	return int(val), nil
}

// TryReadPacket tries to read complete packet from buffer
func TryReadPacket(conn *JSPConnection) (interface{}, error) {
	buf := conn.Buffer
	blen := len(buf)
	
	if blen == 0 {
		return false, nil // No data
	}
	
	// Find SOF
	pos := -1
	for i, b := range buf {
		if b == JSP_SOF {
			pos = i
			break
		}
	}
	
	if pos == -1 {
		// No SOF found, clear buffer
		conn.Buffer = nil
		return false, nil
	}
	
	if pos > 0 {
		// Remove data before SOF
		conn.Buffer = conn.Buffer[pos:]
		buf = conn.Buffer
		blen = len(buf)
	}
	
	// Check if we have enough data for header
	sofLen := 1
	eofLen := 1
	hlLen := JSP_LENGTH_SIZE
	hdLen := sofLen + hlLen
	
	if blen < hdLen {
		return hdLen - blen, nil // Need more data
	}
	
	// Decode length
	hexLen := string(buf[sofLen : sofLen+hlLen])
	pkLen, err := DecodeHex(hexLen)
	if err != nil {
		// Invalid hex, skip one byte
		conn.Buffer = conn.Buffer[1:]
		return false, nil
	}
	
	// Check if we have complete packet
	fullPkLen := sofLen + hlLen + pkLen + eofLen
	if blen < fullPkLen {
		return fullPkLen - blen, nil // Need more data
	}
	
	// Check EOF
	toffset := sofLen + hlLen + pkLen
	if buf[toffset] != JSP_EOF {
		// Invalid EOF, skip one byte
		conn.Buffer = conn.Buffer[1:]
		return false, nil
	}
	
	// Extract JSON data
	jsonData := buf[sofLen+hlLen : toffset]
	
	// Remove packet from buffer
	conn.Buffer = conn.Buffer[fullPkLen:]
	
	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	
	// Convert keys to lowercase (JSP protocol requirement)
	data = arrLower(data)
	
	return data, nil
}

// arrLower converts all map keys to lowercase recursively
func arrLower(data interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	if m, ok := data.(map[string]interface{}); ok {
		for k, v := range m {
			key := strings.ToLower(k)
			if vm, ok := v.(map[string]interface{}); ok {
				result[key] = arrLower(vm)
			} else if va, ok := v.([]interface{}); ok {
				arr := make([]interface{}, len(va))
				for i, item := range va {
					if itemm, ok := item.(map[string]interface{}); ok {
						arr[i] = arrLower(itemm)
					} else {
						arr[i] = item
					}
				}
				result[key] = arr
			} else {
				result[key] = v
			}
		}
	}
	
	return result
}

// GenerateRID generates new Request ID
func GenerateRID(ridCounter *int, prefix string) string {
	if *ridCounter < 0 || *ridCounter > 0x00FFFFFF {
		*ridCounter = 0
	}
	
	rid := fmt.Sprintf("%s%06X", prefix, *ridCounter)
	*ridCounter++
	return rid
}

// CreateRequest creates new JSP request
func CreateRequest(ridCounter *int, cmd string, params map[string]interface{}, cparams map[string]interface{}, rparams map[string]interface{}) *JSPRequest {
	rid := GenerateRID(ridCounter, "RID")
	
	request := &JSPRequest{
		ID:      rid,
		RKey:    rid,
		Cmd:     cmd,
		Time:    getMtf(),
		Params:  params,
		CParams: cparams,
		RParams: rparams,
	}
	
	return request
}

// SendRequest sends JSP request
func SendRequest(conn *JSPConnection, ridCounter *int, cmd string, params map[string]interface{}) (string, []byte, error) {
	var rid string
	
	// Create request if needed
	if params == nil {
		params = make(map[string]interface{})
	}
	
	data := make(map[string]interface{})
	data["cmd"] = cmd
	
	// Generate RID if not provided
	if _, hasRid := params["rid"]; !hasRid {
		rid = GenerateRID(ridCounter, "RID")
		data["rid"] = rid
	} else {
		rid = params["rid"].(string)
	}
	
	// Copy params to data
	for k, v := range params {
		data[k] = v
	}
	
	// Encode packet
	packet, err := EncodePacket(data)
	if err != nil {
		return "", nil, err
	}
	
	return rid, packet, nil
}

// AnswerRequest sends answer to JSP request
func AnswerRequest(rid string, params map[string]interface{}) ([]byte, error) {
	data := make(map[string]interface{})
	data["rid"] = rid
	
	// Copy params
	if params != nil {
		for k, v := range params {
			if k != "cmd" { // Remove cmd from answer
				data[k] = v
			}
		}
	}
	
	return EncodePacket(data)
}

// ProcessPacket processes incoming JSP packet
func ProcessPacket(packet map[string]interface{}) (string, map[string]interface{}, error) {
	// Check if it's a command
	if _, ok := packet["cmd"].(string); ok {
		return "command", packet, nil
	}
	
	// Check if it's an answer
	if _, ok := packet["rid"].(string); ok {
		return "answer", packet, nil
	}
	
	return "unknown", packet, nil
}

// TransformLockersData transforms lockers data from string to array
func TransformLockersData(pk map[string]interface{}) bool {
	if _, ok := pk["lockers_data"]; !ok {
		return false
	}
	
	// If already array, skip
	if _, ok := pk["lockers_data"].([]interface{}); ok {
		return false
	}
	
	lds, ok := pk["lockers_data"].(string)
	if !ok || len(lds) == 0 {
		return false
	}
	
	// Save original
	pk["_lockers_data"] = lds
	
	// Split by comma or semicolon
	parts := strings.FieldsFunc(lds, func(r rune) bool {
		return r == ',' || r == ';'
	})
	
	if len(parts) == 0 {
		return false
	}
	
	result := make([]interface{}, 0)
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		
		// Parse format: (LETTER:)?NUMBER or LETTER:NUMBER
		// Example: "62:180", "A:27", "M:12"
		var let string
		var num int
		
		if idx := strings.Index(part, ":"); idx >= 0 {
			let = part[:idx]
			if n, err := strconv.Atoi(part[idx+1:]); err == nil {
				num = n
			} else {
				continue
			}
		} else {
			// Just number
			if n, err := strconv.Atoi(part); err == nil {
				num = n
			} else {
				continue
			}
		}
		
		locker := map[string]interface{}{
			"auth_err":    0,
			"read_err":    0,
			"is_passtech": false,
			"block_no":    0,
			"locked":      true,
			"cab_no":      num,
		}
		
		// Check if letter is passtech (A-Z or -)
		if len(let) == 1 {
			if let == "-" {
				locker["is_passtech"] = true
				locker["litera"] = "-"
				locker["block_no"] = 0
			} else if let >= "A" && let <= "Z" {
				locker["is_passtech"] = true
				locker["litera"] = let
				locker["block_no"] = int([]rune(let)[0] - 'A' + 1)
			} else if let >= "0" && let <= "9" {
				// Numeric block
				if n, err := strconv.Atoi(let); err == nil {
					locker["block_no"] = n
				}
			}
		}
		
		result = append(result, locker)
	}
	
	if len(result) == 0 {
		return false
	}
	
	pk["lockers_data"] = result
	return true
}

// getMtf returns microtime as float64
func getMtf() float64 {
	if GetMtfFunc != nil {
		return GetMtfFunc()
	}
	return 0.0
}

// SetMtfFunc sets function to get microtime
var GetMtfFunc func() float64

// InitMtf initializes microtime function
func InitMtf(mtfFunc func() float64) {
	GetMtfFunc = mtfFunc
}

// SendRelayOpen sends relay open command
func SendRelayOpen(ridCounter *int, uid string, caption string, timeMs int, cid string) ([]byte, error) {
	data := map[string]interface{}{
		"cmd": "relay_open",
	}
	
	if uid != "" {
		data["uid"] = uid
	}
	if caption != "" {
		data["caption"] = caption
	}
	if timeMs > 0 {
		data["time"] = timeMs
	}
	if cid != "" {
		data["cid"] = cid
	}
	
	rid := GenerateRID(ridCounter, "RID")
	data["rid"] = rid
	
	return EncodePacket(data)
}

// SendRelayClose sends relay close command
func SendRelayClose(ridCounter *int) ([]byte, error) {
	data := map[string]interface{}{
		"cmd": "relay_close",
	}
	
	rid := GenerateRID(ridCounter, "RID")
	data["rid"] = rid
	
	return EncodePacket(data)
}

// CreateMessagePacket creates a JSP message packet (without RID counter)
func CreateMessagePacket(text string, timeMs int) []byte {
	data := map[string]interface{}{
		"cmd": "message",
		"text": text,
		"time": timeMs,
	}
	packet, err := EncodePacket(data)
	if err != nil {
		return nil
	}
	return packet
}

// SendMessage sends message command
func SendMessage(ridCounter *int, text string, timeMs int) ([]byte, error) {
	data := map[string]interface{}{
		"cmd": "message",
	}
	
	if text != "" {
		data["text"] = text
	}
	if timeMs > 0 {
		data["time"] = timeMs
	}
	
	rid := GenerateRID(ridCounter, "RID")
	data["rid"] = rid
	
	return EncodePacket(data)
}

// SendPing sends ping command
func SendPing(ridCounter *int) ([]byte, error) {
	data := map[string]interface{}{
		"cmd": "ping",
	}
	
	rid := GenerateRID(ridCounter, "RID")
	data["rid"] = rid
	
	return EncodePacket(data)
}

