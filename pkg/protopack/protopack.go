// Package protopack implements TLV-based binary serialization format
// compatible with PHP proto_pack.inc.
// Format: each value is encoded as: Tag(1 byte) + Length(3 bytes BE) + Value(Length bytes)
package protopack

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Data type tags
const (
	DT_BOOL = 0x01
	DT_INT  = 0x02
	DT_DBL  = 0x03
	DT_STR  = 0x04
	DT_NUL  = 0x05
	DT_HASH = 0x06
	DT_ARR  = 0x07
	DT_BIN  = 0x08
)

// --- Encoding ---

// EncodeValue encodes a Go value into proto_pack binary format
func EncodeValue(value interface{}) []byte {
	switch v := value.(type) {
	case bool:
		val := byte(0)
		if v {
			val = 1
		}
		return encodeTLV(DT_BOOL, []byte{val})
	case int:
		return encodeTLV(DT_INT, encodeInt(int64(v)))
	case int64:
		return encodeTLV(DT_INT, encodeInt(v))
	case float64:
		return encodeTLV(DT_DBL, encodeDouble(v))
	case string:
		tag := DT_STR
		if isBinaryString(v) {
			tag = DT_BIN
		}
		return encodeTLV(byte(tag), []byte(v))
	case nil:
		return encodeTLV(DT_NUL, nil)
	case []interface{}:
		return encodeTLV(DT_ARR, EncodeArray(v))
	case map[string]interface{}:
		return encodeTLV(DT_HASH, EncodeHash(v))
	default:
		return encodeTLV(DT_NUL, nil)
	}
}

// EncodeArray encodes an array of values (proto_pack_encode_arr_ex)
func EncodeArray(arr []interface{}) []byte {
	var result []byte
	for _, v := range arr {
		result = append(result, EncodeValue(v)...)
	}
	return result
}

// EncodeArrayEx encodes top-level array values sequentially (for TCP wire format)
func EncodeArrayEx(arr []interface{}) []byte {
	return EncodeArray(arr)
}

// EncodeHash encodes a key-value map (proto_pack_encode_arr)
func EncodeHash(m map[string]interface{}) []byte {
	var result []byte
	for k, v := range m {
		result = append(result, EncodeValue(k)...)
		result = append(result, EncodeValue(v)...)
	}
	return result
}

// --- Decoding ---

// DecodeArrayEx decodes sequential values from proto_pack binary data
// Returns a slice of decoded values (proto_pack_decode_arr_ex equivalent)
func DecodeArrayEx(data []byte) ([]interface{}, error) {
	var result []interface{}
	offset := 0

	for offset < len(data) {
		val, newOffset, err := DecodeValue(data, offset)
		if err != nil {
			if len(result) == 0 {
				return nil, err
			}
			break // Return what we have
		}
		result = append(result, val)
		offset = newOffset
	}

	return result, nil
}

// DecodeValue decodes a single value from data at given offset
// Returns the value, new offset, and error
func DecodeValue(data []byte, offset int) (interface{}, int, error) {
	tag, length, valueData, err := decodeTLV(data, offset)
	if err != nil {
		return nil, offset, err
	}

	newOffset := offset + 4 + length // 1 byte tag + 3 bytes length + value

	switch tag {
	case DT_BOOL:
		if length >= 1 {
			return valueData[0] == 1, newOffset, nil
		}
		return false, newOffset, nil

	case DT_INT:
		return decodeInt(valueData, length), newOffset, nil

	case DT_DBL:
		return decodeDouble(valueData), newOffset, nil

	case DT_STR:
		return string(valueData), newOffset, nil

	case DT_NUL:
		return nil, newOffset, nil

	case DT_HASH:
		m, err := decodeHash(valueData)
		return m, newOffset, err

	case DT_ARR:
		arr, err := DecodeArrayEx(valueData)
		return arr, newOffset, err

	case DT_BIN:
		return string(valueData), newOffset, nil

	default:
		return nil, newOffset, fmt.Errorf("unknown proto_pack type: 0x%02X", tag)
	}
}

// decodeHash decodes key-value pairs from proto_pack data
func decodeHash(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	offset := 0

	for offset < len(data) {
		// Decode key
		keyVal, newOffset, err := DecodeValue(data, offset)
		if err != nil {
			break
		}
		offset = newOffset

		key, ok := keyVal.(string)
		if !ok {
			key = fmt.Sprintf("%v", keyVal)
		}

		// Decode value
		val, newOffset2, err := DecodeValue(data, offset)
		if err != nil {
			break
		}
		offset = newOffset2

		result[key] = val
	}

	return result, nil
}

// --- TLV helpers ---

