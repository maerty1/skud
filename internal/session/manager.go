package session

import (
	"fmt"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"sync"
	"time"
)

// CSVLoggerInterface defines CSV logger methods
type CSVLoggerInterface interface {
	LogSession(session *types.Session, conn *types.Connection) error
}

// SessionManager manages user sessions
type SessionManager struct {
	sessions     map[string]*types.Session
	idGen        int
	mutex        sync.RWMutex
	config       *types.Config
	httpClient   HTTPClientInterface
	heliosClient HeliosClientInterface
	pool         interface{}        // ConnectionPool interface
	csvLogger    CSVLoggerInterface // CSV logger
}

// HeliosClientInterface defines Helios client methods
type HeliosClientInterface interface {
	StartVerification(sessionID string, camPID string, personID string) (string, error)
	CloseRequest(requestID string)
}

// ConnectionPoolInterface defines connection pool methods
type ConnectionPoolInterface interface {
	SendJSPRelayOpen(key string, uid string, caption string, timeMs int, cid string) error
	SendJSPRelayClose(key string) error
	SendJSPMessage(key string, text string, timeMs int) error
	Send(key string, data []byte) error
	GetConnection(key string) *types.Connection
	LockTerminal(key string, sessionID string, text string) error
	UnlockTerminal(key string, sessionID string) error
}

// HTTPClientInterface defines HTTP client methods
type HTTPClientInterface interface {
	CheckAccess(uid string, terminalID string, tagType string, lockers []types.LockerInfo) (*types.KPOResult, string, error)
	SendAccessReport(uid string, terminalID string, result bool, message string) error
	GetUserCID(uid string) (string, error)
}

// NewSessionManager creates new session manager
func NewSessionManager(config *types.Config) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*types.Session),
		idGen:    0,
		config:   config,
	}
}

// SetHTTPClient sets HTTP client for 1C integration
func (sm *SessionManager) SetHTTPClient(client interface{}) {
	if hc, ok := client.(HTTPClientInterface); ok {
		sm.httpClient = hc
	}
}

// SetPool sets connection pool for sending commands
func (sm *SessionManager) SetPool(pool interface{}) {
	sm.pool = pool
}

// SetCSVLogger sets CSV logger for session logging
func (sm *SessionManager) SetCSVLogger(logger CSVLoggerInterface) {
	sm.csvLogger = logger
}

// ProcessSessionStage processes session based on current stage
func (sm *SessionManager) ProcessSessionStage(sessionID string) error {
	sm.mutex.Lock()
	session, exists := sm.sessions[sessionID]
	sm.mutex.Unlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.Processed || session.Completed {
		return nil // Already processed
	}

	// Check wait state first
	if session.Wait != nil {
		if !sm.checkWait(session) {
			// Still waiting
			return nil
		}
		// Wait completed, continue processing
	}

	switch session.Stage {
	case types.SESSION_STAGE_KPO_RESULT:
		return sm.processKpoResult(session)
	case types.SESSION_STAGE_KPO_DIRECT:
		return sm.processKpoDirect(session)
	case types.SESSION_STAGE_OPEN_FIRST:
		return sm.processOpenFirst(session)
	case types.SESSION_STAGE_FIRST_PASSED:
		return sm.processFirstPassed(session)
	case types.SESSION_STAGE_CAM_RESULT:
		return sm.processCamResult(session)
	case types.SESSION_STAGE_OPEN_SECOND:
		return sm.processOpenSecond(session)
	case types.SESSION_STAGE_SECOND_PASSED:
		return sm.processSecondPassed(session)
	case types.SESSION_STAGE_PASSED:
		return sm.processPassed(session)
	case types.SESSION_STAGE_LAST_ANSWER:
		return sm.processLastAnswer(session)
	case types.SESSION_STAGE_DONE:
		return sm.processDone(session)
	}

	return nil
}

// processKpoResult processes KPO result stage
func (sm *SessionManager) processKpoResult(session *types.Session) error {
	kpoData, ok := session.Data["kpo"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no KPO data in session")
	}

	result, ok := kpoData["result"].(types.KPOResult)
	if !ok {
		return nil // Wait for result
	}

	message, _ := kpoData["message"].(string)
	if message == "" {
		message = "Проходите" // Default allow message
	}

	if result != types.KPO_RES_YES {
		// Access denied
		session.Data["result"] = 0
		session.Data["message"] = message
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
		return nil
	}

	// Access granted
	session.Data["result"] = 1
	session.Data["message"] = message

	// Check if we need camera verification
	if sm.config.CamServiceActive {
		session.Stage = types.SESSION_STAGE_OPEN_FIRST
	} else {
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
	}

	return nil
}

