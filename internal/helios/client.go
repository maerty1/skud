package helios

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"nd-go/pkg/types"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// HeliosEventType represents Helios event type
type HeliosEventType string

const (
	HELIOS_EVENT_YES  HeliosEventType = "YES"  // Person verified
	HELIOS_EVENT_NO   HeliosEventType = "NO"   // Person not recognized
	HELIOS_EVENT_NF   HeliosEventType = "NF"   // Person not found
	HELIOS_EVENT_COR  HeliosEventType = "COR"  // Correlation update
	HELIOS_EVENT_FAIL HeliosEventType = "FAIL" // Request failed
)

// HeliosRequest represents a Helios verification request
type HeliosRequest struct {
	ID          string
	SessionID   string
	CamPID      string
	PersonID    string
	Conn        *websocket.Conn
	Processed   bool
	Completed   bool
	StartTime   time.Time
	CloseSent   bool
	CloseReason string
	Data        map[string]interface{}
	mutex       sync.RWMutex
}

// HeliosClient manages Helios WebSocket connections
type HeliosClient struct {
	config        *types.Config
	requests      map[string]*HeliosRequest
	mutex         sync.RWMutex
	httpClient    *http.Client
	eventCallback HeliosEventCallback
}

// HeliosEventCallback is called when Helios event is received
type HeliosEventCallback func(request *HeliosRequest, eventType HeliosEventType, data map[string]interface{})

var eventCallback HeliosEventCallback

// NewHeliosClient creates new Helios client
func NewHeliosClient(config *types.Config) *HeliosClient {
	return &HeliosClient{
		config: config,
		requests: make(map[string]*HeliosRequest),
		httpClient: &http.Client{
			Timeout: time.Duration(config.ServiceRequestExpireTime * float64(time.Second)),
		},
	}
}

// SetEventCallback sets callback for Helios events
func (hc *HeliosClient) SetEventCallback(callback HeliosEventCallback) {
	hc.eventCallback = callback
}

