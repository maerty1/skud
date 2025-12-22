package connection

import (
	"encoding/json"
	"fmt"
	"nd-go/internal/protocols/gat"
	"nd-go/internal/protocols/jsp"
	"nd-go/internal/protocols/pocket"
	"nd-go/internal/protocols/sphinx"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ConnectionPool manages TCP connections
type ConnectionPool struct {
	connections   map[string]*Connection
	reconnections map[string]*types.Reconnection
	listeners     map[string]*net.TCPListener
	mutex         sync.RWMutex
	config        *types.Config
	// Event handlers
	onTagRead   func(connKey, uid string, readerType uint8, auth bool)
	onPassEvent func(connKey string, passed bool)
}

// Connection represents a single connection
type Connection struct {
	Key           string
	Conn          net.Conn
	Addr          string
	Port          int
	Connected     bool
	LastActivity  time.Time
	StartTime     time.Time
	Settings      *types.TerminalSettings
	Buffer        []byte
	PendingData   []byte
	JSPConn       *jsp.JSPConnection // JSP connection state
	JSPRIDCounter int                // JSP Request ID counter
	PocketPing    *PocketPingState   // POCKET ping state
	GatPing       *GatPingState      // GAT ping state
	SphinxPing    *SphinxPingState   // SPHINX ping state
}

// PocketPingState represents POCKET ping state
type PocketPingState struct {
	PingInterval  int       // Interval in seconds
	PingTimeout   int       // Timeout in seconds
	PingSent      bool      // Whether ping was sent
	PingSinceLast int       // Seconds since last ping
	LastPingTime  time.Time // Time when ping was sent
}

// GatPingState represents GAT ping state
type GatPingState struct {
	PingInterval  int       // Interval in seconds
	PingTimeout   int       // Timeout in seconds
	PingSent      bool      // Whether ping was sent
	PingSinceLast int       // Seconds since last ping
	LastPingTime  time.Time // Time when ping was sent
	TerminalType  uint8     // Terminal type (0x01 for ACCESS, etc.)
}

// SphinxPingState represents SPHINX ping state
type SphinxPingState struct {
	PingInterval  int       // Interval in seconds
	PingTimeout   int       // Timeout in seconds
	PingSent      bool      // Whether ping was sent
	PingSinceLast int       // Seconds since last ping
	LastPingTime  time.Time // Time when ping was sent
}

// Note: Reconnection type is now defined in types.Reconnection

// NewConnectionPool creates new connection pool
func NewConnectionPool(config *types.Config) *ConnectionPool {
	return &ConnectionPool{
		connections:   make(map[string]*Connection),
		reconnections: make(map[string]*types.Reconnection),
		listeners:     make(map[string]*net.TCPListener),
		config:        config,
	}
}

// SetEventHandlers sets event handlers for connection events
func (cp *ConnectionPool) SetEventHandlers(onTagRead func(string, string, uint8, bool), onPassEvent func(string, bool)) {
	cp.onTagRead = onTagRead
	cp.onPassEvent = onPassEvent
}

// StartClient starts client connection
func (cp *ConnectionPool) StartClient(addr string, port int, timeout float64, errCode *int, errStr *string) (string, error) {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	key := fmt.Sprintf("%s:%d", addr, port)

	// Check if connection already exists
	if conn, exists := cp.connections[key]; exists && conn.Connected {
		return key, nil
	}

	// Create TCP connection
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		if errCode != nil {
			*errCode = 1
		}
		if errStr != nil {
			*errStr = err.Error()
		}
		return "", err
	}

	conn, err := net.DialTimeout("tcp", tcpAddr.String(), time.Duration(timeout*float64(time.Second)))
	if err != nil {
		if errCode != nil {
			*errCode = 2
		}
		if errStr != nil {
			*errStr = err.Error()
		}
		return "", err
	}

	connection := &Connection{
		Key:           key,
		Conn:          conn,
		Addr:          addr,
		Port:          port,
		Connected:     true,
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		Buffer:        make([]byte, 0),
		PendingData:   make([]byte, 0),
		JSPConn:       jsp.NewJSPConnection(),
		JSPRIDCounter: 0,
	}

	cp.connections[key] = connection

	// Start goroutine for reading
	go cp.handleConnection(connection)

	return key, nil
}

// StartServer starts TCP server
func (cp *ConnectionPool) StartServer(addr string, port int) error {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	key := fmt.Sprintf("%s:%d", addr, port)

	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	cp.listeners[key] = listener

	// Start accepting connections
	go cp.acceptConnections(listener)

	return nil
}

// acceptConnections accepts incoming connections
func (cp *ConnectionPool) acceptConnections(listener *net.TCPListener) {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			continue
		}

		// Create connection object
		addr := conn.RemoteAddr().(*net.TCPAddr)
		key := fmt.Sprintf("%s:%d", addr.IP.String(), addr.Port)

		connection := &Connection{
			Key:           key,
			Conn:          conn,
			Addr:          addr.IP.String(),
			Port:          addr.Port,
			Connected:     true,
			StartTime:     time.Now(),
			LastActivity:  time.Now(),
			Buffer:        make([]byte, 0),
			PendingData:   make([]byte, 0),
			JSPConn:       jsp.NewJSPConnection(),
			JSPRIDCounter: 0,
		}

		// Initialize JSP connection if it's JSP terminal
		if connection.Settings != nil && connection.Settings.Type == types.TTYPE_JSP {
			// Set auto-ping if enabled
			if cp.config.JSPDevAutoPingEnabled {
				connection.JSPConn.PingInterval = 10 // Default 10 seconds
				connection.JSPConn.PingTimeout = 15  // Default 15 seconds
			}
		}

		// Initialize POCKET ping state if it's POCKET terminal
		if connection.Settings != nil && connection.Settings.Type == types.TTYPE_POCKET {
			connection.PocketPing = &PocketPingState{
				PingInterval: 10, // Default 10 seconds
				PingTimeout:  15, // Default 15 seconds
				PingSent:     false,
			}
		}

		// Initialize GAT ping state if it's GAT terminal
		if connection.Settings != nil && connection.Settings.Type == types.TTYPE_GAT {
			connection.GatPing = &GatPingState{
				PingInterval: 10, // Default 10 seconds
				PingTimeout:  15, // Default 15 seconds
				PingSent:     false,
				TerminalType: 0x01, // Default ACCESS terminal type
			}
		}

		// Initialize SPHINX ping state if it's SPHINX terminal
		if connection.Settings != nil && connection.Settings.Type == types.TTYPE_SPHINX {
			connection.SphinxPing = &SphinxPingState{
				PingInterval: 5,  // Default 5 seconds (as in PHP)
				PingTimeout:  10, // Default 10 seconds (as in PHP)
				PingSent:     false,
			}
		}

		cp.mutex.Lock()
		cp.connections[key] = connection
		cp.mutex.Unlock()

		// Start goroutine for handling connection
		go cp.handleConnection(connection)
	}
}