// processOpenFirst processes opening first gate
func (sm *SessionManager) processOpenFirst(session *types.Session) error {
	message, _ := session.Data["message"].(string)
	if message == "" {
		message = "Проходите"
	}

	// TODO: Send relay open command to terminal
	// For now, just move to waiting for pass
	session.Stage = types.SESSION_STAGE_FIRST_PASSED

	return nil
}

// processFirstPassed processes first gate passed
func (sm *SessionManager) processFirstPassed(session *types.Session) error {
	passed, ok := session.Data["passed_first"].(bool)
	if !ok {
		return nil // Wait for pass event
	}

	if !passed {
		session.Data["result"] = 0
		session.Data["message"] = "Проход не зарегистрирован"
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
		return nil
	}

	// Check if camera verification is needed
	if sm.config.CamServiceActive && session.CID != "" {
		session.Stage = types.SESSION_STAGE_CAM_RESULT
	} else {
		session.Stage = types.SESSION_STAGE_OPEN_SECOND
	}

	return nil
}

// processCamResult processes camera recognition result
func (sm *SessionManager) processCamResult(session *types.Session) error {
	camData, ok := session.Data["cam"].(map[string]interface{})
	if !ok {
		return nil // Wait for camera result
	}

	result, ok := camData["result"].(types.CamResult)
	if !ok {
		return nil // Wait for result
	}

	switch result {
	case types.CAM_RES_YES:
		session.Data["result"] = 1
		session.Stage = types.SESSION_STAGE_OPEN_SECOND
	case types.CAM_RES_NO:
		session.Data["result"] = 0
		session.Data["message"] = sm.config.CamServiceResultMsgNo
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
	case types.CAM_RES_NF:
		session.Data["result"] = 0
		session.Data["message"] = sm.config.CamServiceResultMsgNf
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
	case types.CAM_RES_FAIL:
		session.Data["result"] = 0
		session.Data["message"] = sm.config.CamServiceResultMsgFail
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
	default:
		if sm.config.CamAlwaysPass {
			session.Data["result"] = 1
			session.Stage = types.SESSION_STAGE_OPEN_SECOND
		} else {
			session.Data["result"] = 0
			session.Data["message"] = "Ошибка распознавания"
			session.Stage = types.SESSION_STAGE_LAST_ANSWER
		}
	}

	return nil
}

// processOpenSecond processes opening second gate
func (sm *SessionManager) processOpenSecond(session *types.Session) error {
	// TODO: Send relay open command to second terminal
	session.Stage = types.SESSION_STAGE_SECOND_PASSED
	return nil
}

// processSecondPassed processes second gate passed
func (sm *SessionManager) processSecondPassed(session *types.Session) error {
	passed, ok := session.Data["passed_second"].(bool)
	if !ok {
		return nil // Wait for pass event
	}

	session.Data["passed"] = passed
	session.Stage = types.SESSION_STAGE_PASSED

	return nil
}

// processPassed processes successful access completion
func (sm *SessionManager) processPassed(session *types.Session) error {
	// Send access report to 1C
	if sm.httpClient != nil {
		terminalID := sm.extractTerminalID(session.Key)
		result := session.Data["result"].(int) > 0

		if err := sm.httpClient.SendAccessReport(session.UID, terminalID, result, ""); err != nil {
			fmt.Printf("Failed to send access report for session %s: %v\n", session.ID, err)
			// Continue anyway
		}
	}

	session.ReportSent = true
	session.Stage = types.SESSION_STAGE_DONE
	return nil
}

// processLastAnswer processes final answer stage
func (sm *SessionManager) processLastAnswer(session *types.Session) error {
	result, _ := session.Data["result"].(int)
	message, _ := session.Data["message"].(string)

	if result > 0 {
		// Send allow message to terminal
		sm.sendAllowMessage(session, message)
		session.Stage = types.SESSION_STAGE_PASSED
	} else {
		// Send deny message to terminal
		sm.sendDenyMessage(session, message)
		session.Stage = types.SESSION_STAGE_DONE
	}

	return nil
}

// sendAllowMessage sends allow message to terminal
func (sm *SessionManager) sendAllowMessage(session *types.Session, message string) {
	if sm.pool == nil {
		return
	}

	pool, ok := sm.pool.(ConnectionPoolInterface)
	if !ok {
		return
	}

	// Calculate pass time (default 3 seconds)
	passTime := 3000
	if pt, ok := session.Data["pass_time"].(int); ok && pt > 0 {
		passTime = pt
	}

	// Get connection settings to determine terminal type
	// For now, try JSP first, then fallback to other protocols
	cid := session.CID
	if cid == "" {
		cid = session.UID
	}

	// Try JSP protocol
	if err := pool.SendJSPRelayOpen(session.Key, session.UID, message, passTime, cid); err != nil {
		// If JSP fails, try other protocols or just log
		fmt.Printf("Failed to send JSP relay open: %v\n", err)
	}
}

