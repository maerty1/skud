package daemon

import (
	"encoding/json"
	"fmt"
	"nd-go/internal/cardlist"
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

// handleAPICardList handles card list management API
// GET    /api/cardlist          - list all cards (gmclist + mclist)
// GET    /api/cardlist/global   - list gmclist only
// GET    /api/cardlist/secondary - list mclist only
// POST   /api/cardlist/global/add    - add to gmclist:   body: [{"uid":"...", "message":"..."}]
// POST   /api/cardlist/global/del    - remove from gmclist: body: ["uid1", "uid2"]
// POST   /api/cardlist/global/sync   - sync gmclist:     body: [{"uid":"...", "message":"..."}]
// POST   /api/cardlist/secondary/add - add to mclist
// POST   /api/cardlist/secondary/del - remove from mclist
func (d *Daemon) handleAPICardList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if d.cardList == nil {
		http.Error(w, `{"error":"card list not initialized"}`, http.StatusInternalServerError)
		return
	}

	path := r.URL.Path
	pathParts := strings.Split(strings.TrimPrefix(path, "/api/cardlist"), "/")
	// pathParts[0] = "" (empty), pathParts[1] = "global"/"secondary", etc.

	if r.Method == http.MethodGet {
		var result interface{}
		switch {
		case len(pathParts) >= 2 && pathParts[1] == "global":
			result = d.cardList.GetGlobalList()
		case len(pathParts) >= 2 && pathParts[1] == "secondary":
			result = d.cardList.GetSecondaryList()
		default:
			result = map[string]interface{}{
				"gmclist": d.cardList.GetGlobalList(),
				"mclist":  d.cardList.GetSecondaryList(),
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": result})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if len(pathParts) < 3 {
		http.Error(w, `{"error":"missing action (add/del/sync)"}`, http.StatusBadRequest)
		return
	}

	listType := pathParts[1] // "global" or "secondary"
	action := pathParts[2]   // "add", "del", "sync"

	switch action {
	case "add":
		var entries []cardListEntry
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
			return
		}
		clEntries := make([]cardlistEntryConvert, len(entries))
		for i, e := range entries {
			clEntries[i] = cardlistEntryConvert{UID: e.UID, Message: e.Message}
		}
		var added []string
		if listType == "global" {
			added = d.cardList.AddGlobal(toCardEntries(clEntries))
		} else {
			added = d.cardList.AddSecondary(toCardEntries(clEntries))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": added})

	case "del":
		var uids []string
		if err := json.NewDecoder(r.Body).Decode(&uids); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
			return
		}
		var removed []string
		if listType == "global" {
			removed = d.cardList.DelGlobal(uids)
		} else {
			removed = d.cardList.DelSecondary(uids)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": removed})

	case "sync":
		if listType != "global" {
			http.Error(w, `{"error":"sync only supported for global list"}`, http.StatusBadRequest)
			return
		}
		var entries []cardListEntry
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
			return
		}
		clEntries := make([]cardlistEntryConvert, len(entries))
		for i, e := range entries {
			clEntries[i] = cardlistEntryConvert{UID: e.UID, Message: e.Message}
		}
		result := d.cardList.SyncGlobal(toCardEntries(clEntries))
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": result})

	default:
		http.Error(w, `{"error":"unknown action"}`, http.StatusBadRequest)
	}
}

// cardListEntry is a JSON-friendly card entry for the API
type cardListEntry struct {
	UID     string `json:"uid"`
	Message string `json:"message"`
}

type cardlistEntryConvert struct {
	UID     string
	Message string
}

// handleAPITermLogs handles terminal logs API with pagination (hndl_tlogs equivalent)
// GET /api/tlogs                          - list all terminal keys with counts
// GET /api/tlogs/{key}                    - get all entries for terminal
// GET /api/tlogs/{key}/count              - get entry count
// GET /api/tlogs/{key}/page/{size}        - get page count for given size
// GET /api/tlogs/{key}/page/{size}/{page} - get specific page
// GET /api/tlogs/{key}/page_r/{size}/{page} - get specific page (reversed)
func (d *Daemon) handleAPITermLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if d.termLogs == nil {
		http.Error(w, `{"error":"term logs not initialized"}`, http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/tlogs")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		// List all keys
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": d.termLogs.GetAll()})
		return
	}

	termKey := parts[0]

	if len(parts) == 1 {
		// Get all entries
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": d.termLogs.Get(termKey)})
		return
	}

	cmd := parts[1]
	switch cmd {
	case "count":
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": d.termLogs.Count(termKey)})
	case "page", "page_r":
		reversed := cmd == "page_r"
		if len(parts) < 3 {
			http.Error(w, `{"error":"page size required"}`, http.StatusBadRequest)
			return
		}
		pageSize, err := strconv.Atoi(parts[2])
		if err != nil || pageSize < 1 {
			pageSize = 20
		}

		if len(parts) < 4 {
			// Return page count
			count := d.termLogs.Count(termKey)
			pages := 0
			if count > 0 {
				pages = (count-1)/pageSize + 1
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": pages})
			return
		}

		pageStr := parts[3]
		var pageNo int
		switch strings.ToLower(pageStr) {
		case "first":
			pageNo = 0
		case "last":
			count := d.termLogs.Count(termKey)
			if count > 0 {
				pageNo = (count - 1) / pageSize
			}
		default:
			pageNo, _ = strconv.Atoi(pageStr)
		}

		entries, _, _ := d.termLogs.GetPage(termKey, pageSize, pageNo, reversed)
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": entries})
	default:
		http.Error(w, `{"error":"unknown command"}`, http.StatusBadRequest)
	}
}

// handleAPISettings handles runtime settings API (hndl_settings equivalent)
// GET  /api/system/settings              - get all settings
// GET  /api/system/settings/{key}        - get specific setting
// POST /api/system/settings/{key}        - set specific setting (body: {"value": ...})
func (d *Daemon) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	path := strings.TrimPrefix(r.URL.Path, "/api/system/settings")
	path = strings.Trim(path, "/")

	// GET: return settings
	if r.Method == http.MethodGet {
		if path == "" {
			// Return all editable settings
			settings := map[string]interface{}{
				"term_list_check_time": d.config.TermListCheckTime,
				"log_event_count":     d.config.LogEventCount,
				"log_dev_event_count": d.config.LogDevEventCount,
				"cam_service_active":  d.config.CamServiceActive,
				"crt_service_active":  d.config.CRTServiceActive,
				"crt_check_time":      d.config.CRTCheckTime,
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": settings})
			return
		}

		// Return specific setting
		val := d.getSettingValue(path)
		if val == nil {
			http.Error(w, `{"code":505,"error":"setting not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": val})
		return
	}

	// POST: set setting
	if r.Method == http.MethodPost {
		if path == "" {
			http.Error(w, `{"error":"setting key required"}`, http.StatusBadRequest)
			return
		}

		var body struct {
			Value interface{} `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
			return
		}

		ok := d.setSettingValue(path, body.Value)
		if !ok {
			http.Error(w, `{"code":505,"error":"setting not found or invalid value"}`, http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": true})
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

// getSettingValue returns a runtime setting value by key
func (d *Daemon) getSettingValue(key string) interface{} {
	switch key {
	case "term_list_check_time":
		return d.config.TermListCheckTime
	case "log_event_count":
		return d.config.LogEventCount
	case "log_dev_event_count":
		return d.config.LogDevEventCount
	case "cam_service_active":
		return d.config.CamServiceActive
	case "crt_service_active":
		return d.config.CRTServiceActive
	case "crt_check_time":
		return d.config.CRTCheckTime
	default:
		return nil
	}
}

// setSettingValue sets a runtime setting value by key
func (d *Daemon) setSettingValue(key string, value interface{}) bool {
	switch key {
	case "term_list_check_time":
		if v, ok := toFloat64(value); ok && v >= 0 && v <= 86400 {
			d.config.TermListCheckTime = v
			return true
		}
	case "log_event_count":
		if v, ok := toInt(value); ok && v >= 0 && v <= 50000 {
			d.config.LogEventCount = v
			return true
		}
	case "log_dev_event_count":
		if v, ok := toInt(value); ok && v >= 0 && v <= 50000 {
			d.config.LogDevEventCount = v
			return true
		}
	case "cam_service_active":
		if v, ok := toBool(value); ok {
			d.config.CamServiceActive = v
			return true
		}
	case "crt_service_active":
		if v, ok := toBool(value); ok {
			d.config.CRTServiceActive = v
			return true
		}
	case "crt_check_time":
		if v, ok := toFloat64(value); ok && v >= 0 {
			d.config.CRTCheckTime = v
			return true
		}
	}
	return false
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case int64:
		return int(val), true
	}
	return 0, false
}

func toBool(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case float64:
		return val != 0, true
	case int:
		return val != 0, true
	}
	return false, false
}

func toCardEntries(entries []cardlistEntryConvert) []cardlist.CardEntry {
	result := make([]cardlist.CardEntry, len(entries))
	for i, e := range entries {
		result[i] = cardlist.CardEntry{UID: e.UID, Message: e.Message}
	}
	return result
}

// handleAPIHalt handles shutdown request (ecmdh_halt equivalent)
// POST /api/system/halt
func (d *Daemon) handleAPIHalt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": "shutting down"})

	// Initiate shutdown in background
	go func() {
		time.Sleep(500 * time.Millisecond)
		d.Stop()
	}()
}

// terminalAddRequest represents a terminal add request
type terminalAddRequest struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Role     string `json:"role"`
	RegQuery bool   `json:"reg_query"`
}

// handleAPITerminalsAdd adds terminals (ecmdh_termlist add equivalent)
// POST /api/terminals/add
// Body: [{"ip":"1.2.3.4", "port":9000, "id":"T001", "type":"pocket"}]
func (d *Daemon) handleAPITerminalsAdd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var terminals []terminalAddRequest
	if err := json.NewDecoder(r.Body).Decode(&terminals); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}

	var added []map[string]interface{}
	for _, t := range terminals {
		if t.IP == "" || t.Port <= 0 {
			continue
		}

		// Start client connection
		var errCode int
		var errStr string
		key, err := d.pool.StartClient(t.IP, t.Port, d.config.TerminalConnectTimeout, &errCode, &errStr)
		if err != nil {
			d.logger.Warn(fmt.Sprintf("Failed to add terminal %s:%d: %v", t.IP, t.Port, err))
			continue
		}

		// Set terminal settings
		termType := types.TTYPE_POCKET
		switch strings.ToLower(t.Type) {
		case "gat":
			termType = types.TTYPE_GAT
		case "sphinx":
			termType = types.TTYPE_SPHINX
		case "jsp":
			termType = types.TTYPE_JSP
		}

		conn := d.pool.GetConnection(key)
		if conn != nil {
			if conn.Settings == nil {
				conn.Settings = &types.TerminalSettings{}
			}
			conn.Settings.Type = termType
			conn.Settings.ID = t.ID
			conn.Settings.CTRole = t.Role
			conn.Settings.RegQuery = t.RegQuery
		}

		added = append(added, map[string]interface{}{
			"key":  key,
			"ip":   t.IP,
			"port": t.Port,
			"id":   t.ID,
			"type": t.Type,
		})
	}

	if len(added) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 505, "error": "no valid terminals to add"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": added})
}

// handleAPITerminalsDel removes terminals (ecmdh_termlist del equivalent)
// POST /api/terminals/del
// Body: ["key1", "key2"] or ["ip:port", "ip:port"]
func (d *Daemon) handleAPITerminalsDel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var keys []string
	if err := json.NewDecoder(r.Body).Decode(&keys); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}

	var removed []string
	for _, key := range keys {
		// Try to find by ip:port if key contains ':' or '.'
		if strings.Contains(key, ":") || strings.Contains(key, ".") {
			// Find connection key by address
			connections := d.pool.GetConnections()
			for connKey, conn := range connections {
				addr := fmt.Sprintf("%s:%d", conn.Addr, conn.Port)
				if addr == key {
					key = connKey
					break
				}
			}
		}

		if d.pool.GetConnection(key) != nil {
			d.pool.DropConnection(key)
			removed = append(removed, key)
		}
	}

	if len(removed) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 505, "error": "no valid terminals to remove"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": removed})
}

