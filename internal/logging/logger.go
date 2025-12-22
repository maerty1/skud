package logging

import (
	"fmt"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Logger represents logging system
type Logger struct {
	config     *types.Config
	logFile    *os.File
	screenFile *os.File
	mutex      sync.Mutex
	// Ring buffer for in-memory logs
	logBuffer   []LogEntry
	bufferSize  int
	bufferPos   int
	bufferMutex sync.RWMutex
	// Log rotation state
	logFileSize    int64
	screenFileSize int64
	lastRotationCheck time.Time
}

// NewLogger creates new logger instance
func NewLogger(config *types.Config) *Logger {
	bufferSize := 1000 // Store last 1000 log entries in memory
	logger := &Logger{
		config:     config,
		logBuffer:  make([]LogEntry, bufferSize),
		bufferSize: bufferSize,
		bufferPos:  0,
	}

	// Create logs directory
	logDir := "logs"
	os.MkdirAll(logDir, 0755)

	// Open log files
	if config.LogFile != "" {
		if file, err := os.OpenFile(filepath.Join(logDir, config.LogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			logger.logFile = file
			// Get current file size
			if stat, err := file.Stat(); err == nil {
				logger.logFileSize = stat.Size()
			}
		}
	}

	if config.LogFileScreen != "" {
		if file, err := os.OpenFile(filepath.Join(logDir, config.LogFileScreen), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			logger.screenFile = file
			// Get current file size
			if stat, err := file.Stat(); err == nil {
				logger.screenFileSize = stat.Size()
			}
		}
	}

	// Initialize rotation check time
	logger.lastRotationCheck = time.Now()

	return logger
}

// Info logs info message
func (l *Logger) Info(message string) {
	l.log("INFO", message)
}

// Warn logs warning message
func (l *Logger) Warn(message string) {
	l.log("WARN", message)
}

// Error logs error message
func (l *Logger) Error(message string) {
	l.log("ERROR", message)
}

// Debug logs debug message
func (l *Logger) Debug(message string) {
	l.log("DEBUG", message)
}

// log writes log message
func (l *Logger) log(level, message string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now()
	timestamp := now.Format("02-01-2006 15:04:05")
	logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, level, message)
	logLineBytes := int64(len(logLine))

	// Check rotation for main log file
	if l.logFile != nil && l.config.LogRotationEnabled {
		if l.shouldRotate(l.logFile, l.logFileSize, logLineBytes) {
			l.rotateLogFile("log")
		}
	}

	// Write to main log file
	if l.logFile != nil {
		l.logFile.WriteString(logLine)
		l.logFile.Sync()
		l.logFileSize += logLineBytes
	}

	// Check rotation for screen log file
	if l.screenFile != nil && l.config.LogRotationEnabled {
		if l.shouldRotate(l.screenFile, l.screenFileSize, logLineBytes) {
			l.rotateLogFile("screen")
		}
	}

	// Write to screen log file
	if l.screenFile != nil {
		l.screenFile.WriteString(logLine)
		l.screenFile.Sync()
		l.screenFileSize += logLineBytes
	}

	// Write to stdout for INFO level
	if level == "INFO" {
		fmt.Print(logLine)
	}

	// Add to ring buffer
	l.addToBuffer(LogEntry{
		Time:    now,
		Level:   level,
		Message: message,
	})
}

// addToBuffer adds log entry to ring buffer
func (l *Logger) addToBuffer(entry LogEntry) {
	l.bufferMutex.Lock()
	defer l.bufferMutex.Unlock()

	// Add to current position
	l.logBuffer[l.bufferPos] = entry
	l.bufferPos = (l.bufferPos + 1) % l.bufferSize
}

// shouldRotate checks if log file should be rotated
func (l *Logger) shouldRotate(file *os.File, currentSize int64, newLineSize int64) bool {
	if !l.config.LogRotationEnabled {
		return false
	}

	// Check size limit
	if l.config.LogRotationMaxSize > 0 {
		if currentSize+newLineSize > l.config.LogRotationMaxSize {
			return true
		}
	}

	// Check time-based rotation (once per day)
	if l.config.LogRotationMaxDays > 0 {
		now := time.Now()
		if now.Sub(l.lastRotationCheck) > 24*time.Hour {
			l.lastRotationCheck = now
			// Check file modification time
			if stat, err := file.Stat(); err == nil {
				if now.Sub(stat.ModTime()) > time.Duration(l.config.LogRotationMaxDays)*24*time.Hour {
					return true
				}
			}
		}
	}

	return false
}

// rotateLogFile rotates a log file
func (l *Logger) rotateLogFile(logType string) {
	logDir := "logs"
	var file *os.File
	var fileName string

	if logType == "log" {
		file = l.logFile
		fileName = l.config.LogFile
	} else if logType == "screen" {
		file = l.screenFile
		fileName = l.config.LogFileScreen
	} else {
		return
	}

	if file == nil || fileName == "" {
		return
	}

	// Close current file
	file.Close()

	// Generate rotated filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	ext := filepath.Ext(fileName)
	rotatedName := fmt.Sprintf("%s_%s%s", baseName, timestamp, ext)
	rotatedPath := filepath.Join(logDir, rotatedName)
	originalPath := filepath.Join(logDir, fileName)

	// Rename current file
	if err := os.Rename(originalPath, rotatedPath); err != nil {
		// If rename fails, try to copy
		fmt.Printf("Warning: Failed to rotate log file %s: %v\n", fileName, err)
	}

	// Open new log file
	var err error
	if logType == "log" {
		if newFile, err := os.OpenFile(originalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			l.logFile = newFile
			l.logFileSize = 0
		}
	} else if logType == "screen" {
		if newFile, err := os.OpenFile(originalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			l.screenFile = newFile
			l.screenFileSize = 0
		}
	}

	if err != nil {
		fmt.Printf("Warning: Failed to open new log file %s: %v\n", fileName, err)
	}

	// Clean up old rotated files
	l.cleanupOldLogs(logDir, baseName, ext)
}

// cleanupOldLogs removes old rotated log files
func (l *Logger) cleanupOldLogs(logDir, baseName, ext string) {
	if l.config.LogRotationMaxFiles == 0 && l.config.LogRotationMaxDays == 0 {
		return // Keep all files
	}

	// Find all rotated files
	pattern := filepath.Join(logDir, baseName+"_*"+ext)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	// Sort by modification time (oldest first)
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	files := make([]fileInfo, 0, len(matches))
	for _, match := range matches {
		if stat, err := os.Stat(match); err == nil {
			files = append(files, fileInfo{
				path:    match,
				modTime: stat.ModTime(),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	now := time.Now()

	// Remove files exceeding max count
	if l.config.LogRotationMaxFiles > 0 && len(files) > l.config.LogRotationMaxFiles {
		toRemove := len(files) - l.config.LogRotationMaxFiles
		for i := 0; i < toRemove; i++ {
			os.Remove(files[i].path)
		}
		files = files[toRemove:]
	}

	// Remove files exceeding max age
	if l.config.LogRotationMaxDays > 0 {
		maxAge := time.Duration(l.config.LogRotationMaxDays) * 24 * time.Hour
		for _, file := range files {
			if now.Sub(file.modTime) > maxAge {
				os.Remove(file.path)
			}
		}
	}
}

// Close closes log files
func (l *Logger) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.logFile != nil {
		l.logFile.Close()
	}

	if l.screenFile != nil {
		l.screenFile.Close()
	}
}

// LogEntry represents a log entry
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// GetRecentLogs returns recent log entries from ring buffer
func (l *Logger) GetRecentLogs(level string, limit int) []LogEntry {
	l.bufferMutex.RLock()
	defer l.bufferMutex.RUnlock()

	if limit <= 0 || limit > 1000 {
		limit = 1000
	}

	// Collect all non-empty entries
	allEntries := make([]LogEntry, 0, l.bufferSize)
	for i := 0; i < l.bufferSize; i++ {
		idx := (l.bufferPos + i) % l.bufferSize
		entry := l.logBuffer[idx]
		// Check if entry is valid (not zero time)
		if !entry.Time.IsZero() {
			// Filter by level if specified
			if level == "" || entry.Level == level {
				allEntries = append(allEntries, entry)
			}
		}
	}

	// Reverse to get most recent first
	for i, j := 0, len(allEntries)-1; i < j; i, j = i+1, j-1 {
		allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
	}

	// Limit results
	if len(allEntries) > limit {
		allEntries = allEntries[:limit]
	}

	return allEntries
}

// LogEvent logs terminal event
func (l *Logger) LogEvent(event map[string]interface{}) {
	// This will be implemented for CSV logging
	l.Debug(fmt.Sprintf("Terminal event: %+v", event))
}

// LogPacket logs packet data
func (l *Logger) LogPacket(direction string, key string, data []byte, parsed interface{}) {
	if l.config.LogFileLow != "" {
		logLine := fmt.Sprintf("%s [%s:%s]:\n%s\nData: %+v\n\n",
			direction, key, time.Now().Format("15:04:05.000"), utils.NDs(data, " ", "0x"), parsed)

		// Write to low-level log file
		logDir := "logs"
		os.MkdirAll(logDir, 0755)

		if file, err := os.OpenFile(filepath.Join(logDir, l.config.LogFileLow), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			defer file.Close()
			file.WriteString(logLine)
			file.Sync()
		}
	}
}
