package crt

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"nd-go/pkg/types"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CRTEventCallback is called when CRT identifies a person
type CRTEventCallback func(terminalID string, personID string, fio string, camID string, score float64, data map[string]interface{})

// CRTClient manages Vizir video recognition integration
type CRTClient struct {
	config        *types.Config
	httpClient    *http.Client
	mutex         sync.RWMutex
	fetchedTime   float64 // last fetched timestamp (Unix seconds with microseconds)
	camEvents     map[string]map[string]interface{}            // camera_id -> last event
	camSeen       map[string]map[string]map[string]interface{} // camera_id -> person_id -> data
	personSeen    map[string]map[string]map[string]interface{} // person_id -> camera_id -> data
	personCamBan  map[string]float64                           // "cam_id_pid" -> ban expiry time
	sessRequests  map[string]*SessionRequest                   // "cam_id_pid" -> session request
	eventCallback CRTEventCallback
	initialized   bool

	// idle timers
	seenIdleNextTime float64
	banIdleNextTime  float64
}

// SessionRequest represents a pending CRT session request (verification mode)
type SessionRequest struct {
	RID       string
	SessionID string
	Time      float64
	Timeout   float64
	CamID     string
	PID       string
}

// NewCRTClient creates new CRT client
func NewCRTClient(config *types.Config) *CRTClient {
	timeout := config.CRTServiceExpireTime
	if timeout <= 0 {
		timeout = 5.0
	}

	return &CRTClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout * float64(time.Second)),
		},
		camEvents:    make(map[string]map[string]interface{}),
		camSeen:      make(map[string]map[string]map[string]interface{}),
		personSeen:   make(map[string]map[string]map[string]interface{}),
		personCamBan: make(map[string]float64),
		sessRequests: make(map[string]*SessionRequest),
		initialized:  false,
	}
}

// SetEventCallback sets callback for person identification events
func (c *CRTClient) SetEventCallback(cb CRTEventCallback) {
	c.eventCallback = cb
}

