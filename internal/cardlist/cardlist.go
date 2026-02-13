package cardlist

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// validHexUID checks that a card UID is 8-20 hex characters (parse_gmc equivalent)
var validHexUID = regexp.MustCompile(`^[0-9A-F]{8,20}$`)

// CardList manages a deny-list of card UIDs with associated messages.
// gmclist = global master card list (checked first, blocks with message)
// mclist  = secondary card list (checked after gmclist)
type CardList struct {
	gmclist map[string]string // uid -> message (global deny list)
	mclist  map[string]string // uid -> message (secondary deny list)
	mutex   sync.RWMutex
	file    string // optional persistence file
}

// NewCardList creates a new CardList
func NewCardList() *CardList {
	return &CardList{
		gmclist: make(map[string]string),
		mclist:  make(map[string]string),
	}
}

// SetPersistFile sets the file path for persistence
func (cl *CardList) SetPersistFile(path string) {
	cl.file = path
}

// parseGMC validates and normalizes card UID (8-20 hex chars, uppercase)
func parseGMC(uid string) string {
	uid = strings.ToUpper(strings.TrimSpace(uid))
	if validHexUID.MatchString(uid) {
		return uid
	}
	return ""
}

// --- gmclist operations (global deny list) ---

// CheckGlobal checks if a UID is in the global deny list.
// Returns message if found, empty string if not.
func (cl *CardList) CheckGlobal(uid string) string {
	uid = strings.ToUpper(strings.TrimSpace(uid))
	cl.mutex.RLock()
	defer cl.mutex.RUnlock()
	if msg, ok := cl.gmclist[uid]; ok {
		return msg
	}
	return ""
}

// CheckSecondary checks if a UID is in the secondary deny list.
// Returns message if found, empty string if not.
func (cl *CardList) CheckSecondary(uid string) string {
	uid = strings.ToUpper(strings.TrimSpace(uid))
	cl.mutex.RLock()
	defer cl.mutex.RUnlock()
	if msg, ok := cl.mclist[uid]; ok {
		return msg
	}
	return ""
}

// GetGlobalList returns a copy of the global deny list
func (cl *CardList) GetGlobalList() map[string]string {
	cl.mutex.RLock()
	defer cl.mutex.RUnlock()
	result := make(map[string]string, len(cl.gmclist))
	for k, v := range cl.gmclist {
		result[k] = v
	}
	return result
}

// GetSecondaryList returns a copy of the secondary deny list
func (cl *CardList) GetSecondaryList() map[string]string {
	cl.mutex.RLock()
	defer cl.mutex.RUnlock()
	result := make(map[string]string, len(cl.mclist))
	for k, v := range cl.mclist {
		result[k] = v
	}
	return result
}

// AddGlobal adds cards to the global deny list. Returns list of actually added UIDs.
func (cl *CardList) AddGlobal(cards []CardEntry) []string {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	var added []string
	for _, c := range cards {
		uid := parseGMC(c.UID)
		if uid == "" {
			continue
		}
		cl.gmclist[uid] = c.Message
		added = append(added, uid)
	}
	if len(added) > 0 {
		cl.persistAsync()
	}
	return added
}

// AddSecondary adds cards to the secondary deny list. Returns list of actually added UIDs.
func (cl *CardList) AddSecondary(cards []CardEntry) []string {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	var added []string
	for _, c := range cards {
		uid := parseGMC(c.UID)
		if uid == "" {
			continue
		}
		cl.mclist[uid] = c.Message
		added = append(added, uid)
	}
	if len(added) > 0 {
		cl.persistAsync()
	}
	return added
}

// DelGlobal removes cards from the global deny list. Returns list of actually removed UIDs.
func (cl *CardList) DelGlobal(uids []string) []string {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	var removed []string
	for _, uid := range uids {
		uid = parseGMC(uid)
		if uid == "" {
			continue
		}
		if _, ok := cl.gmclist[uid]; ok {
			delete(cl.gmclist, uid)
			removed = append(removed, uid)
		}
	}
	if len(removed) > 0 {
		cl.persistAsync()
	}
	return removed
}

// DelSecondary removes cards from the secondary deny list. Returns list of actually removed UIDs.
func (cl *CardList) DelSecondary(uids []string) []string {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	var removed []string
	for _, uid := range uids {
		uid = parseGMC(uid)
		if uid == "" {
			continue
		}
		if _, ok := cl.mclist[uid]; ok {
			delete(cl.mclist, uid)
			removed = append(removed, uid)
		}
	}
	if len(removed) > 0 {
		cl.persistAsync()
	}
	return removed
}

// SyncGlobal synchronizes the global list to match the provided UIDs.
// Adds missing UIDs, removes UIDs not in the list.
// Returns map with "add" and "del" keys.
func (cl *CardList) SyncGlobal(cards []CardEntry) map[string][]string {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()

	incoming := make(map[string]string)
	for _, c := range cards {
		uid := parseGMC(c.UID)
		if uid != "" {
			incoming[uid] = c.Message
		}
	}

	result := map[string][]string{
		"add": {},
		"del": {},
	}

	// Add missing
	for uid, msg := range incoming {
		if _, exists := cl.gmclist[uid]; !exists {
			cl.gmclist[uid] = msg
			result["add"] = append(result["add"], uid)
		}
	}

	// Remove extra
	for uid := range cl.gmclist {
		if _, exists := incoming[uid]; !exists {
			delete(cl.gmclist, uid)
			result["del"] = append(result["del"], uid)
		}
	}

	if len(result["add"]) > 0 || len(result["del"]) > 0 {
		cl.persistAsync()
	}

	return result
}

// CardEntry represents a card entry with UID and message
type CardEntry struct {
	UID     string `json:"uid"`
	Message string `json:"message"`
}

// --- Persistence ---

type persistData struct {
	GMCList map[string]string `json:"gmclist"`
	MCList  map[string]string `json:"mclist"`
}

func (cl *CardList) persistAsync() {
	if cl.file == "" {
		return
	}
	// Save in background
	data := persistData{
		GMCList: make(map[string]string, len(cl.gmclist)),
		MCList:  make(map[string]string, len(cl.mclist)),
	}
	for k, v := range cl.gmclist {
		data.GMCList[k] = v
	}
	for k, v := range cl.mclist {
		data.MCList[k] = v
	}
	go func() {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Printf("CardList: failed to marshal: %v\n", err)
			return
		}
		if err := os.WriteFile(cl.file, jsonData, 0644); err != nil {
			fmt.Printf("CardList: failed to save: %v\n", err)
		}
	}()
}

// Load loads card lists from persistence file
func (cl *CardList) Load() error {
	if cl.file == "" {
		return nil
	}

	fileData, err := os.ReadFile(cl.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's OK
		}
		return fmt.Errorf("failed to read card list file: %v", err)
	}

	var data persistData
	if err := json.Unmarshal(fileData, &data); err != nil {
		return fmt.Errorf("failed to parse card list file: %v", err)
	}

	cl.mutex.Lock()
	defer cl.mutex.Unlock()

	if data.GMCList != nil {
		cl.gmclist = data.GMCList
	}
	if data.MCList != nil {
		cl.mclist = data.MCList
	}

	fmt.Printf("CardList: loaded %d global, %d secondary entries\n", len(cl.gmclist), len(cl.mclist))
	return nil
}
