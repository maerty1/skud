package daemon

import (
	"encoding/json"
	"fmt"
	"nd-go/pkg/types"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// handleAPISessionDetail serves detailed session information
func (d *Daemon) handleAPISessionDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract session ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/session/")
	if path == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	session := d.sessionMgr.GetSession(path)
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Get connection info
	var conn *types.Connection
	if pool, ok := d.sessionMgr.GetPool().(interface {
		GetConnection(string) *types.Connection
	}); ok {
		conn = pool.GetConnection(session.Key)
	} else {
		conn = d.pool.GetConnection(session.Key)
	}

	result := map[string]interface{}{
		"id":          session.ID,
		"key":         session.Key,
		"apkey":       session.Apkey,
		"uid":         session.UID,
		"cid":         session.CID,
		"stage":       session.Stage.String(),
		"req_time":    session.ReqTime.Unix() * 1000,
		"processed":   session.Processed,
		"completed":   session.Completed,
		"report_sent": session.ReportSent,
		"data":        session.Data,
	}

	if conn != nil {
		result["connection"] = map[string]interface{}{
			"ip":        conn.IP,
			"port":      conn.Port,
			"type":      string(conn.Settings.Type),
			"connected": conn.Connected,
		}
	}

	json.NewEncoder(w).Encode(result)
}

// handleAPITerminals serves terminals list with management options
func (d *Daemon) handleAPITerminals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "GET" {
		// Get all terminals from termlist
		termlist := d.pool.GetTerminalList()
		json.NewEncoder(w).Encode(termlist)
	} else if r.Method == "POST" {
		// Connect to terminal
		var req struct {
			IP   string `json:"ip"`
			Port int    `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Try to connect
		err := d.pool.ConnectToTerminal(req.IP, req.Port)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "connected"})
	} else if r.Method == "DELETE" {
		// Disconnect terminal
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "Terminal key required", http.StatusBadRequest)
			return
		}

		d.pool.Disconnect(key)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "disconnected"})
	}
}

// handleAPITerminalDetail serves detailed terminal information
func (d *Daemon) handleAPITerminalDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract terminal key from path
	path := strings.TrimPrefix(r.URL.Path, "/api/terminal/")
	if path == "" {
		http.Error(w, "Terminal key required", http.StatusBadRequest)
		return
	}

	// Decode URL-encoded key
	key, err := url.QueryUnescape(path)
	if err != nil {
		key = path // Fallback to original if decoding fails
	}

	conn := d.pool.GetConnection(key)
	if conn == nil {
		// Try to find by searching all connections
		allConns := d.pool.GetConnections()
		found := false
		for k, c := range allConns {
			if k == key || c.Addr+":"+strconv.Itoa(c.Port) == key {
				conn = d.pool.GetConnection(k)
				found = true
				break
			}
		}
		if !found {
			http.Error(w, fmt.Sprintf("Terminal not found: %s", key), http.StatusNotFound)
			return
		}
	}

	if conn == nil {
		http.Error(w, "Terminal not found", http.StatusNotFound)
		return
	}

	result := map[string]interface{}{
		"key":           conn.Key,
		"ip":            conn.IP,
		"port":          conn.Port,
		"type":          "unknown",
		"connected":     conn.Connected,
		"start_time":    conn.StartTime.Unix() * 1000,
		"last_activity": conn.LastActivity.Unix() * 1000,
	}

	if conn.Settings != nil {
		result["type"] = string(conn.Settings.Type)
		result["settings"] = conn.Settings
		result["terminal_id"] = conn.Settings.ID
	}

	json.NewEncoder(w).Encode(result)
}

// handleAPILogs serves log entries
func (d *Daemon) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get log level and limit from query params
	level := r.URL.Query().Get("level")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	// Get logs from logger
	logs := d.logger.GetRecentLogs(level, limit)

	result := map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	}

	json.NewEncoder(w).Encode(result)
}

// handleAPIConfig serves configuration (read-only for security)
func (d *Daemon) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Return safe config (without sensitive data)
	safeConfig := map[string]interface{}{
		"server_addr":         d.config.ServerAddr,
		"server_port":         d.config.ServerPort,
		"web_addr":            d.config.WebAddr,
		"web_port":            d.config.WebPort,
		"http_service_active": d.config.HTTPServiceActive,
		"http_service_name":   d.config.HTTPServiceName,
		"cam_service_active":  d.config.CamServiceActive,
		"cam_service_ip":      d.config.CamServiceIP,
		"cam_service_port":    d.config.CamServicePort,
	}

	json.NewEncoder(w).Encode(safeConfig)
}

// handleAPIEvents serves Server-Sent Events for real-time updates
func (d *Daemon) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Send events
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-d.eventCh:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-ticker.C:
			// Send heartbeat
			fmt.Fprintf(w, ": heartbeat\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}