// generateSecKey generates WebSocket security key
func generateSecKey() string {
	key := make([]byte, 16)
	for i := range key {
		key[i] = byte(time.Now().UnixNano() % 256)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// generateAccept generates WebSocket accept key from sec key
func generateAccept(secKey string) string {
	const magicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(secKey + magicString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// StartVerification starts Helios verification request
func (hc *HeliosClient) StartVerification(sessionID string, camPID string, personID string) (string, error) {
	if !hc.config.CamServiceActive {
		return "", fmt.Errorf("camera service not active")
	}

	// Build WebSocket URL
	url := fmt.Sprintf("ws://%s:%d/api/cameras/%s/verify?person_id=%s&subscribe=&max_mps=10&detect_face=none&correlation_face=none",
		hc.config.CamServiceIP, hc.config.CamServicePort, camPID, personID)

	// Generate WebSocket handshake headers
	secKey := generateSecKey()
	secAccept := generateAccept(secKey)

	headers := http.Header{}
	headers.Set("Origin", "pkdaemon")
	headers.Set("Sec-WebSocket-Key", secKey)
	headers.Set("Sec-WebSocket-Protocol", "verification")
	headers.Set("Sec-WebSocket-Version", "13")

	// Add extra headers from config
	if hc.config.CamServiceRequestExtraHeaders != nil && len(hc.config.CamServiceRequestExtraHeaders) > 0 {
		for k, v := range hc.config.CamServiceRequestExtraHeaders {
			headers.Set(k, v)
		}
	}

	// Create dialer
	dialer := websocket.Dialer{
		HandshakeTimeout: time.Duration(hc.config.ServiceRequestExpireTime * float64(time.Second)),
	}

	// Connect to WebSocket
	conn, resp, err := dialer.Dial(url, headers)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Helios: %v", err)
	}
	defer resp.Body.Close()

	// Verify accept header
	acceptHeader := resp.Header.Get("Sec-WebSocket-Accept")
	if acceptHeader != secAccept {
		conn.Close()
		return "", fmt.Errorf("invalid WebSocket accept header")
	}

	// Create request
	requestID := fmt.Sprintf("helios_%d_%s", time.Now().UnixNano(), sessionID)
	request := &HeliosRequest{
		ID:        requestID,
		SessionID: sessionID,
		CamPID:    camPID,
		PersonID:  personID,
		Conn:      conn,
		StartTime: time.Now(),
		Data:      make(map[string]interface{}),
	}

	hc.mutex.Lock()
	hc.requests[requestID] = request
	hc.mutex.Unlock()

	// Start reading messages in goroutine
	go hc.readMessages(request)

	return requestID, nil
}

// readMessages reads messages from WebSocket connection
func (hc *HeliosClient) readMessages(request *HeliosRequest) {
	defer func() {
		request.Conn.Close()
		hc.mutex.Lock()
		delete(hc.requests, request.ID)
		hc.mutex.Unlock()
	}()

	request.Conn.SetReadDeadline(time.Now().Add(time.Duration(hc.config.ServiceRequestExpireTime * float64(time.Second))))

	for {
		messageType, message, err := request.Conn.ReadMessage()
		if err != nil {
			if !request.Processed {
				hc.handleEvent(request, HELIOS_EVENT_FAIL, map[string]interface{}{
					"error": err.Error(),
				})
			}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			var jsonData map[string]interface{}
			if err := json.Unmarshal(message, &jsonData); err != nil {
				continue
			}

			// Handle different event types
			if verified, ok := jsonData["verified"].(bool); ok && verified {
				hc.handleEvent(request, HELIOS_EVENT_YES, jsonData)
				hc.closeConnection(request, 1000, "autoclose")
				return
			}

			if terminated, ok := jsonData["terminated"].(bool); ok && terminated {
				hc.closeConnection(request, 1000, "autoclose")
				return
			}

			if _, ok := jsonData["correlations"].(map[string]interface{}); ok {
				hc.handleEvent(request, HELIOS_EVENT_COR, jsonData)
			}

		case websocket.CloseMessage:
			closeCode, closeText := parseCloseMessage(message)
			if closeCode == 4002 && len(closeText) > 0 {
				hc.handleEvent(request, HELIOS_EVENT_NF, map[string]interface{}{
					"error": closeText,
				})
			}
			request.CloseReason = fmt.Sprintf("%d:%s", closeCode, closeText)
			return

		case websocket.PingMessage:
			request.Conn.WriteMessage(websocket.PongMessage, message)

		case websocket.PongMessage:
			// Update read deadline
			request.Conn.SetReadDeadline(time.Now().Add(time.Duration(hc.config.ServiceRequestExpireTime * float64(time.Second))))
		}
	}
}

// handleEvent handles Helios event
func (hc *HeliosClient) handleEvent(request *HeliosRequest, eventType HeliosEventType, data map[string]interface{}) {
	request.mutex.Lock()
	request.Processed = true
	request.Data = data
	request.mutex.Unlock()

	if hc.eventCallback != nil {
		hc.eventCallback(request, eventType, data)
	}
}

// closeConnection closes WebSocket connection
func (hc *HeliosClient) closeConnection(request *HeliosRequest, code int, reason string) {
	request.mutex.Lock()
	defer request.mutex.Unlock()

	if request.CloseSent {
		return
	}

	request.CloseSent = true
	closeMsg := websocket.FormatCloseMessage(code, reason)
	request.Conn.WriteMessage(websocket.CloseMessage, closeMsg)
}

// parseCloseMessage parses WebSocket close message
func parseCloseMessage(message []byte) (int, string) {
	if len(message) < 2 {
		return 1000, ""
	}
	code := int(message[0])<<8 | int(message[1])
	text := ""
	if len(message) > 2 {
		text = string(message[2:])
	}
	return code, text
}

// GetRequest returns request by ID
func (hc *HeliosClient) GetRequest(requestID string) *HeliosRequest {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	return hc.requests[requestID]
}

// CloseRequest closes request connection
func (hc *HeliosClient) CloseRequest(requestID string) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	request, ok := hc.requests[requestID]
	if !ok {
		return
	}

	hc.closeConnection(request, 1000, "manual close")
	delete(hc.requests, requestID)
}