// Init initializes CRT module
func (c *CRTClient) Init() bool {
	if !c.config.CRTServiceActive {
		return false
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.fetchedTime = 0.0
	c.camEvents = make(map[string]map[string]interface{})
	c.camSeen = make(map[string]map[string]map[string]interface{})
	c.personSeen = make(map[string]map[string]map[string]interface{})
	c.personCamBan = make(map[string]float64)
	c.sessRequests = make(map[string]*SessionRequest)
	c.initialized = true
	return true
}

// IdleProc performs periodic CRT processing (called from daemon main loop)
func (c *CRTClient) IdleProc() {
	if !c.config.CRTServiceActive || !c.initialized {
		return
	}

	mtf := float64(time.Now().UnixMicro()) / 1e6

	c.idleSeen(mtf)
	c.idleBan(mtf)
	c.idleSessionRequests(mtf)

	// Poll Vizir for new camera events
	cct := c.config.CRTCheckTime
	if cct > 0.0 {
		lastCheck := float64(c.config.CRTLastCheck.UnixMicro()) / 1e6
		if lastCheck < mtf-cct {
			c.pollCameraEvents()
			c.config.CRTLastCheck = time.Now()
		}
	}
}

// AddSessionRequest adds a verification-mode session request (cam waits for person to appear)
func (c *CRTClient) AddSessionRequest(sessionID string, camID string, pid string) string {
	camID = strings.TrimSpace(camID)
	pid = strings.TrimSpace(pid)
	if camID == "" || pid == "" {
		return ""
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	rid := camID + "_" + pid
	stmo := c.config.CRTSeenTimeout
	if stmo <= 0 {
		return ""
	}

	mtf := float64(time.Now().UnixMicro()) / 1e6
	c.sessRequests[rid] = &SessionRequest{
		RID:       rid,
		SessionID: sessionID,
		Time:      mtf,
		Timeout:   mtf + stmo,
		CamID:     camID,
		PID:       pid,
	}

	// Check if person is already seen
	if c.trySessionRequest(camID, pid) {
		return "" // Already found, callback was fired
	}

	return rid
}

// trySessionRequest checks if person is already in cam_seen and fires callback
func (c *CRTClient) trySessionRequest(camID string, pid string) bool {
	rid := camID + "_" + pid
	req, ok := c.sessRequests[rid]
	if !ok {
		return false
	}

	camSeenData, ok := c.camSeen[camID]
	if !ok {
		return false
	}
	data, ok := camSeenData[pid]
	if !ok {
		return false
	}

	// Person found -- fire callback
	if c.eventCallback != nil {
		termID, _ := data["term_id"].(string)
		fio, _ := data["fio"].(string)
		score, _ := data["score"].(float64)
		c.eventCallback(termID, pid, fio, camID, score, data)
	}

	_ = req
	delete(c.sessRequests, rid)
	return true
}

// Cam2Term maps camera ID to terminal ID
func (c *CRTClient) Cam2Term(camID string) string {
	if c.config.CRTCamLinks == nil {
		return ""
	}
	return c.config.CRTCamLinks[camID]
}

// TryBanAfterPass sets ban after person passed through gate
func (c *CRTClient) TryBanAfterPass(camID string, pid string) {
	if camID == "" || pid == "" {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	banTime := c.config.CRTBanCamPidTime
	if banTime > 0 && c.config.CRTBanPassOnly {
		cpid := camID + "_" + pid
		mtf := float64(time.Now().UnixMicro()) / 1e6
		c.personCamBan[cpid] = mtf + banTime
	}
}

// pollCameraEvents polls Vizir API for new camera events (stage 1)
func (c *CRTClient) pollCameraEvents() {
	c.mutex.RLock()
	ft := c.fetchedTime
	c.mutex.RUnlock()

	// Build URL
	baseURL := c.config.CRTServiceURL + "CameraEvent/GetItems"
	params := url.Values{}
	params.Set("criteria.matchDateSortType", "2")
	params.Set("criteria.ignoreMatchcesOnSameCameras", "true")
	params.Set("criteria.includeAllFaceCards", "true")

	take := 1
	if ft > 0.0 {
		take = 20
		ftStr := formatCRTTime(ft)
		if ftStr != "" {
			params.Set("criteria.matchDateFrom", ftStr)
		}
	}
	params.Set("criteria.take", fmt.Sprintf("%d", take))

	fullURL := baseURL + "?" + params.Encode()

	data, err := c.doRequest(fullURL, 1)
	if err != nil {
		fmt.Printf("CRT poll error: %v\n", err)
		return
	}

	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return
	}

	// Parse camera events
	events, maxDateStr := getCameraEvents(arr)
	maxDate := parseCRTTime(maxDateStr)
	if maxDate <= 0 {
		return
	}

	c.mutex.Lock()
	first := c.fetchedTime == 0.0
	c.fetchedTime = maxDate + 1.0
	c.mutex.Unlock()

	if first {
		return // First poll -- just establish baseline
	}

	// Process each new event
	for camIDStr, item := range events {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		fcid := getIntValue(itemMap, "FaceCardId")
		if fcid <= 0 {
			continue
		}

		// Check if this is a new event
		c.mutex.RLock()
		oldEvent := c.camEvents[camIDStr]
		c.mutex.RUnlock()

		oldFcid := 0
		if oldEvent != nil {
			oldFcid = getIntValue(oldEvent, "FaceCardId")
		}
		if fcid == oldFcid {
			continue
		}

		c.mutex.Lock()
		c.camEvents[camIDStr] = itemMap
		c.mutex.Unlock()

		termID := c.Cam2Term(camIDStr)
		if termID == "" {
			continue
		}

		camIDVal := getStringValue(itemMap, "CameraId")

		// Stage 2: Get match details
		go c.fetchMatchDetails(fcid, camIDVal, camIDStr, termID, itemMap)
	}
}

// fetchMatchDetails fetches match details for a face card (stage 2)
func (c *CRTClient) fetchMatchDetails(fcid int, camID string, camIDStr string, termID string, event map[string]interface{}) {
	matchURL := fmt.Sprintf("%sMatchDetailMessage/GetItems?criteria.parentFaceCardId=%d&criteria.orderType=1&criteria.take=1",
		c.config.CRTServiceURL, fcid)

	data, err := c.doRequest(matchURL, 2)
	if err != nil {
		fmt.Printf("CRT match detail error: %v\n", err)
		return
	}

	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return
	}

	// Find best match
	msg := getMatchDetailMessage(arr)
	if msg == nil {
		return
	}

	cfcid := getIntValue(msg, "ChildFaceCardId")
	if cfcid <= 0 {
		return
	}

	score := getFloatValue(msg, "Score")

	// Stage 3: Get person card
	c.fetchPersonCard(cfcid, camID, camIDStr, termID, score, event, msg)
}

// fetchPersonCard fetches person card details (stage 3)
func (c *CRTClient) fetchPersonCard(cfcid int, camID string, camIDStr string, termID string, score float64, event map[string]interface{}, match map[string]interface{}) {
	personURL := fmt.Sprintf("%sPersonCard/GetItems?criteria.faceCardId=%d&criteria.includePersonCardPropertyValues=true&criteria.take=1",
		c.config.CRTServiceURL, cfcid)

	data, err := c.doRequest(personURL, 3)
	if err != nil {
		fmt.Printf("CRT person card error: %v\n", err)
		return
	}

	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return
	}

	// Extract person data
	pdata := getPersonCardData(arr)
	if pdata == nil {
		return
	}

	pid, _ := pdata["pid"].(string)
	fio, _ := pdata["fio"].(string)

	if pid == "" {
		return
	}

	// Process identification
	c.processIdentification(camID, camIDStr, termID, pid, fio, score, pdata)
}