// handleConnection handles individual connection
func (cp *ConnectionPool) handleConnection(conn *Connection) {
	buffer := make([]byte, 4096)

	for conn.Connected {
		conn.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Conn.Read(buffer)
		if err != nil {
			cp.disconnect(conn.Key, err.Error())
			break
		}

		if n > 0 {
			conn.LastActivity = time.Now()
			conn.Buffer = append(conn.Buffer, buffer[:n]...)

			// Process data
			cp.processData(conn)
		}
	}
}

// processData processes received data
func (cp *ConnectionPool) processData(conn *Connection) {
	// Process data based on terminal type
	if conn.Settings != nil {
		switch conn.Settings.Type {
		case types.TTYPE_POCKET:
			cp.processPocketData(conn)
		case types.TTYPE_GAT:
			cp.processGatData(conn)
		case types.TTYPE_SPHINX:
			cp.processSphinxData(conn)
		case types.TTYPE_JSP:
			cp.processJSPData(conn)
		default:
			fmt.Printf("Unknown terminal type for connection %s: %s\n", conn.Key, conn.Settings.Type)
		}
	} else {
		// Unknown connection type, try to detect from data
		cp.detectProtocol(conn)
	}
}

// Send sends data to connection
func (cp *ConnectionPool) Send(key string, data []byte) error {
	cp.mutex.RLock()
	conn, exists := cp.connections[key]
	cp.mutex.RUnlock()

	if !exists || !conn.Connected {
		return fmt.Errorf("connection not found or not connected: %s", key)
	}

	conn.LastActivity = time.Now()
	_, err := conn.Conn.Write(data)
	return err
}

// Disconnect disconnects connection (exported for web API)
func (cp *ConnectionPool) Disconnect(key string) {
	cp.disconnect(key, "manual disconnect")
}

// disconnect disconnects connection
func (cp *ConnectionPool) disconnect(key string, reason string) {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	if conn, exists := cp.connections[key]; exists {
		conn.Connected = false
		conn.Conn.Close()

		// Schedule reconnection
		reconn := &types.Reconnection{
			Key:    key,
			ConKey: key,
			IP:     conn.Addr,
			Port:   conn.Port,
			Time:   time.Now(),
			NTime:  time.Now().Add(cp.calculateReconnectionDelay(1)),
			Count:  1,
		}

		if conn.Settings != nil {
			reconn.Settings = conn.Settings
		}

		cp.reconnections[key] = reconn
		delete(cp.connections, key)
	}

	fmt.Printf("Disconnected %s: %s\n", key, reason)
}

// calculateReconnectionDelay calculates delay for reconnection
func (cp *ConnectionPool) calculateReconnectionDelay(count int) time.Duration {
	delay := float64(count) * cp.config.ReconnectionWaitTimeStep
	if delay > cp.config.ReconnectionWaitTimeMax {
		delay = cp.config.ReconnectionWaitTimeMax
	}
	if delay < 0.1 {
		delay = 0.1
	}
	return time.Duration(delay * float64(time.Second))
}

// LockTerminal locks a terminal for a session (shows waiting message)
func (cp *ConnectionPool) LockTerminal(key string, sessionID string, text string) error {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	conn, ok := cp.connections[key]
	if !ok || !conn.Connected {
		return fmt.Errorf("connection not found or not connected: %s", key)
	}

	// Set reader locked in Extra
	if conn.Settings == nil {
		conn.Settings = &types.TerminalSettings{}
	}
	if conn.Settings.Extra == nil {
		conn.Settings.Extra = make(map[string]interface{})
	}
	conn.Settings.Extra["reader_locked"] = sessionID

	// Send lock packet based on terminal type
	if conn.Settings.Type == types.TTYPE_POCKET {
		packet := pocket.CreateLockPacket(text)
		return cp.Send(key, packet)
	} else if conn.Settings.Type == types.TTYPE_JSP {
		// For JSP, send message with waiting using JSP protocol
		if text == "" {
			text = "Ожидание..."
		}
		// Use JSP protocol to send message
		if conn.JSPConn != nil {
			packet := jsp.CreateMessagePacket(text, 0) // 0 = wait until removed
			return cp.Send(key, packet)
		}
		return fmt.Errorf("JSP connection not initialized")
	}
	// For GAT/SPHINX, just mark as locked
	return nil
}