// checkTagReadDeny checks if tag read should be denied (lockers, temp_card)
func (sm *SessionManager) checkTagReadDeny(session *types.Session) bool {
	if sm.pool == nil {
		return false
	}

	pool, ok := sm.pool.(ConnectionPoolInterface)
	if !ok {
		return false
	}

	conn := pool.GetConnection(session.Key)
	if conn == nil || conn.Settings == nil {
		return false
	}

	rfidData, ok := session.Data["rfid"].(map[string]interface{})
	if !ok {
		return false
	}

	// Check auth
	auth, _ := rfidData["auth"].(bool)
	if !auth {
		session.Data["error_message"] = "Метка не прочитана"
		session.Data["result"] = 0
		session.Data["message"] = "Метка не прочитана"
		session.Stage = types.SESSION_STAGE_LAST_ANSWER
		return true
	}

	// Check deny_lockers
	if conn.Settings.DenyLockers {
		if lockersDataF, ok := rfidData["lockers_data_f"].([]string); ok && len(lockersDataF) > 0 {
			msg := "Сдайте шкафы:"
			// Take first 9 items (3 chunks of 3)
			chunks := len(lockersDataF) / 3
			if chunks > 3 {
				chunks = 3
			}
			for i := 0; i < chunks; i++ {
				start := i * 3
				end := start + 3
				if end > len(lockersDataF) {
					end = len(lockersDataF)
				}
				chunk := lockersDataF[start:end]
				msg += " " + fmt.Sprintf("%v", chunk)
			}
			session.Data["error_message"] = msg
			session.Data["result"] = 0
			session.Data["message"] = msg
			session.Stage = types.SESSION_STAGE_LAST_ANSWER
			return true
		}
	}

	// Check deny_ct (temporary cards)
	if conn.Settings.DenyCT {
		if tempCard, ok := rfidData["temp_card"].(bool); ok && tempCard {
			msg := "Это карта\nдля\nкартоприемника"
			session.Data["error_message"] = msg
			session.Data["result"] = 0
			session.Data["message"] = msg
			session.Stage = types.SESSION_STAGE_LAST_ANSWER
			return true
		}
	}

	return false
}

// sendDenyMessage sends deny message to terminal
func (sm *SessionManager) sendDenyMessage(session *types.Session, message string) {
	if sm.pool == nil {
		return
	}

	pool, ok := sm.pool.(ConnectionPoolInterface)
	if !ok {
		return
	}

	// Close relay
	pool.SendJSPRelayClose(session.Key)

	// Send deny message
	pool.SendJSPMessage(session.Key, message, 1500)
}

// processDone processes session completion
func (sm *SessionManager) processDone(session *types.Session) error {
	if !session.ReportSent {
		// TODO: Send final report
		session.ReportSent = true
	}

	// Log session to CSV
	if sm.csvLogger != nil {
		var conn *types.Connection
		if pool, ok := sm.pool.(ConnectionPoolInterface); ok {
			conn = pool.GetConnection(session.Key)
		}
		if err := sm.csvLogger.LogSession(session, conn); err != nil {
			// Log error but don't fail session completion
			fmt.Printf("Failed to log session to CSV: %v\n", err)
		}
	}

	session.Processed = true
	session.Completed = true

	return nil
}

// StartSession starts new access session from tag/card read
func (sm *SessionManager) StartSession(uid string, key string, apkey string, lockers []types.LockerInfo) (*types.Session, error) {
	session, err := sm.CreateSession(uid, key, apkey)
	if err != nil {
		return nil, err
	}

	// Initialize session data
	session.Data = make(map[string]interface{})

	// Store RFID data including lockers
	rfidData := map[string]interface{}{
		"uid": uid,
	}
	if len(lockers) > 0 {
		rfidData["lockers_data"] = lockers
		// Process lockers data for formatted output
		lockersDataArray, lockersDataF := utils.ProcessLockersData(lockers)
		rfidData["lockers_data"] = lockersDataArray
		rfidData["lockers_data_f"] = lockersDataF
	}
	session.Data["rfid"] = rfidData

	// Check deny conditions before starting KPO
	if sm.checkTagReadDeny(session) {
		return session, nil
	}

	// Lock terminal while processing
	if sm.pool != nil {
		if pool, ok := sm.pool.(ConnectionPoolInterface); ok {
			// Lock with default message
			pool.LockTerminal(session.Key, session.ID, "Ожидание...")
		}
	}

	// Start KPO request
	session, err = sm.sendKpoRequest(session)
	if err != nil {
		return session, err
	}

	// Set wait for KPO result
	sm.Wait(session, 0x01, types.SESSION_STAGE_KPO_RESULT, 0, nil) // SESSION_PROC_KPO

	return session, nil
}

