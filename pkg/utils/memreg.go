package utils

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// MEMREG modes
const (
	MEMREG_MODE_AUTO = 0x00
	MEMREG_MODE_SET  = 0x01 // add
	MEMREG_MODE_CLR  = 0x02 // clear/del
	MEMREG_MODE_DISP = 0x03 // display
	MEMREG_MODE_TAKE = 0x04 // take
)

// MemRegKey represents parsed MEMREG key
type MemRegKey struct {
	Storage string // Storage key (e.g., "towel")
	Mode    int    // Mode (SET, CLR, TAKE, etc.)
}

// MemRegStorage manages MEMREG storage in memory
type MemRegStorage struct {
	storage map[string]map[string]interface{} // storage_key -> uid_key -> value
	mutex   sync.RWMutex
}

var globalMemRegStorage *MemRegStorage
var memRegOnce sync.Once

// GetMemRegStorage returns global MEMREG storage instance
func GetMemRegStorage() *MemRegStorage {
	memRegOnce.Do(func() {
		globalMemRegStorage = &MemRegStorage{
			storage: make(map[string]map[string]interface{}),
		}
	})
	return globalMemRegStorage
}

// ParseMemRegKey parses MEMREG key in format "storage/mode" or "storage"
// Returns storage key and mode
func ParseMemRegKey(key string) (*MemRegKey, error) {
	if key == "" {
		return nil, fmt.Errorf("empty key")
	}

	key = strings.TrimSpace(key)
	if len(key) < 1 || len(key) > 64 {
		return nil, fmt.Errorf("invalid key length")
	}

	// Match pattern: ([A-Za-z\d\_\-\+]+)(?:\/([A-Za-z\d\_\-\+]+))?
	re := regexp.MustCompile(`^([A-Za-z\d\_\-\+]+)(?:\/([A-Za-z\d\_\-\+]+))?$`)
	matches := re.FindStringSubmatch(key)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid key format")
	}

	storage := matches[1]
	mode := MEMREG_MODE_AUTO

	if len(matches) > 2 && matches[2] != "" {
		smode := strings.ToLower(strings.TrimSpace(matches[2]))
		switch smode {
		case "set", "add":
			mode = MEMREG_MODE_SET
		case "clr", "clear", "del":
			mode = MEMREG_MODE_CLR
		case "disp":
			mode = MEMREG_MODE_DISP
		case "take":
			mode = MEMREG_MODE_TAKE
		}
	}

	return &MemRegKey{
		Storage: storage,
		Mode:    mode,
	}, nil
}

// Set sets value in storage for given storage_key and uid_key
func (mrs *MemRegStorage) Set(storageKey, uidKey string, value interface{}) error {
	sk, err := ParseMemRegKey(storageKey)
	if err != nil {
		return fmt.Errorf("invalid storage key: %v", err)
	}

	uk, err := ParseMemRegKey(uidKey)
	if err != nil {
		return fmt.Errorf("invalid uid key: %v", err)
	}

	mrs.mutex.Lock()
	defer mrs.mutex.Unlock()

	if mrs.storage[sk.Storage] == nil {
		mrs.storage[sk.Storage] = make(map[string]interface{})
	}

	mrs.storage[sk.Storage][uk.Storage] = value
	return nil
}

// Get gets value from storage for given storage_key and uid_key
// Returns nil if not found
func (mrs *MemRegStorage) Get(storageKey, uidKey string) (interface{}, error) {
	sk, err := ParseMemRegKey(storageKey)
	if err != nil {
		return nil, fmt.Errorf("invalid storage key: %v", err)
	}

	uk, err := ParseMemRegKey(uidKey)
	if err != nil {
		return nil, fmt.Errorf("invalid uid key: %v", err)
	}

	mrs.mutex.RLock()
	defer mrs.mutex.RUnlock()

	if mrs.storage[sk.Storage] == nil {
		return nil, nil // Not found
	}

	if val, exists := mrs.storage[sk.Storage][uk.Storage]; exists {
		return val, nil
	}

	return nil, nil // Not found
}

// Del deletes value from storage
func (mrs *MemRegStorage) Del(storageKey, uidKey string) error {
	sk, err := ParseMemRegKey(storageKey)
	if err != nil {
		return fmt.Errorf("invalid storage key: %v", err)
	}

	uk, err := ParseMemRegKey(uidKey)
	if err != nil {
		return fmt.Errorf("invalid uid key: %v", err)
	}

	mrs.mutex.Lock()
	defer mrs.mutex.Unlock()

	if mrs.storage[sk.Storage] != nil {
		delete(mrs.storage[sk.Storage], uk.Storage)
	}

	return nil
}

// Has checks if value exists in storage
func (mrs *MemRegStorage) Has(storageKey, uidKey string) (bool, error) {
	val, err := mrs.Get(storageKey, uidKey)
	if err != nil {
		return false, err
	}
	return val != nil, nil
}