// UnlockTerminal unlocks a terminal (clears waiting message)
func (cp *ConnectionPool) UnlockTerminal(key string, sessionID string) error {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	conn, ok := cp.connections[key]
	if !ok || !conn.Connected {
		return fmt.Errorf("connection not found or not connected: %s", key)
	}

	// Check if this session locked the terminal
	if conn.Settings == nil || conn.Settings.Extra == nil {
		return fmt.Errorf("terminal not locked")
	}

	lockedSessionID, ok := conn.Settings.Extra["reader_locked"].(string)
	if !ok || lockedSessionID != sessionID {
		return fmt.Errorf("terminal not locked by session: %s", sessionID)
	}

	// Clear reader locked
	delete(conn.Settings.Extra, "reader_locked")

	// Send unlock packet based on terminal type
	if conn.Settings.Type == types.TTYPE_POCKET {
		packet := pocket.CreateUnlockPacket()
		return cp.Send(key, packet)
	} else if conn.Settings.Type == types.TTYPE_JSP {
		// For JSP, send empty message to clear using JSP protocol
		if conn.JSPConn != nil {
			packet := jsp.CreateMessagePacket("", 0)
			return cp.Send(key, packet)
		}
		return fmt.Errorf("JSP connection not initialized")
	}
	// For GAT/SPHINX, just clear lock
	return nil
}

// GetConnection returns connection by key as types.Connection
func (cp *ConnectionPool) GetConnection(key string) *types.Connection {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	conn, exists := cp.connections[key]
	if !exists {
		return nil
	}

	// Convert to types.Connection
	result := &types.Connection{
		Key:          conn.Key,
		ConKey:       conn.Key,
		IP:           conn.Addr,
		Port:         conn.Port,
		Settings:     conn.Settings,
		StartTime:    conn.StartTime,
		LastActivity: conn.LastActivity,
		Connected:    conn.Connected,
	}

	return result
}

// GetConnections returns all connections
func (cp *ConnectionPool) GetConnections() map[string]*Connection {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	result := make(map[string]*Connection)
	for k, v := range cp.connections {
		result[k] = v
	}
	return result
}

// GetReconnections returns all reconnections
func (cp *ConnectionPool) GetReconnections() map[string]*types.Reconnection {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	result := make(map[string]*types.Reconnection)
	for k, v := range cp.reconnections {
		result[k] = v
	}
	return result
}

// IdleProc performs idle processing (reconnections, timeouts)
func (cp *ConnectionPool) IdleProc() {
	now := time.Now()

	// Process reconnections
	cp.mutex.Lock()
	for key, reconn := range cp.reconnections {
		if now.After(reconn.NTime) {
			fmt.Printf("Attempting reconnection to %s:%d\n", reconn.IP, reconn.Port)
			// Try to reconnect
			newKey, err := cp.StartClient(reconn.IP, reconn.Port, cp.config.TerminalConnectTimeout, nil, nil)
			if err == nil {
				// Restore settings if available
				if reconn.Settings != nil {
					if conn := cp.connections[newKey]; conn != nil {
						conn.Settings = reconn.Settings
					}
				}
				delete(cp.reconnections, key)
				fmt.Printf("Reconnected successfully: %s\n", newKey)
			} else {
				reconn.Count++
				reconn.NTime = now.Add(cp.calculateReconnectionDelay(reconn.Count))
				fmt.Printf("Reconnection failed: %v\n", err)
			}
		}
	}
	cp.mutex.Unlock()

	// Check for timeout connections
	cp.mutex.Lock()
	for key, conn := range cp.connections {
		if conn.Connected {
			timeout := time.Duration(cp.config.ServiceRequestExpireTime * float64(time.Second))
			if now.Sub(conn.LastActivity) > timeout {
				cp.disconnect(key, "activity timeout")
			}
		}
	}
	cp.mutex.Unlock()
}

// Close closes all connections and listeners
func (cp *ConnectionPool) Close() {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	// Close all connections
	for _, conn := range cp.connections {
		if conn.Connected {
			conn.Conn.Close()
		}
	}
	cp.connections = make(map[string]*Connection)

	// Close all listeners
	for _, listener := range cp.listeners {
		listener.Close()
	}
	cp.listeners = make(map[string]*net.TCPListener)
}

// detectProtocol tries to detect protocol from data
func (cp *ConnectionPool) detectProtocol(conn *Connection) {
	if len(conn.Buffer) == 0 {
		return
	}

	// Check for POCKET marker
	if len(conn.Buffer) >= 1 && conn.Buffer[0] == 0x2A {
		// Looks like POCKET protocol
		if conn.Settings == nil {
			conn.Settings = &types.TerminalSettings{Type: types.TTYPE_POCKET}
		}
		cp.processPocketData(conn)
		return
	}

	// Check for GAT marker (could be various)
	if len(conn.Buffer) >= 4 {
		// Could be GAT or other protocol
		if conn.Settings == nil {
			conn.Settings = &types.TerminalSettings{Type: types.TTYPE_GAT}
		}
		cp.processGatData(conn)
		return
	}

	// Unknown protocol
	fmt.Printf("Unknown protocol data from %s: %x\n", conn.Key, conn.Buffer[:min(20, len(conn.Buffer))])
	conn.Buffer = nil // Clear buffer
}

// processPocketData processes POCKET protocol data
func (cp *ConnectionPool) processPocketData(conn *Connection) {
	// Try to find complete packet in buffer
	for len(conn.Buffer) >= 8 {
		// Check if we have enough data for header
		if len(conn.Buffer) < 7 {
			break
		}

		// Check marker
		if conn.Buffer[0] != 0x2A {
			// Invalid marker, skip byte
			conn.Buffer = conn.Buffer[1:]
			continue
		}

		// Get payload length
		payloadLen := int(utils.DecodeUint16(conn.Buffer[3:5]))
		packetLen := 7 + payloadLen

		if len(conn.Buffer) < packetLen {
			// Not enough data for complete packet
			break
		}

		// Extract packet
		packetData := conn.Buffer[:packetLen]
		conn.Buffer = conn.Buffer[packetLen:]

		// Process packet
		cp.handlePocketPacket(conn, packetData)
	}
}