// sendKpoRequest sends access request to 1C/KPO
func (sm *SessionManager) sendKpoRequest(session *types.Session) (*types.Session, error) {
	// Set KPO request start time
	session.Data["kpo"] = map[string]interface{}{
		"result":      types.KPO_RES_UNDEF,
		"start_time":  time.Now(),
		"expire_time": time.Now().Add(time.Duration(sm.config.ServiceRequestExpireTime) * time.Second),
	}

	session.ReqTime = time.Now()
	session.Stage = types.SESSION_STAGE_KPO_RESULT

	// Send real HTTP request to 1C
	go func() {
		if sm.httpClient != nil {
			// Extract terminal ID from connection key
			terminalID := sm.extractTerminalID(session.Key)

			// Get lockers data from session
			var lockers []types.LockerInfo
			if rfidData, ok := session.Data["rfid"].(map[string]interface{}); ok {
				if lockersData, ok := rfidData["lockers_data"].([]types.LockerInfo); ok {
					lockers = lockersData
				} else if lockersDataInterface, ok := rfidData["lockers_data"].([]interface{}); ok {
					// Convert []interface{} to []types.LockerInfo
					lockers = make([]types.LockerInfo, 0, len(lockersDataInterface))
					for _, item := range lockersDataInterface {
						if locker, ok := item.(types.LockerInfo); ok {
							lockers = append(lockers, locker)
						} else if lockerMap, ok := item.(map[string]interface{}); ok {
							// Convert map to LockerInfo
							locker := types.LockerInfo{}
							if authErr, ok := lockerMap["auth_err"].(uint8); ok {
								locker.AuthErr = authErr
							}
							if readErr, ok := lockerMap["read_err"].(uint8); ok {
								locker.ReadErr = readErr
							}
							if isPasstech, ok := lockerMap["is_passtech"].(bool); ok {
								locker.IsPasstech = isPasstech
							}
							if blockNo, ok := lockerMap["block_no"].(uint8); ok {
								locker.BlockNo = blockNo
							}
							if litera, ok := lockerMap["litera"].(string); ok {
								locker.Litera = litera
							}
							if locked, ok := lockerMap["locked"].(bool); ok {
								locker.Locked = locked
							}
							if cabNo, ok := lockerMap["cab_no"].(uint16); ok {
								locker.CabNo = cabNo
							}
							lockers = append(lockers, locker)
						}
					}
				}
			}

			result, message, err := sm.httpClient.CheckAccess(session.UID, terminalID, "rfid", lockers)
			if err != nil {
				fmt.Printf("KPO request failed for session %s: %v\n", session.ID, err)

				// Graceful degradation: use autofix if enabled
				if sm.config.ServiceAutofixExpired {
					sm.setKpoResult(session.ID, types.KPO_RES_YES, sm.config.ServiceFixedMsg)
					fmt.Printf("Using autofix for session %s due to HTTP error\n", session.ID)
				} else {
					sm.setKpoResult(session.ID, types.KPO_RES_NO, sm.config.ServiceLinkErrMsg)
				}
				return
			}

			sm.setKpoResult(session.ID, *result, message)

			// Try to get user CID
			if cid, err := sm.httpClient.GetUserCID(session.UID); err == nil {
				session.CID = cid
			}
		} else {
			// No HTTP client, simulate success
			time.Sleep(100 * time.Millisecond)
			sm.setKpoResult(session.ID, types.KPO_RES_YES, "Проходите")
		}
	}()

	return session, nil
}

// extractTerminalID extracts terminal ID from connection key
func (sm *SessionManager) extractTerminalID(connKey string) string {
	// Connection key format: "ip:port"
	// We need to extract just IP or use a mapping
	// For now, return the key as terminal ID
	return connKey
}

// setKpoResult sets KPO verification result
func (sm *SessionManager) setKpoResult(sessionID string, result types.KPOResult, message string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	kpoData, ok := session.Data["kpo"].(map[string]interface{})
	if !ok {
		kpoData = make(map[string]interface{})
		session.Data["kpo"] = kpoData
	}

	kpoData["result"] = result
	kpoData["message"] = message
	kpoData["end_time"] = time.Now()

	return nil
}

// HandlePassEvent handles person passed event
func (sm *SessionManager) HandlePassEvent(sessionID string, gateNumber int) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	switch gateNumber {
	case 1:
		session.Data["passed_first"] = true
	case 2:
		session.Data["passed_second"] = true
	}

	return nil
}

