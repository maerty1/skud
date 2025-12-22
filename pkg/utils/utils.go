package utils

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"nd-go/pkg/types"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GetMtf returns microtime as float64
func GetMtf() float64 {
	now := time.Now()
	return float64(now.Unix()) + float64(now.Nanosecond())/1e9
}

// CRC8 calculates CRC8 checksum
func CRC8(data []byte) uint8 {
	crc := uint8(0)
	for _, b := range data {
		crc ^= b
		for i := 0; i < 8; i++ {
			if (crc & 0x80) != 0 {
				crc = (crc << 1) ^ 0x31
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// EncodeUint16 encodes uint16 to little-endian bytes
func EncodeUint16(val uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, val)
	return buf
}

// EncodeUint32 encodes uint32 to little-endian bytes
func EncodeUint32(val uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, val)
	return buf
}

// DecodeUint16 decodes uint16 from little-endian bytes
func DecodeUint16(data []byte) uint16 {
	if len(data) < 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(data)
}

// DecodeUint32 decodes uint32 from little-endian bytes
func DecodeUint32(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

// EncodeTLV encodes Type-Length-Value structure
func EncodeTLV(tag uint8, value []byte) []byte {
	result := []byte{tag}
	if len(value) > 0 {
		result = append(result, uint8(len(value)))
		result = append(result, value...)
	}
	return result
}

// DecodeTLV decodes Type-Length-Value structure
func DecodeTLV(data []byte) (uint8, []byte, int) {
	if len(data) < 2 {
		return 0, nil, 0
	}

	tag := data[0]
	length := int(data[1])
	offset := 2

	if len(data) < offset+length {
		return tag, nil, 0
	}

	value := data[offset : offset+length]
	return tag, value, offset + length
}

// BytesToHex converts bytes to hex string
func BytesToHex(data []byte) string {
	return fmt.Sprintf("%X", data)
}

// HexToBytes converts hex string to bytes
func HexToBytes(hexStr string) ([]byte, error) {
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	return hex.DecodeString(hexStr)
}

// GetStringValue safely gets string value from map
func GetStringValue(data map[string]interface{}, key string, defaultValue string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// IsIP validates IP address
func IsIP(ip string) bool {
	return net.ParseIP(strings.TrimSpace(ip)) != nil
}

// ParseTType parses terminal type
func ParseTType(ttype string) types.TerminalType {
	switch strings.ToLower(strings.TrimSpace(ttype)) {
	case "gat":
		return types.TTYPE_GAT
	case "pocket":
		return types.TTYPE_POCKET
	case "sphinx":
		return types.TTYPE_SPHINX
	case "jsp":
		return types.TTYPE_JSP
	default:
		return types.TTYPE_GAT
	}
}

// ParseAval parses value (string/bool/int/float)
func ParseAval(aval string) interface{} {
	aval = strings.TrimSpace(aval)
	if aval == "" {
		return false
	}
	if aval == "false" {
		return false
	}
	if aval == "true" {
		return true
	}
	if strings.Contains(aval, ",") {
		parts := strings.Split(aval, ",")
		result := make([]interface{}, len(parts))
		for i, part := range parts {
			result[i] = ParseAval(part)
		}
		return result
	}
	if intVal, err := strconv.Atoi(aval); err == nil {
		return intVal
	}
	if floatVal, err := strconv.ParseFloat(aval, 64); err == nil {
		return floatVal
	}
	return aval
}

// ParseTerm parses terminal configuration string
func ParseTerm(termStr string) (*types.TerminalSettings, error) {
	termStr = strings.TrimSpace(termStr)
	parts := strings.Split(termStr, ":")

	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid term format")
	}

	id := strings.TrimSpace(parts[0])
	ip := id
	port := 8080 // Default port (will be adjusted based on type)

	if len(parts) > 1 && IsIP(parts[1]) {
		ip = strings.TrimSpace(parts[1])
	} else {
		id = ""
	}

	if !IsIP(ip) {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	ttype := types.TTYPE_POCKET
	utf := false
	regQuery := false
	pairs := make(map[string]interface{})

	// First pass: parse all parameters to determine type
	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])
		if part == "" || strings.HasPrefix(part, "_") {
			continue
		}

		// Try to parse as port (numeric value)
		if intVal, err := strconv.Atoi(part); err == nil && intVal > 0 {
			port = intVal
			continue
		}

		// Parse key=value pairs
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := strings.ToLower(strings.TrimSpace(kv[0]))
				val := ParseAval(strings.TrimSpace(kv[1]))
				pairs[key] = val
				// Also store original case for specific keys that need it
				if key == "memreg_dev" || key == "memreg_deny" {
					pairs[kv[0]] = kv[1] // Store original key with original value
				}
			}
		} else {
			// Boolean flag (e.g., "memreg", "utf", "r")
			key := strings.ToLower(strings.TrimSpace(part))
			pairs[key] = true
		}
	}

	// Determine type from pairs
	if val, ok := pairs["type"]; ok {
		if strVal, ok := val.(string); ok {
			ttype = ParseTType(strVal)
		}
	}
	pairs["type"] = ttype

	// Set default port based on type if port wasn't explicitly set
	// Check if port was set in the loop above
	portSet := false
	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])
		if intVal, err := strconv.Atoi(part); err == nil && intVal > 0 {
			portSet = true
			break
		}
	}
	
	if !portSet {
		// Set default port based on terminal type
		switch ttype {
		case types.TTYPE_GAT:
			port = 8000 // GAT default port
		case types.TTYPE_SPHINX:
			port = 3312 // SPHINX default port (from examples)
		case types.TTYPE_JSP:
			port = 8902 // JSP default port
		case types.TTYPE_POCKET:
			port = 8080 // POCKET default port
		default:
			port = 8080
		}
	}

	if val, ok := pairs["u"]; ok {
		if boolVal, ok := val.(bool); ok {
			utf = boolVal
		}
	}

	if val, ok := pairs["r"]; ok {
		if boolVal, ok := val.(bool); ok {
			regQuery = boolVal
		}
	}

	settings := &types.TerminalSettings{
		ID:           id,
		IP:           ip,
		Port:         port,
		Type:         ttype,
		UTF:          utf,
		RegQuery:     regQuery,
		ConfigString: termStr,
		Extra:        pairs,
	}

	return settings, nil
}

