package gtime

import (
	"encoding/csv"
	"fmt"
	"os"
	"sync"
	"time"
)

// GTimeLogger logs solar (time-based) terminal events to monthly CSV files.
// Equivalent of register_gtime_event / gtime_log_fname in PHP.
type GTimeLogger struct {
	logDir string
	keys   []string
	mutex  sync.Mutex
}

// DefaultKeys returns the default column keys for GTime CSV logging
func DefaultKeys() []string {
	return []string{
		"timestamp",
		"id",
		"addres",
		"type",
		"uid",
	}
}

// NewGTimeLogger creates a new GTime logger
// logDir: directory path for log files (e.g. "/var/www/html/gtime/")
// keys: CSV column headers
func NewGTimeLogger(logDir string, keys []string) *GTimeLogger {
	if len(keys) == 0 {
		keys = DefaultKeys()
	}
	return &GTimeLogger{
		logDir: logDir,
		keys:   keys,
	}
}

// RegisterEvent logs a solar event to the monthly CSV file
func (g *GTimeLogger) RegisterEvent(data map[string]string) error {
	if g.logDir == "" {
		return nil // Disabled
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()

	fname, err := g.getLogFileName()
	if err != nil {
		return err
	}

	// Build CSV row based on keys
	row := make([]string, len(g.keys))
	for i, key := range g.keys {
		if val, ok := data[key]; ok {
			row[i] = val
		}
	}

	// Append to file
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open gtime log: %v", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.UseCRLF = true
	if err := w.Write(row); err != nil {
		return fmt.Errorf("failed to write gtime event: %v", err)
	}
	w.Flush()
	return w.Error()
}

// getLogFileName returns the current log file path, creating it with headers if needed
// Format: logDir/mm_yyyy.txt
func (g *GTimeLogger) getLogFileName() (string, error) {
	now := time.Now()
	dirName := fmt.Sprintf("%02d_%04d", now.Month(), now.Year())
	filePath := g.logDir + dirName + ".txt"

	// Check if file exists, create with header if not
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Ensure directory exists
		if err := os.MkdirAll(g.logDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create gtime log dir: %v", err)
		}

		// Create file with header
		f, err := os.Create(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to create gtime log file: %v", err)
		}
		w := csv.NewWriter(f)
		w.UseCRLF = true
		if err := w.Write(g.keys); err != nil {
			f.Close()
			return "", fmt.Errorf("failed to write gtime header: %v", err)
		}
		w.Flush()
		f.Close()
	}

	return filePath, nil
}