// handlePocketPacket handles decoded POCKET packet
func (cp *ConnectionPool) handlePocketPacket(conn *Connection, data []byte) {
	packet, err := pocket.DecodePacket(data)
	if err != nil {
		fmt.Printf("Failed to decode POCKET packet from %s: %v\n", conn.Key, err)
		return
	}

	fmt.Printf("POCKET packet from %s: cmd=0x%02X\n", conn.Key, packet.Cmd)

	// Handle specific commands
	switch packet.Cmd {
	case 0x02: // ReadTag response
		cp.handlePocketTagRead(conn, packet)
	case 0x03: // ReadTagExtended (with lockers)
		cp.handlePocketTagReadExtended(conn, packet)
	case 0x16: // InputChanged
		cp.handlePocketInputChanged(conn, packet)
	case 0x15: // RelayControlEx response
		cp.handlePocketRelayResponse(conn, packet)
	case 0x86: // Enquire response (pong)
		cp.handlePocketEnquireResponse(conn, packet)
	}
}

// handlePocketTagRead handles RFID/biometric tag read (ReadTag - 0x02)
func (cp *ConnectionPool) handlePocketTagRead(conn *Connection, packet *types.Packet) {
	uid, ok := packet.Data["uid"].(string)
	if !ok || uid == "" {
		fmt.Printf("No UID in POCKET tag read from %s\n", conn.Key)
		return
	}

	// Check if this is a MEMREG device terminal
	if conn.Settings != nil && conn.Settings.MemRegDev != "" {
		// Handle MEMREG device (towel/add, towel/take)
		cp.handleMemRegDevice(conn, packet)
		return
	}

	auth, _ := packet.Data["auth"].(bool)
	readerType, _ := packet.Data["reader_type"].(uint8)

	fmt.Printf("POCKET Tag read: key=%s, uid=%s, auth=%v, reader_type=%s\n",
		conn.Key, uid, auth, pocket.GetReaderTypeString(readerType))

	// Call event handler
	if cp.onTagRead != nil {
		cp.onTagRead(conn.Key, uid, readerType, auth)
	}
}

// handlePocketTagReadExtended handles RFID/biometric tag read with lockers (ReadTagExtended - 0x03)
func (cp *ConnectionPool) handlePocketTagReadExtended(conn *Connection, packet *types.Packet) {
	uid, ok := packet.Data["uid"].(string)
	if !ok || uid == "" {
		fmt.Printf("No UID in POCKET tag read extended from %s\n", conn.Key)
		return
	}

	auth, _ := packet.Data["auth"].(bool)

	// Check if this is a MEMREG device terminal
	if conn.Settings != nil && conn.Settings.MemRegDev != "" {
		// Handle MEMREG device (towel/add, towel/take)
		cp.handleMemRegDevice(conn, packet)
		return
	}

	// Get lockers data and temp_card
	var lockers []types.LockerInfo
	var tempCard bool
	if lockersData, ok := packet.Data["lockers_data"].([]types.LockerInfo); ok {
		lockers = lockersData
	}
	if tempCardVal, ok := packet.Data["temp_card"].(bool); ok {
		tempCard = tempCardVal
	}
	fmt.Printf("POCKET Tag read extended: key=%s, uid=%s, auth=%v, lockers_count=%d, temp_card=%v\n",
		conn.Key, uid, auth, len(lockers), tempCard)

	// Store lockers and temp_card in connection for later retrieval
	// This is a workaround - ideally we'd pass lockers through event handler
	if len(lockers) > 0 || tempCard {
		// Store in connection settings extra data
		if conn.Settings == nil {
			conn.Settings = &types.TerminalSettings{}
		}
		if conn.Settings.Extra == nil {
			conn.Settings.Extra = make(map[string]interface{})
		}
		if len(lockers) > 0 {
			conn.Settings.Extra["last_lockers"] = lockers
		}
		if tempCard {
			conn.Settings.Extra["last_temp_card"] = true
		}
		conn.Settings.Extra["last_uid"] = uid
	}

	// Call event handler - we'll need to pass lockers through event
	// For now, use readerType 0 for extended read
	if cp.onTagRead != nil {
		cp.onTagRead(conn.Key, uid, 0, auth)
	}
}

// handlePocketInputChanged handles input state changes (person passed)
func (cp *ConnectionPool) handlePocketInputChanged(conn *Connection, packet *types.Packet) {
	passed, ok := packet.Data["passed"].(bool)
	if !ok {
		return
	}

	fmt.Printf("POCKET Person passed: key=%s, passed=%v\n", conn.Key, passed)

	// Call event handler
	if cp.onPassEvent != nil {
		cp.onPassEvent(conn.Key, passed)
	}
}

// handlePocketRelayResponse handles relay control response
func (cp *ConnectionPool) handlePocketRelayResponse(conn *Connection, packet *types.Packet) {
	fmt.Printf("POCKET Relay response from %s\n", conn.Key)
	// Relay operation completed
}

// handlePocketEnquireResponse handles Enquire response (pong)
func (cp *ConnectionPool) handlePocketEnquireResponse(conn *Connection, packet *types.Packet) {
	// Update last activity time
	conn.LastActivity = time.Now()

	// Reset ping state if we have it
	if conn.PocketPing != nil {
		conn.PocketPing.PingSent = false
		conn.PocketPing.PingSinceLast = 0
	}

	fmt.Printf("POCKET Enquire response (pong) from %s\n", conn.Key)
}