// GenID generates unique ID
func GenID() string {
	hash := md5.Sum([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return hex.EncodeToString(hash[:])[:16]
}

// Colon2NL converts semicolons to newlines
func Colon2NL(s string) string {
	return strings.ReplaceAll(s, ";", "\n")
}

// FromUTF8 converts UTF-8 to Windows-1251 (simplified)
func FromUTF8(s string) string {
	// Simplified conversion - in real implementation use proper charset conversion
	return s
}

// ToUTF8 converts Windows-1251 to UTF-8 (simplified)
func ToUTF8(s string) string {
	// Simplified conversion - in real implementation use proper charset conversion
	return s
}

// NDs formats data as hex string
func NDs(data []byte, sep string, prefix string) string {
	result := make([]string, len(data))
	for i, b := range data {
		result[i] = fmt.Sprintf("%s%02X", prefix, b)
	}
	return strings.Join(result, sep)
}

// LogDate formats date for logging
func LogDate() string {
	return time.Now().Format("[02-01-2006 15:04:05] ")
}

// Plog formats log message
func Plog(str string) string {
	return LogDate() + strings.ReplaceAll(str, "\r", "")
}

// ValidateGMC validates GMC (hex string)
func ValidateGMC(c string) bool {
	matched, _ := regexp.MatchString(`^[\da-fA-F]{8,20}$`, strings.TrimSpace(c))
	return matched
}

// ParseGMC parses GMC string
func ParseGMC(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if ValidateGMC(c) {
		return c
	}
	return ""
}

// CryptStrToHex converts string to hex
func CryptStrToHex(s string) string {
	return hex.EncodeToString([]byte(s))
}

// CryptHexToStr converts hex to string
func CryptHexToStr(h string) ([]byte, error) {
	return hex.DecodeString(h)
}

// GetNL returns newline character
func GetNL() string {
	return "\n"
}

// DayToUnixTime converts day number to unix timestamp
func DayToUnixTime(day int) int64 {
	return int64(day) * 86400
}

// DayFromUnixTime converts unix timestamp to day number
func DayFromUnixTime(ts int64) int {
	return int(ts / 86400)
}

// CorrectUnixTime corrects unix time for DST (simplified)
func CorrectUnixTime(t int64) int64 {
	// Simplified DST correction
	_, offset := time.Unix(t, 0).Zone()
	return t - int64(offset)
}

// CorrectUnixTimeR reverses DST correction
func CorrectUnixTimeR(t int64) int64 {
	// Simplified DST correction
	_, offset := time.Unix(t, 0).Zone()
	return t + int64(offset)
}

// FilterTerminalList filters terminals by IP address using regex
// filter: regex pattern (e.g., `/192\.168\.12\.2(3|4)(2|3|4|5|6|7|8)/`)
// filterAbsent: if true, exclude matching terminals; if false, include only matching
// Returns true if terminal should be included
func FilterTerminalList(ip string, filter string, filterAbsent bool) bool {
	if filter == "" {
		return true // No filter, include all
	}

	// Remove leading/trailing slashes if present (PHP regex format)
	filterPattern := strings.Trim(filter, "/")

	matched, err := regexp.MatchString(filterPattern, ip)
	if err != nil {
		// Invalid regex, include terminal by default
		return true
	}

	if filterAbsent {
		// Exclude matching terminals
		return !matched
	}
	// Include only matching terminals
	return matched
}