// HandleCameraResult handles camera recognition result
func (sm *SessionManager) HandleCameraResult(sessionID string, result types.CamResult, message string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	camData, ok := session.Data["cam"].(map[string]interface{})
	if !ok {
		camData = make(map[string]interface{})
		session.Data["cam"] = camData
	}

	camData["result"] = result
	camData["message"] = message

	return nil
}

// CreateSession creates new session
func (sm *SessionManager) CreateSession(uid string, key string, apkey string) (*types.Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sessionID := sm.generateSessionID()

	session := &types.Session{
		ID:        sessionID,
		Key:       key,
		Apkey:     apkey,
		UID:       uid,
		UIDRaw:    uid,
		Data:      make(map[string]interface{}),
		Stage:     types.SESSION_STAGE_INIT,
		ReqTime:   time.Now(),
		Processed: false,
		Completed: false,
		Alive:     true,
	}

	sm.sessions[sessionID] = session

	return session, nil
}

// GetSession gets session by ID
func (sm *SessionManager) GetSession(sessionID string) *types.Session {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.sessions[sessionID]
}

// GetAllSessions returns all sessions
func (sm *SessionManager) GetAllSessions() []*types.Session {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	result := make([]*types.Session, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		result = append(result, session)
	}
	return result
}

// GetPool returns connection pool interface
func (sm *SessionManager) GetPool() interface{} {
	return sm.pool
}

// UpdateSession updates session
func (sm *SessionManager) UpdateSession(sessionID string, updates map[string]interface{}) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Apply updates
	for key, value := range updates {
		switch key {
		case "stage":
			if stage, ok := value.(types.SessionStage); ok {
				session.Stage = stage
			}
		case "data":
			if data, ok := value.(map[string]interface{}); ok {
				for k, v := range data {
					session.Data[k] = v
				}
			}
		case "cid":
			if cid, ok := value.(string); ok {
				session.CID = cid
			}
		case "processed":
			if processed, ok := value.(bool); ok {
				session.Processed = processed
			}
		case "completed":
			if completed, ok := value.(bool); ok {
				session.Completed = completed
			}
		case "alive":
			if alive, ok := value.(bool); ok {
				session.Alive = alive
			}
		}
	}

	return nil
}

// DeleteSession deletes session
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	delete(sm.sessions, sessionID)
}

// GetActiveSessions returns active sessions
func (sm *SessionManager) GetActiveSessions() []*types.Session {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var active []*types.Session
	for _, session := range sm.sessions {
		if session.Alive && !session.Completed {
			active = append(active, session)
		}
	}

	return active
}

// ProcessSession processes session state machine
func (sm *SessionManager) ProcessSession(sessionID string) error {
	session := sm.GetSession(sessionID)
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	switch session.Stage {
	case types.SESSION_STAGE_INIT:
		// Initialize session - send to KPO processing
		return sm.processKPO(session)

	case types.SESSION_STAGE_KPO_RESULT:
		// KPO result received - check if access granted
		if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
			if result, ok := kpoData["result"].(types.KPOResult); ok {
				// Try to get CID for camera check
				if session.CID == "" && sm.httpClient != nil {
					if cid, err := sm.httpClient.GetUserCID(session.UID); err == nil && cid != "" {
						session.CID = cid
					}
				}

				if result == types.KPO_RES_YES {
					// Access granted
					// Check if we have gate (gpkey) - for now, assume no gate if not specified
					hasGate := sm.hasGate(session)

					if hasGate {
						// Has gate - proceed to open first door
						session.Stage = types.SESSION_STAGE_OPEN_FIRST
						return sm.processAccess(session)
					} else if sm.config.CamServiceActive && session.CID != "" && sm.shouldCheckCamera(session) {
						// No gate but has camera - check camera
						return sm.processCameraCheck(session)
					} else {
						// No gate and no camera - go to kpo_direct
						session.Stage = types.SESSION_STAGE_KPO_DIRECT
						return nil
					}
				} else {
					// Access denied - go to kpo_direct
					session.Stage = types.SESSION_STAGE_KPO_DIRECT
					return nil
				}
			}
		}

	case types.SESSION_STAGE_KPO_DIRECT:
		// KPO direct - show result without gate
		return sm.processKpoDirect(session)

	case types.SESSION_STAGE_CAM_RESULT:
		// Camera result received
		if result, ok := session.Data["cam_result"].(types.CamResult); ok {
			if result == types.CAM_RES_YES || sm.config.CamAlwaysPass {
				// Camera OK - proceed to access
				return sm.processAccess(session)
			} else {
				// Camera failed
				session.Stage = types.SESSION_STAGE_LAST_ANSWER
				return sm.sendDenyResponse(session)
			}
		}

	case types.SESSION_STAGE_OPEN_FIRST:
		// First door opened
		return sm.waitForFirstPass(session)

	case types.SESSION_STAGE_FIRST_PASSED:
		// First passage completed - check camera again if needed
		if passed, ok := session.Data["passed_first"].(bool); ok && passed {
			if sm.shouldCheckCamera(session) {
				return sm.processCameraCheck(session)
			} else {
				return sm.processSecondAccess(session)
			}
		}

	case types.SESSION_STAGE_OPEN_SECOND:
		// Second door should be opened
		return sm.waitForSecondPass(session)

	case types.SESSION_STAGE_SECOND_PASSED:
		// Second passage completed
		session.Stage = types.SESSION_STAGE_PASSED

	case types.SESSION_STAGE_PASSED:
		// Access completed successfully
		return sm.finalizeSession(session)

	case types.SESSION_STAGE_LAST_ANSWER:
		// Final response sent
		session.Completed = true
	}

	return nil
}