// processGatData processes GAT protocol data
func (cp *ConnectionPool) processGatData(conn *Connection) {
	// Try to find complete packet in buffer
	for len(conn.Buffer) >= 8 {
		// Check marker
		if conn.Buffer[0] != 0x2A {
			// Invalid marker, skip byte
			conn.Buffer = conn.Buffer[1:]
			continue
		}

		// Get payload length
		lenLow := int(conn.Buffer[3])
		lenHigh := int(conn.Buffer[4])
		payloadLen := lenLow + (lenHigh << 8)
		packetLen := 8 + payloadLen

		if len(conn.Buffer) < packetLen {
			// Not enough data for complete packet
			break
		}

		// Extract packet
		packetData := conn.Buffer[:packetLen]
		conn.Buffer = conn.Buffer[packetLen:]

		// Process packet
		cp.handleGatPacket(conn, packetData)
	}
}

// handleGatPacket handles decoded GAT packet
func (cp *ConnectionPool) handleGatPacket(conn *Connection, data []byte) {
	packet, err := gat.DecodePacket(data)
	if err != nil {
		fmt.Printf("Failed to decode GAT packet from %s: %v\n", conn.Key, err)
		return
	}

	fmt.Printf("GAT packet from %s: cmd=0x%02X\n", conn.Key, packet.Cmd)

	// Handle specific commands
	switch packet.Cmd {
	case 0xE5: // REQ_MASTER (ping response)
		cp.handleGatReqMasterResponse(conn, packet)
	case 0x80: // CARD_IDENT
		cp.handleGatCardIdent(conn, packet)
	}
}

// handleGatReqMasterResponse handles REQ_MASTER response (pong)
func (cp *ConnectionPool) handleGatReqMasterResponse(conn *Connection, packet *types.Packet) {
	// Update last activity time
	conn.LastActivity = time.Now()

	// Reset ping state if we have it
	if conn.GatPing != nil {
		conn.GatPing.PingSent = false
		conn.GatPing.PingSinceLast = 0
	}

	fmt.Printf("GAT REQ_MASTER response (pong) from %s\n", conn.Key)
}

// handleGatCardIdent handles CARD_IDENT command
func (cp *ConnectionPool) handleGatCardIdent(conn *Connection, packet *types.Packet) {
	uidHex, ok := packet.Data["uid_hex"].(string)
	if !ok || uidHex == "" {
		fmt.Printf("No UID in GAT card ident from %s\n", conn.Key)
		return
	}

	readerType, _ := packet.Data["reader_type"].(uint8)

	fmt.Printf("GAT Card ident: key=%s, uid=%s, reader_type=%d\n",
		conn.Key, uidHex, readerType)

	// Call event handler
	if cp.onTagRead != nil {
		cp.onTagRead(conn.Key, uidHex, readerType, true)
	}
}

// processSphinxData processes SPHINX protocol data
func (cp *ConnectionPool) processSphinxData(conn *Connection) {
	// SPHINX uses text-based protocol with \r\n delimiter
	bufferStr := string(conn.Buffer)

	// Find complete packets (ending with \r\n)
	for {
		delimPos := strings.Index(bufferStr, "\r\n")
		if delimPos == -1 {
			// No complete packet found
			break
		}

		// Extract packet
		packetData := []byte(bufferStr[:delimPos+2])
		bufferStr = bufferStr[delimPos+2:]
		conn.Buffer = []byte(bufferStr)

		// Process packet
		cp.handleSphinxPacket(conn, packetData)
	}
}

// handleSphinxPacket handles decoded SPHINX packet
func (cp *ConnectionPool) handleSphinxPacket(conn *Connection, data []byte) {
	packet, err := sphinx.DecodePacket(data)
	if err != nil {
		fmt.Printf("Failed to decode SPHINX packet from %s: %v\n", conn.Key, err)
		return
	}

	cmd, ok := packet.Data["command"].(string)
	if !ok {
		return
	}

	fmt.Printf("SPHINX packet from %s: cmd=%s\n", conn.Key, cmd)

	// Handle specific commands
	switch cmd {
	case "DELEGATION_START": // Response to our ping
		cp.handleSphinxDelegationStartResponse(conn, packet)
	case "DELEGATION_REQUEST":
		cp.handleSphinxDelegationRequest(conn, packet)
	case "OK":
		// Generic OK response
		if conn.SphinxPing != nil {
			conn.SphinxPing.PingSent = false
			conn.SphinxPing.PingSinceLast = 0
			conn.LastActivity = time.Now()
		}
	}
}

// handleSphinxDelegationStartResponse handles DELEGATION_START response (pong)
func (cp *ConnectionPool) handleSphinxDelegationStartResponse(conn *Connection, packet *types.Packet) {
	// Update last activity time
	conn.LastActivity = time.Now()

	// Reset ping state if we have it
	if conn.SphinxPing != nil {
		conn.SphinxPing.PingSent = false
		conn.SphinxPing.PingSinceLast = 0
	}

	fmt.Printf("SPHINX DELEGATION_START response (pong) from %s\n", conn.Key)
}

// handleSphinxDelegationRequest handles DELEGATION_REQUEST command
func (cp *ConnectionPool) handleSphinxDelegationRequest(conn *Connection, packet *types.Packet) {
	params, ok := packet.Data["params"].([]string)
	if !ok || len(params) < 2 {
		fmt.Printf("Invalid DELEGATION_REQUEST from %s\n", conn.Key)
		return
	}

	// Parse ticket and access type
	ticket := params[0]
	accessType := params[1]

	fmt.Printf("SPHINX Delegation request: key=%s, ticket=%s, access_type=%s\n",
		conn.Key, ticket, accessType)

	// TODO: Process delegation request and create session
	// For now, just log it
}

