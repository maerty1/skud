package termlogs

import (
	"sync"
	"time"
)

// TermLogEntry represents a single terminal log entry
type TermLogEntry struct {
	TKey   string                 `json:"tkey"`
	Type   string                 `json:"type"` // TAG_READ, BARCODE_READ, ACTION_COMPLETED, CARD_IDENT, etc.
	Time   time.Time              `json:"time"`
	UID    string                 `json:"uid,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
	Action *TermLogAction         `json:"action,omitempty"`
}

// TermLogAction represents an action result attached to a log entry
type TermLogAction struct {
	Time   time.Time              `json:"time"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// TermLogs stores per-terminal event logs in memory
type TermLogs struct {
	logs     map[string][]TermLogEntry // termKey -> entries
	maxSize  int                       // max entries per terminal
	mutex    sync.RWMutex
}

// NewTermLogs creates a new TermLogs instance
// maxSize: max entries per terminal (0 = unlimited)
func NewTermLogs(maxSize int) *TermLogs {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &TermLogs{
		logs:    make(map[string][]TermLogEntry),
		maxSize: maxSize,
	}
}

// Add adds a log entry for a terminal. Returns a pointer to the added entry.
func (tl *TermLogs) Add(termKey string, entry TermLogEntry) *TermLogEntry {
	tl.mutex.Lock()
	defer tl.mutex.Unlock()

	if _, ok := tl.logs[termKey]; !ok {
		tl.logs[termKey] = make([]TermLogEntry, 0, 64)
	}

	tl.logs[termKey] = append(tl.logs[termKey], entry)

	// Trim if exceeds max size
	if len(tl.logs[termKey]) > tl.maxSize {
		trimCount := len(tl.logs[termKey]) - tl.maxSize
		tl.logs[termKey] = tl.logs[termKey][trimCount:]
	}

	return &tl.logs[termKey][len(tl.logs[termKey])-1]
}

// GetAll returns all term log keys and their counts
func (tl *TermLogs) GetAll() map[string]int {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	result := make(map[string]int, len(tl.logs))
	for k, v := range tl.logs {
		result[k] = len(v)
	}
	return result
}

// Get returns all entries for a terminal
func (tl *TermLogs) Get(termKey string) []TermLogEntry {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	entries, ok := tl.logs[termKey]
	if !ok {
		return nil
	}

	result := make([]TermLogEntry, len(entries))
	copy(result, entries)
	return result
}

// Count returns the number of entries for a terminal
func (tl *TermLogs) Count(termKey string) int {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	return len(tl.logs[termKey])
}

// GetPage returns a page of entries for a terminal
// pageSize: entries per page (1-500, default 20)
// pageNo: 0-based page number, "first", "last"
// reversed: if true, returns newest first
func (tl *TermLogs) GetPage(termKey string, pageSize int, pageNo int, reversed bool) ([]TermLogEntry, int, error) {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	entries, ok := tl.logs[termKey]
	if !ok {
		return nil, 0, nil
	}

	lcount := len(entries)
	if lcount == 0 {
		return nil, 0, nil
	}

	if pageSize < 1 || pageSize > 500 {
		pageSize = 20
	}

	pmax := (lcount - 1) / pageSize
	totalPages := pmax + 1

	if pageNo < 0 {
		pageNo = 0
	}
	if pageNo > pmax {
		return nil, totalPages, nil // Empty - page out of range
	}

	offset := pageNo * pageSize
	slen := pageSize
	if offset+slen > lcount {
		slen = lcount - offset
	}

	if reversed {
		offset = lcount - offset - slen
	}

	result := make([]TermLogEntry, slen)
	copy(result, entries[offset:offset+slen])

	if reversed {
		// Reverse the result slice
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
	}

	return result, totalPages, nil
}

// GetCommonCount returns total entry count across all terminals
func (tl *TermLogs) GetCommonCount() int {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	total := 0
	for _, entries := range tl.logs {
		total += len(entries)
	}
	return total
}