// processKPO sends request to KPO (1C/Database)
func (sm *SessionManager) processKPO(session *types.Session) error {
	// This would integrate with HTTP client
	// For now, simulate KPO check
	session.Data["kpo_result"] = types.KPO_RES_YES
	session.Data["kpo_message"] = "Access granted"
	session.Stage = types.SESSION_STAGE_KPO_RESULT

	return nil
}

// processCameraCheck sends request to camera service (Helios)
func (sm *SessionManager) processCameraCheck(session *types.Session) error {
	if !sm.config.CamServiceActive {
		session.Data["cam_result"] = types.CAM_RES_YES
		session.Stage = types.SESSION_STAGE_OPEN_FIRST
		return nil
	}

	// Get CID (person ID) for Helios
	personID := session.CID
	if personID == "" {
		// Try to get from HTTP client
		if sm.httpClient != nil {
			if cid, err := sm.httpClient.GetUserCID(session.UID); err == nil && cid != "" {
				personID = cid
				session.CID = cid
			}
		}
	}

	if personID == "" {
		// No CID available, skip camera check
		session.Data["cam_result"] = types.CAM_RES_YES
		session.Stage = types.SESSION_STAGE_OPEN_FIRST
		return nil
	}

	// Initialize camera data
	if session.Data["cam"] == nil {
		session.Data["cam"] = make(map[string]interface{})
	}
	camData := session.Data["cam"].(map[string]interface{})
	camData["result"] = types.CAM_RES_UNDEF
	camData["start_time"] = time.Now()

	// Get camera PID from connection or use default
	camPID := "default"
	if sm.pool != nil {
		if pool, ok := sm.pool.(ConnectionPoolInterface); ok {
			if conn := pool.GetConnection(session.Key); conn != nil && conn.Settings != nil {
				// Try to get cam_pid from settings
				if extra := conn.Settings.Extra; extra != nil {
					if pid, ok := extra["cam_pid"].(string); ok && pid != "" {
						camPID = pid
					}
				}
			}
		}
	}

	// Start Helios verification
	if sm.heliosClient != nil {
		requestID, err := sm.heliosClient.StartVerification(session.ID, camPID, personID)
		if err != nil {
			// On error, use autofix if enabled
			if sm.config.ServiceAutofixExpired {
				camData["result"] = types.CAM_RES_YES
				session.Stage = types.SESSION_STAGE_OPEN_FIRST
			} else {
				camData["result"] = types.CAM_RES_FAIL
				session.Data["result"] = 0
				session.Data["message"] = sm.config.CamServiceResultMsgFail
				session.Stage = types.SESSION_STAGE_LAST_ANSWER
			}
			return err
		}

		// Store request ID in session
		camData["rkey"] = requestID
		session.Stage = types.SESSION_STAGE_CAM_RESULT
	} else {
		// No Helios client, skip camera check
		camData["result"] = types.CAM_RES_YES
		session.Stage = types.SESSION_STAGE_OPEN_FIRST
	}

	return nil
}

// SetHeliosClient sets Helios client for session manager
func (sm *SessionManager) SetHeliosClient(client HeliosClientInterface) {
	sm.heliosClient = client
}

// processAccess processes access granting
func (sm *SessionManager) processAccess(session *types.Session) error {
	session.Stage = types.SESSION_STAGE_OPEN_FIRST
	return sm.sendAccessResponse(session, true, "Access granted")
}