// processIdentification processes identified person (like crt_process_identification in PHP)
func (c *CRTClient) processIdentification(camID string, camIDStr string, termID string, pid string, fio string, score float64, pdata map[string]interface{}) {
	mtf := float64(time.Now().UnixMicro()) / 1e6

	c.mutex.Lock()
	// Store in cam_seen and person_seen
	if c.camSeen[camIDStr] == nil {
		c.camSeen[camIDStr] = make(map[string]map[string]interface{})
	}

	identData := map[string]interface{}{
		"cam_id":  camID,
		"pid":     pid,
		"fio":     fio,
		"term_id": termID,
		"score":   score,
		"pdata":   pdata,
		"mtf":     mtf,
	}
	c.camSeen[camIDStr][pid] = identData

	if c.personSeen[pid] == nil {
		c.personSeen[pid] = make(map[string]map[string]interface{})
	}
	c.personSeen[pid][camIDStr] = identData

	// Verification mode: check if there's a pending session request
	if !c.config.CRTServiceIdentificationMode {
		c.trySessionRequest(camIDStr, pid)
		c.mutex.Unlock()
		return
	}

	// Identification mode: check ban
	cpid := camIDStr + "_" + pid
	banTmo := c.personCamBan[cpid]

	if banTmo > 0 && mtf < banTmo {
		c.mutex.Unlock()
		return // Person is banned (recently identified)
	}

	// Set ban if not ban_pass_only
	banTime := c.config.CRTBanCamPidTime
	if banTime > 0 && !c.config.CRTBanPassOnly {
		banStart := mtf
		if c.config.CRTBanFromCatch {
			if catchMtf, ok := identData["mtf"].(float64); ok {
				banStart = catchMtf
			}
		}
		c.personCamBan[cpid] = banStart + banTime
	}
	c.mutex.Unlock()

	// Fire callback to daemon
	if c.eventCallback != nil {
		c.eventCallback(termID, pid, fio, camIDStr, score, identData)
	}
}