// handleAPITerminalsCheck syncs terminal list (ecmdh_termlist check equivalent)
// POST /api/terminals/check
// Body: [{"ip":"1.2.3.4", "port":9000, "id":"T001", "type":"pocket"}]
func (d *Daemon) handleAPITerminalsCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var terminals []terminalAddRequest
	if err := json.NewDecoder(r.Body).Decode(&terminals); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}

	// Build incoming set
	incoming := make(map[string]terminalAddRequest)
	for _, t := range terminals {
		if t.IP != "" && t.Port > 0 {
			key := fmt.Sprintf("%s:%d", t.IP, t.Port)
			incoming[key] = t
		}
	}

	result := map[string]interface{}{
		"add": []interface{}{},
		"del": []interface{}{},
		"upd": []interface{}{},
	}

	// Find terminals to delete (exist in pool but not in incoming)
	connections := d.pool.GetConnections()
	var toDel []string
	for connKey, conn := range connections {
		addr := fmt.Sprintf("%s:%d", conn.Addr, conn.Port)
		if _, exists := incoming[addr]; !exists {
			toDel = append(toDel, connKey)
		}
	}
	for _, key := range toDel {
		d.pool.DropConnection(key)
	}
	if len(toDel) > 0 {
		result["del"] = toDel
	}

	// Find terminals to add (in incoming but not in pool)
	var toAdd []terminalAddRequest
	for addr, t := range incoming {
		found := false
		for _, conn := range connections {
			connAddr := fmt.Sprintf("%s:%d", conn.Addr, conn.Port)
			if connAddr == addr {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, t)
		}
	}

	// Add missing terminals
	var addedResults []map[string]interface{}
	for _, t := range toAdd {
		var errCode int
		var errStr string
		key, err := d.pool.StartClient(t.IP, t.Port, d.config.TerminalConnectTimeout, &errCode, &errStr)
		if err != nil {
			continue
		}
		termType := types.TTYPE_POCKET
		switch strings.ToLower(t.Type) {
		case "gat":
			termType = types.TTYPE_GAT
		case "sphinx":
			termType = types.TTYPE_SPHINX
		case "jsp":
			termType = types.TTYPE_JSP
		}
		conn := d.pool.GetConnection(key)
		if conn != nil && conn.Settings == nil {
			conn.Settings = &types.TerminalSettings{}
		}
		if conn != nil && conn.Settings != nil {
			conn.Settings.Type = termType
			conn.Settings.ID = t.ID
			conn.Settings.CTRole = t.Role
			conn.Settings.RegQuery = t.RegQuery
		}
		addedResults = append(addedResults, map[string]interface{}{"key": key, "ip": t.IP, "port": t.Port, "id": t.ID})
	}
	if len(addedResults) > 0 {
		result["add"] = addedResults
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"code": 200, "data": result})
}