// waitForFirstPass waits for first passage
func (sm *SessionManager) waitForFirstPass(session *types.Session) error {
	// Check if passed event occurred
	if passed, ok := session.Data["passed"].(map[string]interface{}); ok {
		if passedVal, ok := passed["passed"].(bool); ok && passedVal {
			session.Data["passed_first"] = true
			session.Stage = types.SESSION_STAGE_FIRST_PASSED

			// Clear passed data
			delete(session.Data, "passed")

			// Check if need camera after first pass
			if sm.shouldCheckCamera(session) {
				return sm.processCameraCheck(session)
			} else {
				return sm.processSecondAccess(session)
			}
		}
	}
	// Still waiting
	return nil
}

// processSecondAccess processes second access
func (sm *SessionManager) processSecondAccess(session *types.Session) error {
	session.Stage = types.SESSION_STAGE_OPEN_SECOND
	return sm.sendAccessResponse(session, true, "Proceed to second door")
}

// waitForSecondPass waits for second passage
func (sm *SessionManager) waitForSecondPass(session *types.Session) error {
	// Check if passed event occurred
	if passed, ok := session.Data["passed"].(map[string]interface{}); ok {
		if passedVal, ok := passed["passed"].(bool); ok && passedVal {
			session.Data["passed_second"] = true
			session.Stage = types.SESSION_STAGE_SECOND_PASSED

			// Clear passed data
			delete(session.Data, "passed")
		}
	}
	// Still waiting or completed
	return nil
}

// finalizeSession finalizes successful session
func (sm *SessionManager) finalizeSession(session *types.Session) error {
	session.Processed = true
	session.Completed = true
	session.ReportSent = true

	// Log successful access
	fmt.Printf("Session completed: %s, UID: %s\n", session.ID, session.UID)

	return nil
}

// sendAccessResponse sends access granted response
func (sm *SessionManager) sendAccessResponse(session *types.Session, granted bool, message string) error {
	session.Data["result"] = granted
	session.Data["message"] = message
	session.Data["result_time"] = time.Now()

	// This would send response to terminal
	fmt.Printf("Access response: %s - %s\n", session.ID, message)

	return nil
}

// sendDenyResponse sends access denied response
func (sm *SessionManager) sendDenyResponse(session *types.Session) error {
	return sm.sendAccessResponse(session, false, "Access denied")
}

// shouldCheckCamera checks if camera verification is needed
func (sm *SessionManager) shouldCheckCamera(session *types.Session) bool {
	return sm.config.CamServiceActive && session.CID != ""
}

// generateSessionID generates unique session ID
func (sm *SessionManager) generateSessionID() string {
	sm.idGen++
	return fmt.Sprintf("session_%d_%d", time.Now().Unix(), sm.idGen)
}

// CleanupExpiredSessions removes expired sessions
func (sm *SessionManager) CleanupExpiredSessions() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	expired := make([]string, 0)

	for id, session := range sm.sessions {
		if now.Sub(session.ReqTime) > time.Duration(sm.config.SessionExpireTime*float64(time.Second)) {
			expired = append(expired, id)
		}
	}

	for _, id := range expired {
		delete(sm.sessions, id)
	}

	if len(expired) > 0 {
		fmt.Printf("Cleaned up %d expired sessions\n", len(expired))
	}
}

// GetSessionStats returns session statistics
func (sm *SessionManager) GetSessionStats() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := map[string]interface{}{
		"total_sessions":     len(sm.sessions),
		"active_sessions":    0,
		"completed_sessions": 0,
		"current_time":       time.Now().Unix(),
	}

	for _, session := range sm.sessions {
		if session.Alive && !session.Completed {
			stats["active_sessions"] = stats["active_sessions"].(int) + 1
		}
		if session.Completed {
			stats["completed_sessions"] = stats["completed_sessions"].(int) + 1
		}
	}

	return stats
}

// Wait starts waiting for a process to complete
func (sm *SessionManager) Wait(session *types.Session, procType int, dstStage types.SessionStage, timeout float64, params map[string]interface{}) error {
	if session.Processed || session.Completed {
		return fmt.Errorf("session already processed")
	}

	// Calculate expire time
	expireTime := time.Now()
	if timeout <= 0 {
		// Use default timeout based on proc type
		switch procType {
		case 0x01: // SESSION_PROC_KPO
			timeout = sm.config.ServiceRequestExpireTime
		case 0x02: // SESSION_PROC_CAM
			timeout = sm.config.ServiceRequestExpireTime
		case 0x03: // SESSION_PROC_PASS
			// Pass time + additional expire time
			timeout = 5.0 // Default pass time
			// Note: TermPassAddExpireTime would be added here if available in config
		default:
			timeout = 5.0
		}
	}
	expireTime = expireTime.Add(time.Duration(timeout * float64(time.Second)))

	session.Wait = &types.SessionWait{
		ProcType:   procType,
		ExpireTime: expireTime,
		DstStage:   dstStage,
		Params:     params,
	}

	return nil
}