// processJSPData processes JSP protocol data
func (cp *ConnectionPool) processJSPData(conn *Connection) {
	if conn.JSPConn == nil {
		conn.JSPConn = jsp.NewJSPConnection()
	}

	// Add data to JSP buffer
	conn.JSPConn.Buffer = append(conn.JSPConn.Buffer, conn.Buffer...)
	conn.Buffer = nil

	// Try to read packets
	for {
		result, err := jsp.TryReadPacket(conn.JSPConn)
		if err != nil {
			fmt.Printf("JSP packet error from %s: %v\n", conn.Key, err)
			break
		}

		// Check result type
		switch v := result.(type) {
		case bool:
			if !v {
				// No more packets
				break
			}
			continue
		case int:
			// Need more data
			return
		case map[string]interface{}:
			// Got packet, process it
			cp.handleJSPPacket(conn, v)
		default:
			// Unknown result
			break
		}
	}
}

// handleJSPPacket handles decoded JSP packet
func (cp *ConnectionPool) handleJSPPacket(conn *Connection, packet map[string]interface{}) {
	packetType, data, err := jsp.ProcessPacket(packet)
	if err != nil {
		fmt.Printf("JSP packet processing error from %s: %v\n", conn.Key, err)
		return
	}

	fmt.Printf("JSP packet from %s: type=%s\n", conn.Key, packetType)

	switch packetType {
	case "command":
		cp.handleJSPCommand(conn, data)
	case "answer":
		cp.handleJSPAnswer(conn, data)
	default:
		fmt.Printf("Unknown JSP packet type: %s\n", packetType)
	}
}

// handleJSPCommand handles JSP command
func (cp *ConnectionPool) handleJSPCommand(conn *Connection, packet map[string]interface{}) {
	cmd, ok := packet["cmd"].(string)
	if !ok {
		return
	}

	cmd = strings.ToLower(strings.TrimSpace(cmd))

	switch cmd {
	case "tag_read":
		cp.handleJSPTagRead(conn, packet)
	case "pass_report":
		cp.handleJSPPassReport(conn, packet)
	case "pong":
		// Ping response, just update activity
		cp.handleJSPPong(conn, packet)
	default:
		// Unknown command, send answer if has RID
		if rid, ok := packet["rid"].(string); ok {
			answer, _ := jsp.AnswerRequest(rid, map[string]interface{}{"result": false, "error": "unknown command"})
			cp.Send(conn.Key, answer)
		}
	}
}

// handleJSPTagRead handles JSP tag_read command
func (cp *ConnectionPool) handleJSPTagRead(conn *Connection, packet map[string]interface{}) {
	uid, ok := packet["uid"].(string)
	if !ok {
		fmt.Printf("JSP tag_read missing UID from %s\n", conn.Key)
		return
	}

	// Transform lockers data if present
	jsp.TransformLockersData(packet)

	// Extract reader type (default to RFID)
	readerType := uint8(0x01) // Card Reader
	if rt, ok := packet["reader_type"].(float64); ok {
		readerType = uint8(rt)
	}

	auth := true
	if a, ok := packet["auth"].(bool); ok {
		auth = a
	}

	fmt.Printf("JSP Tag read: key=%s, uid=%s, reader_type=%d, auth=%v\n",
		conn.Key, uid, readerType, auth)

	// Call event handler
	if cp.onTagRead != nil {
		cp.onTagRead(conn.Key, uid, readerType, auth)
	}

	// Store RID if present
	if rid, ok := packet["rid"].(string); ok {
		// Store in connection for later use
		if conn.JSPConn != nil {
			if conn.JSPConn.Requests == nil {
				conn.JSPConn.Requests = make(map[string]*jsp.JSPRequest)
			}
			conn.JSPConn.Requests[rid] = &jsp.JSPRequest{
				ID:   rid,
				RKey: rid,
				Cmd:  "tag_read",
				Time: utils.GetMtf(),
			}
		}
	}
}

// handleJSPPassReport handles JSP pass_report command
func (cp *ConnectionPool) handleJSPPassReport(conn *Connection, packet map[string]interface{}) {
	passed := true
	if p, ok := packet["passed"].(bool); ok {
		passed = p
	}

	fmt.Printf("JSP Pass report: key=%s, passed=%v\n", conn.Key, passed)

	// Call event handler
	if cp.onPassEvent != nil {
		cp.onPassEvent(conn.Key, passed)
	}
}

// handleJSPAnswer handles JSP answer packet
func (cp *ConnectionPool) handleJSPAnswer(conn *Connection, packet map[string]interface{}) {
	rid, ok := packet["rid"].(string)
	if !ok {
		return
	}

	// Find request
	if conn.JSPConn == nil || conn.JSPConn.Requests == nil {
		return
	}

	request, exists := conn.JSPConn.Requests[rid]
	if !exists {
		return
	}

	fmt.Printf("JSP Answer for request %s from %s\n", rid, conn.Key)

	// Process answer based on request command
	switch request.Cmd {
	case "relay_open":
		// Relay opened
		fmt.Printf("JSP Relay opened for request %s\n", rid)
	case "message":
		// Message sent
		fmt.Printf("JSP Message sent for request %s\n", rid)
	case "ping":
		// Ping response (pong)
		fmt.Printf("JSP Pong received for request %s\n", rid)
		conn.JSPConn.PingSent = false
		conn.JSPConn.PingSinceLast = 0
		conn.LastActivity = time.Now()
	}

	// Remove request
	delete(conn.JSPConn.Requests, rid)
}

// handleJSPPong handles JSP pong response
func (cp *ConnectionPool) handleJSPPong(conn *Connection, packet map[string]interface{}) {
	conn.JSPConn.PingSent = false
	conn.JSPConn.PingSinceLast = 0
	conn.LastActivity = time.Now()
	fmt.Printf("JSP Pong received from %s\n", conn.Key)
}

