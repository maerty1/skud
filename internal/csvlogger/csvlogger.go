package csvlogger

import (
	"encoding/csv"
	"fmt"
	"nd-go/pkg/types"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CSVLogger handles CSV logging for sessions
type CSVLogger struct {
	logDir    string
	keys      []string
	delimiter rune
	mutex     sync.Mutex
}

// NewCSVLogger creates new CSV logger
func NewCSVLogger(logDir string) *CSVLogger {
	if logDir == "" {
		logDir = "./csv/"
	}

	// Ensure directory exists
	os.MkdirAll(logDir, 0777)

	return &CSVLogger{
		logDir:    logDir,
		delimiter: ';', // Semicolon delimiter as in ND
		keys: []string{
			"session_time",
			"term_id",
			"term_addr",
			"term_role",
			"uid",
			"kpo_result",
			"kpo_msg",
			"cam_result",
			"cam_cid",
			"final_result",
			"final_msg",
		},
	}
}

// LogSession logs session data to CSV file
func (cl *CSVLogger) LogSession(session *types.Session, conn *types.Connection) error {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()

	// Get CSV file name
	filename := cl.getCSVFilename("")
	if filename == "" {
		return fmt.Errorf("failed to get CSV filename")
	}

	// Prepare data row
	data := cl.prepareSessionData(session, conn)

	// Write to CSV file
	return cl.writeCSVRow(filename, data)
}

// getCSVFilename returns CSV filename for current date
// Format: dd_mm_yyyy.csv (e.g., 25_12_2024.csv)
func (cl *CSVLogger) getCSVFilename(suffix string) string {
	now := time.Now()
	dateStr := now.Format("02_01_2006") // dd_mm_yyyy format
	
	filename := dateStr
	if suffix != "" {
		filename += suffix
	}
	filename += ".csv"
	
	return filepath.Join(cl.logDir, filename)
}

// prepareSessionData prepares session data for CSV logging
func (cl *CSVLogger) prepareSessionData(session *types.Session, conn *types.Connection) map[string]string {
	data := make(map[string]string)

	// session_time - format: dd.mm.yy HH:MM:SS
	if !session.ReqTime.IsZero() {
		data["session_time"] = session.ReqTime.Format("02.01.06 15:04:05")
	} else {
		data["session_time"] = time.Now().Format("02.01.06 15:04:05")
	}

	// term_id
	if conn != nil && conn.Settings != nil {
		data["term_id"] = conn.Settings.ID
	} else {
		data["term_id"] = ""
	}

	// term_addr - terminal address (IP:Port or connection key)
	if conn != nil {
		if conn.Settings != nil {
			data["term_addr"] = fmt.Sprintf("%s:%d", conn.Settings.IP, conn.Settings.Port)
		} else {
			data["term_addr"] = conn.Key
		}
	} else {
		data["term_addr"] = ""
	}

	// term_role
	if conn != nil && conn.Settings != nil {
		// Extract role from settings if available
		if role, ok := conn.Settings.Extra["role"].(string); ok {
			data["term_role"] = role
		} else {
			data["term_role"] = ""
		}
	} else {
		data["term_role"] = ""
	}

	// uid
	data["uid"] = session.UID

	// kpo_result
	kpoResult := "UNDEF"
	if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
		if result, ok := kpoData["result"].(types.KPOResult); ok {
			kpoResult = kpoResultToString(result)
		}
	}
	data["kpo_result"] = kpoResult

	// kpo_msg
	kpoMsg := ""
	if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
		if msg, ok := kpoData["message"].(string); ok {
			kpoMsg = msg
		}
	}
	data["kpo_msg"] = nl2comma(kpoMsg)

	// cam_result
	camResult := "UNDEF"
	if camData, ok := session.Data["cam"].(map[string]interface{}); ok {
		if result, ok := camData["result"].(types.CamResult); ok {
			camResult = camResultToString(result)
		}
	}
	data["cam_result"] = camResult

	// cam_cid
	data["cam_cid"] = session.CID

	// final_result
	finalResult := "NO"
	if result, ok := session.Data["result"].(int); ok {
		if result > 0 {
			finalResult = "YES"
		}
	}
	data["final_result"] = finalResult

	// final_msg
	finalMsg := ""
	if msg, ok := session.Data["message"].(string); ok {
		finalMsg = msg
	}
	data["final_msg"] = nl2comma(finalMsg)

	return data
}

// writeCSVRow writes a row to CSV file
func (cl *CSVLogger) writeCSVRow(filename string, data map[string]string) error {
	// Check if file exists, if not create with header
	fileExists := fileExists(filename)
	
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = cl.delimiter

	// Write header if file is new
	if !fileExists {
		header := make([]string, len(cl.keys))
		for i, key := range cl.keys {
			header[i] = key
		}
		if err := writer.Write(header); err != nil {
			return fmt.Errorf("failed to write CSV header: %v", err)
		}
	}

	// Write data row
	row := make([]string, len(cl.keys))
	for i, key := range cl.keys {
		row[i] = data[key]
	}
	if err := writer.Write(row); err != nil {
		return fmt.Errorf("failed to write CSV row: %v", err)
	}

	writer.Flush()
	return writer.Error()
}

// Helper functions

// kpoResultToString converts KPOResult to string
func kpoResultToString(result types.KPOResult) string {
	switch result {
	case types.KPO_RES_YES:
		return "YES"
	case types.KPO_RES_NO:
		return "NO"
	case types.KPO_RES_FAIL:
		return "FAIL"
	default:
		return "UNDEF"
	}
}

// camResultToString converts CamResult to string
func camResultToString(result types.CamResult) string {
	switch result {
	case types.CAM_RES_YES:
		return "YES"
	case types.CAM_RES_NO:
		return "NO"
	case types.CAM_RES_NF:
		return "NF"
	case types.CAM_RES_FAIL:
		return "FAIL"
	default:
		return "UNDEF"
	}
}

// nl2comma replaces newlines with commas (as in ND)
func nl2comma(s string) string {
	s = strings.ReplaceAll(s, "\r\n", ";")
	s = strings.ReplaceAll(s, "\n", ";")
	s = strings.ReplaceAll(s, "\r", ";")
	return s
}

// fileExists checks if file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