// checkWait checks if wait condition is met or expired
func (sm *SessionManager) checkWait(session *types.Session) bool {
	if session.Wait == nil {
		return true
	}

	now := time.Now()

	switch session.Wait.ProcType {
	case 0x01: // SESSION_PROC_KPO
		// Check if KPO result is available
		if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
			if result, ok := kpoData["result"].(types.KPOResult); ok && result != types.KPO_RES_UNDEF {
				// Result available - wait done
				sm.waitDone(session)
				return true
			}
		}
		// Check timeout
		if now.After(session.Wait.ExpireTime) {
			// Timeout - use autofix or deny
			if sm.config.ServiceAutofixExpired {
				sm.setKpoResult(session.ID, types.KPO_RES_YES, sm.config.ServiceFixedMsg)
			} else {
				sm.setKpoResult(session.ID, types.KPO_RES_NO, sm.config.ServiceLinkErrMsg)
			}
			sm.waitDone(session)
			return true
		}
		return false

	case 0x02: // SESSION_PROC_CAM
		// Check if CAM result is available
		if camData, ok := session.Data["cam"].(map[string]interface{}); ok {
			if result, ok := camData["result"].(types.CamResult); ok && result != types.CAM_RES_UNDEF {
				// Result available - wait done
				sm.waitDone(session)
				return true
			}
		}
		// Check timeout
		if now.After(session.Wait.ExpireTime) {
			// Timeout - set CAM result to NO
			if camData, ok := session.Data["cam"].(map[string]interface{}); ok {
				camData["result"] = types.CAM_RES_NO
				camData["answer_data"] = map[string]interface{}{"error": "idle_timeout"}
			}
			sm.waitDone(session)
			return true
		}
		return false

	case 0x03: // SESSION_PROC_PASS
		// Check if pass event occurred
		if passed, ok := session.Data["passed"].(map[string]interface{}); ok {
			waitKey, _ := session.Wait.Params["key"].(string)
			passedKey, _ := passed["key"].(string)
			if waitKey == "" || waitKey == passedKey {
				// Pass event occurred - wait done
				sm.waitDone(session)
				return true
			}
		}
		// Check timeout
		if now.After(session.Wait.ExpireTime) {
			// Timeout - set passed to false
			waitKey, _ := session.Wait.Params["key"].(string)
			session.Data["passed"] = map[string]interface{}{
				"passed": false,
				"time":   now,
				"tmo":    true,
				"key":    waitKey,
			}
			sm.waitDone(session)
			return true
		}
		return false

	default:
		// Unknown proc type - clear wait
		sm.waitDone(session)
		return true
	}
}

// waitDone completes wait and transitions to destination stage
func (sm *SessionManager) waitDone(session *types.Session) {
	if session.Wait == nil {
		return
	}

	dstStage := session.Wait.DstStage
	session.Wait = nil

	if dstStage != 0 {
		session.Stage = dstStage
	}
}

// hasGate checks if session has gate (gpkey) - simplified check
func (sm *SessionManager) hasGate(session *types.Session) bool {
	// In PHP, this checks for gpkey from cgate_get_op_conn
	// For now, we'll check if connection has gate settings
	if sm.pool != nil {
		if pool, ok := sm.pool.(ConnectionPoolInterface); ok {
			if conn := pool.GetConnection(session.Key); conn != nil && conn.Settings != nil {
				// Check if there's a gate configured
				// This is simplified - in production would check for actual gate connection
				return false // Assume no gate for now
			}
		}
	}
	return false
}

// processKpoDirect processes KPO direct stage (no gate, show result directly)
func (sm *SessionManager) processKpoDirect(session *types.Session) error {
	if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
		if result, ok := kpoData["result"].(types.KPOResult); ok {
			message := ""
			if msg, ok := kpoData["message"].(string); ok {
				message = msg
			}

			if result == types.KPO_RES_YES {
				// Access granted - show allow message
				session.Data["result"] = 1
				session.Data["message"] = message
				session.Stage = types.SESSION_STAGE_LAST_ANSWER
				sm.sendAllowMessage(session, message)
			} else if result == types.KPO_RES_NO {
				// Access denied - show deny message
				session.Data["result"] = 0
				session.Data["message"] = message
				session.Stage = types.SESSION_STAGE_LAST_ANSWER
				sm.sendDenyMessage(session, message)
			} else {
				// Error - show error message
				session.Data["result"] = 0
				session.Data["message"] = message
				session.Stage = types.SESSION_STAGE_LAST_ANSWER
				sm.sendDenyMessage(session, message)
			}
		} else {
			// Wait for result
			return nil
		}
	}
	return nil
}