// doRequest performs HTTP request to Vizir API
func (c *CRTClient) doRequest(path string, stage int) (interface{}, error) {
	fullURL := fmt.Sprintf("http://%s:%d%s", c.config.CRTServiceIP, c.config.CRTServicePort, path)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set host header
	if c.config.CRTServiceName != "" {
		req.Host = c.config.CRTServiceName
	}

	// Add extra headers
	for _, hdr := range c.config.CRTServiceExtraHeaders {
		parts := strings.SplitN(hdr, ":", 2)
		if len(parts) == 2 {
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	req.Header.Set("Accept", "application/json")

	// Set timeout based on stage
	client := c.httpClient
	switch stage {
	case 1:
		if c.config.CRTServiceConnectTime1 > 0 {
			client = &http.Client{Timeout: time.Duration(c.config.CRTServiceConnectTime1 * float64(time.Second))}
		}
	case 2:
		if c.config.CRTServiceConnectTime2 > 0 {
			client = &http.Client{Timeout: time.Duration(c.config.CRTServiceConnectTime2 * float64(time.Second))}
		}
	case 3:
		if c.config.CRTServiceConnectTime3 > 0 {
			client = &http.Client{Timeout: time.Duration(c.config.CRTServiceConnectTime3 * float64(time.Second))}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	return result, nil
}

// idleSeen cleans up expired seen entries
func (c *CRTClient) idleSeen(mtf float64) {
	stmo := c.config.CRTSeenTimeout
	if stmo <= 0 {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.seenIdleNextTime > mtf {
		return
	}

	threshold := mtf - stmo

	for camID, persons := range c.camSeen {
		for pid, data := range persons {
			if imtf, ok := data["mtf"].(float64); ok && imtf < threshold {
				delete(persons, pid)
			}
		}
		if len(persons) == 0 {
			delete(c.camSeen, camID)
		}
	}

	for pid, cameras := range c.personSeen {
		for camID, data := range cameras {
			if imtf, ok := data["mtf"].(float64); ok && imtf < threshold {
				delete(cameras, camID)
			}
		}
		if len(cameras) == 0 {
			delete(c.personSeen, pid)
		}
	}

	c.seenIdleNextTime = mtf + 10.0
}

// idleBan cleans up expired bans
func (c *CRTClient) idleBan(mtf float64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.banIdleNextTime > mtf {
		return
	}

	for banID, expiry := range c.personCamBan {
		if expiry < mtf {
			delete(c.personCamBan, banID)
		}
	}

	c.banIdleNextTime = mtf + 10.0
}

// idleSessionRequests cleans up expired session requests
func (c *CRTClient) idleSessionRequests(mtf float64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for rid, req := range c.sessRequests {
		if req.Timeout < mtf {
			delete(c.sessRequests, rid)
		}
	}
}

// --- Helper functions ---

// formatCRTTime formats Unix timestamp to CRT API time format (ISO 8601 with microseconds)
func formatCRTTime(mtf float64) string {
	sec := int64(mtf)
	usec := int64(math.Round((mtf - float64(sec)) * 1e7))
	t := time.Unix(sec, 0).UTC()
	return fmt.Sprintf("%s.%07dZ", t.Format("2006-01-02T15:04:05"), usec)
}

// parseCRTTime parses CRT API time string to Unix timestamp
func parseCRTTime(str string) float64 {
	if str == "" {
		return 0
	}
	// Format: 2019-04-25T13:59:46.9470000Z
	t, err := time.Parse("2006-01-02T15:04:05.9999999Z", str)
	if err != nil {
		// Try without fractional seconds
		t, err = time.Parse("2006-01-02T15:04:05Z", str)
		if err != nil {
			return 0
		}
	}
	return float64(t.UnixMicro()) / 1e6
}

// getCameraEvents extracts camera events from API response
func getCameraEvents(arr []interface{}) (map[string]interface{}, string) {
	maxDate := ""
	events := make(map[string]interface{})

	for _, item := range arr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if maxDate == "" {
			maxDate = getStringValueCI(itemMap, "DateCreated")
		}

		camID := strings.TrimSpace(getStringValueCI(itemMap, "CameraId"))
		if camID != "" {
			if _, exists := events[camID]; !exists {
				events[camID] = normalizeKeys(itemMap)
			}
		}
	}

	return events, maxDate
}

// getMatchDetailMessage finds best match from API response
func getMatchDetailMessage(arr []interface{}) map[string]interface{} {
	var bestMsg map[string]interface{}
	var bestScore float64

	for _, item := range arr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemMap = normalizeKeys(itemMap)

		cfcid := getIntValue(itemMap, "ChildFaceCardId")
		score := getFloatValue(itemMap, "Score")

		if cfcid <= 0 || score <= 0 || score > 1.0 {
			continue
		}

		if bestMsg == nil || score > bestScore {
			bestMsg = itemMap
			bestScore = score
		}
	}

	return bestMsg
}

// getPersonCardData extracts person data from PersonCard API response
func getPersonCardData(arr []interface{}) map[string]interface{} {
	for _, item := range arr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemMap = normalizeKeys(itemMap)

		// Must have PersonCardPropertyValues, AlternateId, Information
		if _, ok := itemMap["PersonCardPropertyValues"]; !ok {
			continue
		}
		if _, ok := itemMap["Information"]; !ok {
			continue
		}

		pid := strings.TrimSpace(getStringValue(itemMap, "Information"))
		if pid == "" {
			continue
		}

		fio := extractFIO(itemMap)
		header := getStringValue(itemMap, "Header")

		return map[string]interface{}{
			"pid":    pid,
			"fio":    fio,
			"header": header,
		}
	}

	return nil
}

// extractFIO extracts FIO from PersonCardPropertyValues
func extractFIO(pcard map[string]interface{}) string {
	vals, ok := pcard["PersonCardPropertyValues"].([]interface{})
	if !ok {
		return ""
	}

	for _, v := range vals {
		valMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		valMap = normalizeKeys(valMap)

		// Check if PropertyTemplateEntry.Name == "_fio"
		pte, ok := valMap["PropertyTemplateEntry"].(map[string]interface{})
		if !ok {
			continue
		}
		pte = normalizeKeys(pte)

		name := getStringValue(pte, "Name")
		if name == "_fio" {
			value := getStringValue(valMap, "Value")
			if value != "" {
				return value
			}
		}
	}

	return ""
}

// normalizeKeys normalizes JSON keys to consistent casing
func normalizeKeys(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		// Store with original case AND upper case
		result[k] = v
		upper := strings.ToUpper(k)
		if upper != k {
			result[upper] = v
		}
	}
	return result
}

// getStringValue gets string value from map (case-sensitive)
func getStringValue(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getStringValueCI gets string value from map (case-insensitive)
func getStringValueCI(m map[string]interface{}, key string) string {
	// Try exact match
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	// Try upper case
	upper := strings.ToUpper(key)
	if v, ok := m[upper]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	// Try lower case
	lower := strings.ToLower(key)
	if v, ok := m[lower]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getIntValue gets int value from map (handles float64 from JSON)
func getIntValue(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case int64:
			return int(val)
		}
	}
	upper := strings.ToUpper(key)
	if v, ok := m[upper]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		}
	}
	return 0
}

// getFloatValue gets float64 value from map
func getFloatValue(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	upper := strings.ToUpper(key)
	if v, ok := m[upper]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

// FormatScore formats score as percentage string
func FormatScore(score float64) string {
	if score > 0 && score <= 1.0 {
		prc := int(math.Round(score * 100))
		return fmt.Sprintf("%d%%", prc)
	}
	return ""
}