// SendJSPMessage sends message to JSP terminal
func (cp *ConnectionPool) SendJSPMessage(key string, text string, timeMs int) error {
	cp.mutex.RLock()
	conn, exists := cp.connections[key]
	cp.mutex.RUnlock()

	if !exists || !conn.Connected {
		return fmt.Errorf("connection not found or not connected: %s", key)
	}

	if conn.Settings == nil || conn.Settings.Type != types.TTYPE_JSP {
		return fmt.Errorf("connection is not JSP type: %s", key)
	}

	if conn.JSPConn == nil {
		return fmt.Errorf("JSP connection not initialized: %s", key)
	}

	packet := jsp.CreateMessagePacket(text, timeMs)
	return cp.Send(key, packet)
}

// SendJSPRelayOpen sends relay open command to JSP terminal
func (cp *ConnectionPool) SendJSPRelayOpen(key string, uid string, caption string, timeMs int, cid string) error {
	cp.mutex.RLock()
	conn, exists := cp.connections[key]
	cp.mutex.RUnlock()

	if !exists || !conn.Connected {
		return fmt.Errorf("connection not found or not connected: %s", key)
	}

	if conn.Settings == nil || conn.Settings.Type != types.TTYPE_JSP {
		return fmt.Errorf("connection is not JSP type: %s", key)
	}

	if conn.JSPConn == nil {
		return fmt.Errorf("JSP connection not initialized: %s", key)
	}

	packet, err := jsp.SendRelayOpen(&conn.JSPRIDCounter, uid, caption, timeMs, cid)
	if err != nil {
		return err
	}

	// Store request
	if conn.JSPConn.Requests == nil {
		conn.JSPConn.Requests = make(map[string]*jsp.JSPRequest)
	}

	// Extract RID from packet (parse JSON)
	var packetData map[string]interface{}
	if err := json.Unmarshal(packet[5:len(packet)-1], &packetData); err == nil {
		if rid, ok := packetData["rid"].(string); ok {
			conn.JSPConn.Requests[rid] = &jsp.JSPRequest{
				ID:   rid,
				RKey: rid,
				Cmd:  "relay_open",
				Time: utils.GetMtf(),
			}
		}
	}

	return cp.Send(key, packet)
}

// SendJSPRelayClose sends relay close command to JSP terminal
func (cp *ConnectionPool) SendJSPRelayClose(key string) error {
	cp.mutex.RLock()
	conn, exists := cp.connections[key]
	cp.mutex.RUnlock()

	if !exists || !conn.Connected {
		return fmt.Errorf("connection not found or not connected: %s", key)
	}

	if conn.Settings == nil || conn.Settings.Type != types.TTYPE_JSP {
		return fmt.Errorf("connection is not JSP type: %s", key)
	}

	if conn.JSPConn == nil {
		return fmt.Errorf("JSP connection not initialized: %s", key)
	}

	packet, err := jsp.SendRelayClose(&conn.JSPRIDCounter)
	if err != nil {
		return err
	}

	// Store request
	if conn.JSPConn.Requests == nil {
		conn.JSPConn.Requests = make(map[string]*jsp.JSPRequest)
	}

	// Extract RID from packet
	// Packet format: SOF(1) + LENGTH(4) + JSON + EOF(1)
	var packetData map[string]interface{}
	if len(packet) > 6 {
		jsonData := packet[5 : len(packet)-1]
		if err := json.Unmarshal(jsonData, &packetData); err == nil {
		if rid, ok := packetData["rid"].(string); ok {
			conn.JSPConn.Requests[rid] = &jsp.JSPRequest{
				ID:   rid,
				RKey: rid,
				Cmd:  "relay_close",
				Time: utils.GetMtf(),
			}
		}
		}
	}

	return cp.Send(key, packet)
}