func encodeTLV(tag byte, value []byte) []byte {
	length := len(value)
	result := make([]byte, 4+length)
	result[0] = tag
	result[1] = byte((length >> 16) & 0xFF)
	result[2] = byte((length >> 8) & 0xFF)
	result[3] = byte(length & 0xFF)
	if length > 0 {
		copy(result[4:], value)
	}
	return result
}

func decodeTLV(data []byte, offset int) (byte, int, []byte, error) {
	remaining := len(data) - offset
	if remaining < 4 {
		return 0, 0, nil, fmt.Errorf("not enough data for TLV header at offset %d", offset)
	}

	tag := data[offset]
	length := (int(data[offset+1]) << 16) | (int(data[offset+2]) << 8) | int(data[offset+3])

	if remaining-4 < length {
		return 0, 0, nil, fmt.Errorf("not enough data for TLV value: need %d, have %d", length, remaining-4)
	}

	value := data[offset+4 : offset+4+length]
	return tag, length, value, nil
}

// --- Number encoding/decoding ---

// encodeInt encodes an integer as big-endian bytes (1-4 bytes as needed)
func encodeInt(val int64) []byte {
	absVal := val
	if absVal < 0 {
		absVal = -absVal
	}

	var numBytes int
	switch {
	case absVal <= 0x7F:
		numBytes = 1
	case absVal <= 0x7FFF:
		numBytes = 2
	case absVal <= 0x7FFFFF:
		numBytes = 3
	default:
		numBytes = 4
	}

	encoded := uint32(absVal)
	if val < 0 {
		switch numBytes {
		case 1:
			encoded |= 0x80
		case 2:
			encoded |= 0x8000
		case 3:
			encoded |= 0x800000
		case 4:
			encoded |= 0x80000000
		}
	}

	result := make([]byte, numBytes)
	for i := numBytes - 1; i >= 0; i-- {
		result[numBytes-1-i] = byte((encoded >> (uint(i) * 8)) & 0xFF)
	}
	return result
}

// decodeInt decodes an integer from big-endian bytes
func decodeInt(data []byte, length int) int64 {
	if length == 0 || len(data) == 0 {
		return 0
	}
	if length > len(data) {
		length = len(data)
	}

	var val uint32
	for i := 0; i < length; i++ {
		val = (val << 8) | uint32(data[i])
	}

	// Check sign bit
	var signMask uint32
	switch length {
	case 1:
		signMask = 0x80
	case 2:
		signMask = 0x8000
	case 3:
		signMask = 0x800000
	default:
		signMask = 0x80000000
	}

	neg := (val & signMask) != 0
	val &= signMask - 1 // Clear sign bit

	result := int64(val)
	if neg {
		result = -result
	}
	return result
}

// encodeDouble encodes a float64 as 8 bytes (int part 4 bytes + frac part 4 bytes)
func encodeDouble(val float64) []byte {
	neg := val < 0
	if neg {
		val = -val
	}

	intPart := int64(val) & 0x7FFFFFFF
	fracPart := val - float64(intPart)
	fracInt := int64(fracPart * 1000000000) // proto_pack_prec_mask(4) = 0x3B9ACA00

	if neg {
		intPart |= 0x80000000
	}

	result := make([]byte, 8)
	binary.BigEndian.PutUint32(result[0:4], uint32(intPart))
	binary.BigEndian.PutUint32(result[4:8], uint32(fracInt))
	return result
}

// decodeDouble decodes a float64 from 8 bytes
func decodeDouble(data []byte) float64 {
	if len(data) < 8 {
		if len(data) >= 4 {
			// Integer only
			return float64(decodeInt(data, len(data)))
		}
		return 0
	}

	intVal := binary.BigEndian.Uint32(data[0:4])
	fracVal := binary.BigEndian.Uint32(data[4:8])

	neg := (intVal & 0x80000000) != 0
	intVal &= 0x7FFFFFFF

	result := float64(intVal) + float64(fracVal)/1000000000.0

	if neg {
		result = -result
	}
	return result
}

// isBinaryString checks if string contains binary control characters
func isBinaryString(s string) bool {
	for _, c := range s {
		if (c >= 0x00 && c <= 0x08) || (c >= 0x0B && c <= 0x0C) || (c >= 0x0E && c <= 0x1F) {
			return true
		}
	}
	return false
}

// Dump creates a hex dump of binary data (for debugging)
func Dump(data []byte) string {
	var result string
	for i := 0; i < len(data); i += 16 {
		result += fmt.Sprintf("0x%08X ", i)
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		ascii := ""
		for j := i; j < end; j++ {
			result += fmt.Sprintf(" %02X", data[j])
			if data[j] > 32 && data[j] < 127 {
				ascii += string(rune(data[j]))
			} else {
				ascii += "."
			}
		}
		for j := end; j < i+16; j++ {
			result += "   "
		}
		result += "  " + ascii + "\n"
	}
	return result
}

// Round helper for float precision
func init() {
	_ = math.Round // ensure math package is imported
}