// min returns minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetTerminalList returns list of terminals from 1C with connection status
// Also includes active connections that match the filter, even if not in 1C list
// Optimized for fast response - returns terminals immediately even if connection checks are pending
func (cp *ConnectionPool) GetTerminalList() []map[string]interface{} {
	cp.mutex.RLock()
	
	// Get terminal list from config (set by daemon)
	terminalList := cp.config.TerminalList
	if terminalList == nil {
		terminalList = []map[string]interface{}{}
	}
	
	// Get connections and reconnections for quick lookup
	connections := make(map[string]*Connection)
	for k, v := range cp.connections {
		connections[k] = v
	}
	reconnections := make(map[string]*types.Reconnection)
	for k, v := range cp.reconnections {
		reconnections[k] = v
	}
	filter := cp.config.TermListFilter
	filterAbsent := cp.config.TermListFilterAbsent
	
	cp.mutex.RUnlock() // Release lock early to allow concurrent access

	// Build a map to track terminals by IP:Port (to avoid duplicates)
	terminalMap := make(map[string]map[string]interface{})
	
	// First, add all terminals from 1C list (even if not connected)
	for _, termData := range terminalList {
		// Extract terminal info - try to get from parsed settings first
		var id, ip string
		var port int
		var termType string
		
		// Check if terminal was parsed and has settings stored
		if settings, ok := termData["_parsed_settings"].(*types.TerminalSettings); ok && settings != nil {
			id = settings.ID
			ip = settings.IP
			port = settings.Port
			termType = string(settings.Type)
		} else {
			// Fallback to parsing from raw data
			id = utils.GetStringValue(termData, "ID", utils.GetStringValue(termData, "id", ""))
			ipStr := utils.GetStringValue(termData, "IP", utils.GetStringValue(termData, "ip", ""))
			
			// Parse IP string to extract IP and port
			if parsed, err := utils.ParseTerm(id + ":" + ipStr); err == nil && parsed != nil {
				ip = parsed.IP
				port = parsed.Port
				termType = string(parsed.Type)
			} else if parsed, err := utils.ParseTerm(ipStr); err == nil && parsed != nil {
				ip = parsed.IP
				port = parsed.Port
				termType = string(parsed.Type)
			} else {
				// Simple fallback parsing
				parts := strings.Split(ipStr, ":")
				ip = parts[0]
				if len(parts) > 1 {
					if portInt, err := strconv.Atoi(parts[1]); err == nil {
						port = portInt
					} else {
						port = 8080
					}
				} else {
					port = 8080
				}
				
				// Try to extract type from IP string
				termType = "gat" // default
				for i := 2; i < len(parts); i++ {
					if strings.HasPrefix(parts[i], "type=") {
						typeVal := strings.TrimPrefix(parts[i], "type=")
						termType = typeVal
						break
					}
				}
			}
			
			// Override from separate fields if available
			if portVal, ok := termData["PORT"]; ok {
				if portFloat, ok := portVal.(float64); ok {
					port = int(portFloat)
				} else if portInt, ok := portVal.(int); ok {
					port = portInt
				}
			}
			if typeStr := utils.GetStringValue(termData, "TYPE", utils.GetStringValue(termData, "type", "")); typeStr != "" {
				termType = typeStr
			}
		}
		
		// Find connection by IP:Port or by terminal ID
		connected := false
		connKey := ""
		lastActivity := time.Time{}
		connectionError := ""
		reconnecting := false
		
		// Try to find connection by IP:Port pattern (using cached connections map)
		for key, conn := range connections {
			if conn.Addr == ip && conn.Port == port {
				connected = conn.Connected
				connKey = key
				lastActivity = conn.LastActivity
				break
			}
		}
		
		// If not found by IP:Port, try to find by terminal ID in settings
		if !connected {
			for key, conn := range connections {
				if conn.Settings != nil && conn.Settings.ID == id {
					connected = conn.Connected
					connKey = key
					lastActivity = conn.LastActivity
					break
				}
			}
		}
		
		// Check if terminal is in reconnection queue (using cached reconnections map)
		if !connected {
			for key, reconn := range reconnections {
				if reconn.IP == ip && reconn.Port == port {
					reconnecting = true
					connKey = key
					connectionError = fmt.Sprintf("Переподключение (попытка %d)", reconn.Count)
					break
				}
			}
		}
		
		// Check for connection error in terminal data
		if !connected && !reconnecting {
			if errMsg, ok := termData["_connection_error"].(string); ok && errMsg != "" {
				connectionError = errMsg
			} else if lastAttempt, ok := termData["_last_connection_attempt"].(time.Time); ok && !lastAttempt.IsZero() {
				connectionError = "Не удалось подключиться"
			}
		}
		
		// Build terminal info
		termInfo := make(map[string]interface{})
		termInfo["id"] = id
		termInfo["ip"] = ip
		termInfo["port"] = port
		termInfo["type"] = termType
		termInfo["key"] = connKey
		termInfo["connected"] = connected
		termInfo["reconnecting"] = reconnecting
		termInfo["connection_error"] = connectionError
		termInfo["last_activity"] = lastActivity
		
		// Copy original data
		for k, v := range termData {
			if _, exists := termInfo[k]; !exists {
				termInfo[k] = v
			}
		}
		
		// Store in map using IP:Port as key (to avoid duplicates)
		mapKey := fmt.Sprintf("%s:%d", ip, port)
		terminalMap[mapKey] = termInfo
	}
	
	// Second, add all active connections that match the filter, even if not in 1C list
	for key, conn := range connections {
		// Check if connection matches filter
		if filter != "" {
			if !utils.FilterTerminalList(conn.Addr, filter, filterAbsent) {
				continue // Skip if doesn't match filter
			}
		}
		
		// Check if we already have this terminal from 1C list
		mapKey := fmt.Sprintf("%s:%d", conn.Addr, conn.Port)
		if _, exists := terminalMap[mapKey]; exists {
			// Already in list, skip (keep the one from 1C as it has more info)
			continue
		}
		
		// Extract terminal info from connection
		id := ""
		termType := "gat"
		if conn.Settings != nil {
			if conn.Settings.ID != "" {
				id = conn.Settings.ID
			}
			termType = string(conn.Settings.Type)
		}
		
		// Generate ID if missing
		if id == "" {
			id = key
		}
		
		// Create terminal info for active connection
		termInfo := make(map[string]interface{})
		termInfo["id"] = id
		termInfo["ip"] = conn.Addr
		termInfo["port"] = conn.Port
		termInfo["type"] = termType
		termInfo["key"] = key
		termInfo["connected"] = conn.Connected
		termInfo["reconnecting"] = false
		termInfo["connection_error"] = ""
		termInfo["last_activity"] = conn.LastActivity
		
		// Add to map
		terminalMap[mapKey] = termInfo
	}
	
	// Convert map to slice
	result := make([]map[string]interface{}, 0, len(terminalMap))
	for _, termInfo := range terminalMap {
		result = append(result, termInfo)
	}
	
	return result
}

// ConnectToTerminal manually connects to a terminal
func (cp *ConnectionPool) ConnectToTerminal(ip string, port int) error {
	key := fmt.Sprintf("%s:%d", ip, port)

	// Check if already connected
	cp.mutex.RLock()
	if _, exists := cp.connections[key]; exists {
		cp.mutex.RUnlock()
		return fmt.Errorf("already connected to %s", key)
	}
	cp.mutex.RUnlock()

	// Start client connection
	var errCode int
	var errStr string
	_, err := cp.StartClient(ip, port, cp.config.TerminalConnectTimeout, &errCode, &errStr)
	if err != nil {
		return fmt.Errorf("failed to connect: %v (code: %d, str: %s)", err, errCode, errStr)
	}
	return nil
}
