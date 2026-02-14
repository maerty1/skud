package daemon

import (
	"encoding/json"
	"fmt"
	"nd-go/config"
	"nd-go/internal/cardlist"
	"nd-go/internal/connection"
	"nd-go/internal/crt"
	"nd-go/internal/csvlogger"
	"nd-go/internal/gtime"
	"nd-go/internal/handler"
	"nd-go/internal/helios"
	"nd-go/internal/httpclient"
	"nd-go/internal/logging"
	"nd-go/internal/protocols/gat"
	"nd-go/internal/protocols/jsp"
	"nd-go/internal/protocols/pocket"
	"nd-go/internal/protocols/sphinx"
	"nd-go/internal/session"
	"nd-go/internal/storage"
	"nd-go/internal/termlogs"
	"nd-go/internal/email"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Daemon represents main application daemon
type Daemon struct {
	config       *types.Config
	pool         *connection.ConnectionPool
	logger       *logging.Logger
	handlers     *handler.HandlerManager
	httpClient   *httpclient.HTTPClient
	heliosClient *helios.HeliosClient
	crtClient    *crt.CRTClient
	cardList     *cardlist.CardList
	gtimeLogger  *gtime.GTimeLogger
	termLogs     *termlogs.TermLogs
	sessionMgr   *session.SessionManager
	csvLogger    *csvlogger.CSVLogger
	storageStore *storage.SQLiteStore
	running      bool
	mutex        sync.RWMutex
	server       *net.TCPListener
	webServer    *http.Server
	shutdownCh   chan bool
	startTime    time.Time
	eventCh      chan map[string]interface{} // Канал для real-time событий
}

// NewDaemon creates new daemon instance with default config
func NewDaemon() *Daemon {
	return NewDaemonWithConfig("", nil)
}

// NewDaemonWithConfig creates new daemon instance with custom config
func NewDaemonWithConfig(configPath string, cmdArgs map[string]string) *Daemon {
	fmt.Println("Loading config...")
	cfg := config.LoadConfig(configPath, cmdArgs)
	if cfg == nil {
		fmt.Println("ERROR: Config is nil!")
		return nil
	}
	fmt.Println("Config loaded")

	fmt.Println("Creating logger...")
	logger := logging.NewLogger(cfg)
	fmt.Println("Logger created")

	fmt.Println("Creating connection pool...")
	pool := connection.NewConnectionPool(cfg)
	fmt.Println("Connection pool created")

	fmt.Println("Setting up event handlers...")
	// Event handlers will be set later when daemon is created
	fmt.Println("Event handlers set")

	fmt.Println("Creating handlers...")
	handlers := handler.NewHandlerManager()
	fmt.Println("Handlers created")

	fmt.Println("Creating HTTP client...")
	httpClient := httpclient.NewHTTPClient(cfg)
	fmt.Println("HTTP client created")

	fmt.Println("Creating session manager...")
	sessionMgr := session.NewSessionManager(cfg)
	sessionMgr.SetHTTPClient(httpClient)
	sessionMgr.SetPool(pool)
	fmt.Println("Session manager created")

	var csvLogger *csvlogger.CSVLogger
	var storageStore *storage.SQLiteStore
	if cfg.StorageSqlitePath != "" {
		fmt.Println("Creating SQLite storage...")
		storageStore = storage.NewSQLiteStore(cfg.StorageSqlitePath)
		if err := storageStore.Open(); err != nil {
			fmt.Printf("Warning: SQLite storage open failed: %v, falling back to CSV\n", err)
			storageStore = nil
			csvLogger = csvlogger.NewCSVLogger("./csv/")
			sessionMgr.SetCSVLogger(csvLogger)
		} else {
			sessionMgr.SetCSVLogger(storageStore)
			fmt.Println("SQLite storage created and set")
		}
	} else {
		fmt.Println("Creating CSV logger...")
		csvLogger = csvlogger.NewCSVLogger("./csv/")
		sessionMgr.SetCSVLogger(csvLogger)
		fmt.Println("CSV logger created and set")
	}

	fmt.Println("Creating Helios client...")
	heliosClient := helios.NewHeliosClient(cfg)
	heliosClient.SetEventCallback(func(request *helios.HeliosRequest, eventType helios.HeliosEventType, data map[string]interface{}) {
		// This will be set after daemon is created
	})
	fmt.Println("Helios client created")

	fmt.Println("Creating CRT client...")
	crtClient := crt.NewCRTClient(cfg)
	fmt.Println("CRT client created")

	fmt.Println("Creating term logs...")
	termLogsStore := termlogs.NewTermLogs(cfg.LogEventCount)
	fmt.Println("Term logs created")

	var gtimeLogger *gtime.GTimeLogger
	if storageStore == nil {
		fmt.Println("Creating GTime logger (CSV)...")
		gtimeLogger = gtime.NewGTimeLogger(cfg.LogFile+"_gtime/", gtime.DefaultKeys())
		fmt.Println("GTime logger created")
	}

	fmt.Println("Creating card list...")
	cardListMgr := cardlist.NewCardList()
	cardListMgr.SetPersistFile("cardlist.json")
	if err := cardListMgr.Load(); err != nil {
		fmt.Printf("Warning: failed to load card list: %v\n", err)
	}
	fmt.Println("Card list created")

	daemon := &Daemon{
		config:       cfg,
		pool:         pool,
		logger:       logger,
		handlers:     handlers,
		httpClient:   httpClient,
		heliosClient: heliosClient,
		crtClient:    crtClient,
		cardList:     cardListMgr,
		gtimeLogger:  gtimeLogger,
		termLogs:     termLogsStore,
		sessionMgr:   sessionMgr,
		csvLogger:    csvLogger,
		storageStore: storageStore,
		running:      false,
		shutdownCh:   make(chan bool),
		startTime:    time.Now(),
		eventCh:      make(chan map[string]interface{}, 100),
	}

	// Set event handlers for connection pool
	pool.SetEventHandlers(daemon.ProcessTagRead, daemon.ProcessPassEvent)
	pool.SetBarcodeHandler(daemon.ProcessBarcodeRead)

	// Set Helios event callback and client
	heliosClient.SetEventCallback(daemon.handleHeliosEvent)
	sessionMgr.SetHeliosClient(heliosClient)

	// Set CRT event callback and initialize
	crtClient.SetEventCallback(daemon.handleCRTIdentification)
	if cfg.CRTServiceActive {
		if crtClient.Init() {
			fmt.Println("CRT (Vizir) service initialized")
		} else {
			fmt.Println("CRT (Vizir) service initialization failed")
		}
	}

	return daemon
}

// Start starts the daemon
func (d *Daemon) Start() error {
	fmt.Println("Start() called")
	d.mutex.Lock()
	d.running = true
	d.mutex.Unlock()

	fmt.Println("Запуск СКД (Система контроля доступа)...")
	d.logger.Info("Запуск СКД (Система контроля доступа)...")

	// Initialize JSP protocol
	jsp.InitMtf(utils.GetMtf)

	// Initialize handlers
	d.initHandlers()

	// Start TCP server for commands
	if err := d.startServer(); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	d.logger.Info(fmt.Sprintf("Server started on %s:%d", d.config.ServerAddr, d.config.ServerPort))

	// Start web server if enabled
	if d.config.WebEnabled {
		if err := d.startWebServer(); err != nil {
			return fmt.Errorf("failed to start web server: %v", err)
		}
		d.logger.Info(fmt.Sprintf("Web server started on %s:%d", d.config.WebAddr, d.config.WebPort))
	}

	// Handle signals
	go d.handleSignals()

	// Main loop
	d.mainLoop()

	return nil
}

// Stop stops the daemon
func (d *Daemon) Stop() {
	d.mutex.Lock()
	d.running = false
	d.mutex.Unlock()

	d.logger.Info("Остановка СКД (Система контроля доступа)...")

	if d.server != nil {
		d.server.Close()
	}

	if d.webServer != nil {
		d.webServer.Close()
	}

	d.pool.Close()
	if d.storageStore != nil {
		d.storageStore.Close()
	}
	d.logger.Close()

	close(d.shutdownCh)
}

// mainLoop runs main daemon loop
func (d *Daemon) mainLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-d.shutdownCh:
			return
		case <-ticker.C:
			d.idleProc()
		}
	}
}

// idleProc performs idle processing
func (d *Daemon) idleProc() {
	// Connection pool idle processing
	d.pool.IdleProc()

	// Process HTTP requests timeouts
	d.processHTTPTimeouts()

	// Process session timeouts
	d.processSessionTimeouts()

	// Cleanup expired sessions
	d.sessionMgr.CleanupExpiredSessions()

	// Check terminal list
	d.checkTerminalList()

	// Process active sessions
	d.processSessions()

	// Process JSP auto-ping
	d.processJSPAutoPing()

	// Process POCKET auto-ping
	d.processPocketAutoPing()

	// Process GAT auto-ping
	d.processGatAutoPing()

	// Process SPHINX auto-ping
	d.processSphinxAutoPing()

	// Process CRT (Vizir) polling
	if d.crtClient != nil {
		d.crtClient.IdleProc()
	}

	// Email digest at configured times
	d.trySendEmailDigest()
}

// trySendEmailDigest sends email digest at configured times (e.g. 08:00, 20:00).
func (d *Daemon) trySendEmailDigest() {
	if !d.config.EmailEnabled || len(d.config.EmailRecipients) == 0 || d.config.EmailHost == "" {
		return
	}
	if d.storageStore == nil {
		return
	}
	now := time.Now()
	currentSlot := now.Format("15:04")
	for _, slot := range d.config.EmailSendTimes {
		if slot != currentSlot {
			continue
		}
		// Avoid sending twice in the same minute
		if now.Sub(d.config.EmailLastSent) < 2*time.Minute {
			return
		}
		// Send digest: sessions since last send (or last 24h if first time)
		since := d.config.EmailLastSent
		if since.IsZero() {
			since = now.Add(-24 * time.Hour)
		}
		rows, err := d.storageStore.GetSessionsSince(since)
		if err != nil {
			d.logger.Warn(fmt.Sprintf("Email digest: get sessions: %v", err))
			return
		}
		var body strings.Builder
		body.WriteString(fmt.Sprintf("Отчёт СКД за период с %s по %s.\n\n", since.Format("02.01.2006 15:04"), now.Format("02.01.2006 15:04")))
		body.WriteString(fmt.Sprintf("Всего событий: %d\n\n", len(rows)))
		for _, r := range rows {
			body.WriteString(fmt.Sprintf("%s | %s | %s | %s | %s | %s | %s\n",
				getStr(r, "session_time"), getStr(r, "term_id"), getStr(r, "uid"), getStr(r, "kpo_result"), getStr(r, "final_result"), getStr(r, "final_msg"), getStr(r, "term_addr")))
		}
		subject := d.config.EmailSubject
		if subject == "" {
			subject = "СКД отчёт за %s"
		}
		subject = fmt.Sprintf(subject, now.Format("02.01.2006 15:04"))
		if err := email.Send(d.config.EmailHost, d.config.EmailPort, d.config.EmailUser, d.config.EmailPassword,
			d.config.EmailFrom, d.config.EmailRecipients, subject, body.String()); err != nil {
			d.logger.Warn(fmt.Sprintf("Email digest send: %v", err))
			return
		}
		d.config.EmailLastSent = now
		d.logger.Info(fmt.Sprintf("Email digest sent to %v", d.config.EmailRecipients))
		return
	}
}

func getStr(m map[string]string, k string) string {
	if s, ok := m[k]; ok {
		return s
	}
	return ""
}

// processJSPAutoPing processes JSP auto-ping for connections
func (d *Daemon) processJSPAutoPing() {
	now := time.Now()
	connections := d.pool.GetConnections()

	for key, conn := range connections {
		if conn.Settings == nil || conn.Settings.Type != types.TTYPE_JSP {
			continue
		}

		if conn.JSPConn == nil {
			continue
		}

		jspConn := conn.JSPConn

		// Check ping timeout
		if jspConn.PingSent && jspConn.PingTimeout > 0 {
			elapsed := int(now.Sub(conn.LastActivity).Seconds())
			if elapsed >= jspConn.PingTimeout {
				d.logger.Warn(fmt.Sprintf("JSP ping timeout for %s, disconnecting", key))
				// Close connection
				conn.Conn.Close()
				conn.Connected = false
				continue
			}
		}

		// Check if we need to send ping
		if !jspConn.PingSent && jspConn.PingInterval > 0 {
			elapsed := int(now.Sub(conn.LastActivity).Seconds())
			if elapsed >= jspConn.PingInterval {
				// Send ping
				packet, err := jsp.SendPing(&conn.JSPRIDCounter)
				if err == nil {
					d.pool.Send(key, packet)
					jspConn.PingSent = true
					jspConn.PingSinceLast = 0
					d.logger.Info(fmt.Sprintf("JSP ping sent to %s", key))
				}
			}
		}
	}
}

// processPocketAutoPing processes POCKET auto-ping for connections
func (d *Daemon) processPocketAutoPing() {
	now := time.Now()
	connections := d.pool.GetConnections()

	for key, conn := range connections {
		if conn.Settings == nil || conn.Settings.Type != types.TTYPE_POCKET {
			continue
		}

		if conn.PocketPing == nil {
			continue
		}

		pocketPing := conn.PocketPing

		// Check ping timeout
		if pocketPing.PingSent && pocketPing.PingTimeout > 0 {
			elapsed := int(now.Sub(pocketPing.LastPingTime).Seconds())
			if elapsed >= pocketPing.PingTimeout {
				d.logger.Warn(fmt.Sprintf("POCKET ping timeout for %s, disconnecting", key))
				// Close connection
				conn.Conn.Close()
				conn.Connected = false
				continue
			}
		}

		// Check if we need to send ping
		if !pocketPing.PingSent && pocketPing.PingInterval > 0 {
			elapsed := int(now.Sub(conn.LastActivity).Seconds())
			if elapsed >= pocketPing.PingInterval {
				// Send Enquire packet (ping)
				packet := pocket.CreateEnquirePacket()
				if err := d.pool.Send(key, packet); err == nil {
					pocketPing.PingSent = true
					pocketPing.PingSinceLast = 0
					pocketPing.LastPingTime = now
					d.logger.Info(fmt.Sprintf("POCKET ping (Enquire) sent to %s", key))
				} else {
					d.logger.Warn(fmt.Sprintf("Failed to send POCKET ping to %s: %v", key, err))
				}
			}
		}
	}
}

// processGatAutoPing processes GAT auto-ping for connections
func (d *Daemon) processGatAutoPing() {
	now := time.Now()
	connections := d.pool.GetConnections()

	for key, conn := range connections {
		if conn.Settings == nil || conn.Settings.Type != types.TTYPE_GAT {
			continue
		}

		if conn.GatPing == nil {
			continue
		}

		gatPing := conn.GatPing

		// Check ping timeout
		if gatPing.PingSent && gatPing.PingTimeout > 0 {
			elapsed := int(now.Sub(gatPing.LastPingTime).Seconds())
			if elapsed >= gatPing.PingTimeout {
				d.logger.Warn(fmt.Sprintf("GAT ping timeout for %s, disconnecting", key))
				// Close connection
				conn.Conn.Close()
				conn.Connected = false
				continue
			}
		}

		// Check if we need to send ping
		if !gatPing.PingSent && gatPing.PingInterval > 0 {
			elapsed := int(now.Sub(conn.LastActivity).Seconds())
			if elapsed >= gatPing.PingInterval {
				// Send REQ_MASTER packet (ping)
				packet := gat.CreateReqMasterPacket(0x00, gatPing.TerminalType) // 0x00 = broadcast address
				if err := d.pool.Send(key, packet); err == nil {
					gatPing.PingSent = true
					gatPing.PingSinceLast = 0
					gatPing.LastPingTime = now
					d.logger.Info(fmt.Sprintf("GAT ping (REQ_MASTER) sent to %s", key))
				} else {
					d.logger.Warn(fmt.Sprintf("Failed to send GAT ping to %s: %v", key, err))
				}
			}
		}
	}
}

// processSphinxAutoPing processes SPHINX auto-ping for connections
func (d *Daemon) processSphinxAutoPing() {
	now := time.Now()
	connections := d.pool.GetConnections()

	for key, conn := range connections {
		if conn.Settings == nil || conn.Settings.Type != types.TTYPE_SPHINX {
			continue
		}

		if conn.SphinxPing == nil {
			continue
		}

		sphinxPing := conn.SphinxPing

		// Check ping timeout
		if sphinxPing.PingSent && sphinxPing.PingTimeout > 0 {
			elapsed := int(now.Sub(sphinxPing.LastPingTime).Seconds())
			if elapsed >= sphinxPing.PingTimeout {
				d.logger.Warn(fmt.Sprintf("SPHINX ping timeout for %s, disconnecting", key))
				// Close connection
				conn.Conn.Close()
				conn.Connected = false
				continue
			}
		}

		// Check if we need to send ping
		if !sphinxPing.PingSent && sphinxPing.PingInterval > 0 {
			elapsed := int(now.Sub(conn.LastActivity).Seconds())
			if elapsed >= sphinxPing.PingInterval {
				// Send DELEGATION_START packet (ping)
				packet := sphinx.CreateDelegationStartPacket()
				if err := d.pool.Send(key, packet); err == nil {
					sphinxPing.PingSent = true
					sphinxPing.PingSinceLast = 0
					sphinxPing.LastPingTime = now
					d.logger.Info(fmt.Sprintf("SPHINX ping (DELEGATION_START) sent to %s", key))
				} else {
					d.logger.Warn(fmt.Sprintf("Failed to send SPHINX ping to %s: %v", key, err))
				}
			}
		}
	}
}

// processHTTPTimeouts processes HTTP request timeouts
func (d *Daemon) processHTTPTimeouts() {
	now := time.Now()
	timeout := time.Duration(d.config.ServiceRequestExpireTime * float64(time.Second))

	d.mutex.Lock()
	for key, req := range d.config.HTTPRequests {
		if !req.Processed && !req.Completed && now.Sub(req.Time) > timeout {
			d.logger.Warn(fmt.Sprintf("HTTP request timeout: %s", key))
			delete(d.config.HTTPRequests, key)
		}
	}
	d.mutex.Unlock()
}

// processSessionTimeouts processes session timeouts
func (d *Daemon) processSessionTimeouts() {
	now := time.Now()
	timeout := time.Duration(d.config.SessionExpireTime * float64(time.Second))

	d.mutex.Lock()
	for key, session := range d.config.Sessions {
		if !session.Processed && !session.Completed && now.Sub(session.ReqTime) > timeout {
			d.logger.Warn(fmt.Sprintf("Session timeout: %s", key))
			delete(d.config.Sessions, key)
		}
	}
	d.mutex.Unlock()
}

// checkTerminalList checks terminal list periodically
func (d *Daemon) checkTerminalList() {
	now := time.Now()
	checkInterval := time.Duration(d.config.TermListCheckTime * float64(time.Second))

	if d.config.TermListCheckTime > 0 && now.Sub(d.config.TermListLastCheck) > checkInterval {
		d.logger.Info("Checking terminal list...")
		d.config.TermListLastCheck = now

		// Request terminal list from 1C service
		if d.config.HTTPServiceActive {
			terminals, err := d.httpClient.GetTerminalList()
			if err != nil {
				d.logger.Error(fmt.Sprintf("Failed to get terminal list: %v", err))
				return
			}

			d.logger.Info(fmt.Sprintf("Received %d terminals from 1C", len(terminals)))

			// Filter terminals by IP if filter is configured
			filteredCount := 0
			filteredTerminals := make([]map[string]interface{}, 0)
			
			d.mutex.Lock()
			for _, termData := range terminals {
				// Extract terminal ID
				termID := utils.GetStringValue(termData, "ID", utils.GetStringValue(termData, "id", ""))
				
				// Extract IP string (may contain port and parameters)
				ipStr := utils.GetStringValue(termData, "IP", utils.GetStringValue(termData, "ip", ""))
				if ipStr == "" {
					d.logger.Warn("Terminal missing IP, skipping filter check")
					continue
				}

				// Extract clean IP for filtering (first part before ':')
				ip := ipStr
				if strings.Contains(ip, ":") {
					parts := strings.Split(ip, ":")
					ip = parts[0]
				}

				// Apply filter
				if !utils.FilterTerminalList(ip, d.config.TermListFilter, d.config.TermListFilterAbsent) {
					d.logger.Debug(fmt.Sprintf("Terminal %s filtered out by IP filter", ip))
					continue
				}

				// Parse terminal settings and store for quick access
				// This will be used later in GetTerminalList
				if parsed, err := utils.ParseTerm(termID + ":" + ipStr); err == nil && parsed != nil {
					parsed.ID = termID // Ensure ID from termData is used
					termData["_parsed_settings"] = parsed
				} else if parsed, err := utils.ParseTerm(ipStr); err == nil && parsed != nil {
					parsed.ID = termID
					termData["_parsed_settings"] = parsed
				}

				// Add to filtered list
				filteredTerminals = append(filteredTerminals, termData)
				filteredCount++
			}
			
			// Save filtered terminal list FIRST (so web interface can show them immediately)
			d.config.TerminalList = filteredTerminals
			d.mutex.Unlock()

			d.logger.Info(fmt.Sprintf("Processed %d terminals after filtering (from %d total)", filteredCount, len(terminals)))
			d.logger.Info("Terminal list saved. Connections will be established in background...")

			// Process terminals in background goroutine with delay to allow web interface to show terminals first
			go func() {
				// Small delay to ensure web interface has time to display terminals
				time.Sleep(500 * time.Millisecond)
				
				for _, termData := range filteredTerminals {
					// Process each terminal in its own goroutine
					go func(term map[string]interface{}) {
						d.processTerminalFrom1C(term)
					}(termData)
					// Small delay between starting connection attempts to avoid overwhelming the system
					time.Sleep(100 * time.Millisecond)
				}
			}()
		}
	}
}

// startServer starts TCP command server
func (d *Daemon) startServer() error {
	fmt.Printf("DEBUG: Starting server on %s:%d\n", d.config.ServerAddr, d.config.ServerPort)
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", d.config.ServerAddr, d.config.ServerPort))
	if err != nil {
		return err
	}

	server, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	d.server = server

	// Start accepting connections
	go d.acceptConnections()

	return nil
}

// acceptConnections accepts incoming connections
func (d *Daemon) acceptConnections() {
	for {
		conn, err := d.server.AcceptTCP()
		if err != nil {
			if !d.running {
				break
			}
			d.logger.Error(fmt.Sprintf("Accept error: %v", err))
			continue
		}

		go d.handleConnection(conn)
	}
}

// handleConnection handles client connection
func (d *Daemon) handleConnection(conn *net.TCPConn) {
	defer conn.Close()

	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			break
		}

		if n > 0 {
			// Process command
			response := d.processCommand(buffer[:n])
			if len(response) > 0 {
				conn.Write(response)
			}
		}
	}
}

// processCommand processes incoming command
func (d *Daemon) processCommand(data []byte) []byte {
	// Convert to string and clean up
	rawCmd := string(data)

	// Log raw bytes for debugging
	d.logger.Info(fmt.Sprintf("Raw command bytes: %q (len=%d)", data, len(data)))

	// Remove non-printable characters and control characters
	cleanCmd := strings.Map(func(r rune) rune {
		if r >= 32 && r < 127 { // Keep only printable ASCII characters
			return r
		}
		return -1 // Remove character
	}, rawCmd)

	// Remove common protocol prefixes that might come from different clients
	cleanCmd = strings.TrimPrefix(cleanCmd, "*")
	cleanCmd = strings.TrimPrefix(cleanCmd, "+")

	// Try to find the actual command by looking for known command starts
	knownCommands := []string{"system", "stats", "termlist", "settings"}
	for _, cmd := range knownCommands {
		if idx := strings.Index(cleanCmd, cmd); idx >= 0 {
			cleanCmd = cleanCmd[idx:]
			break
		}
	}

	// Trim whitespace
	cleanCmd = strings.TrimSpace(cleanCmd)

	d.logger.Info(fmt.Sprintf("Received command: raw='%s' cleaned='%s'", rawCmd, cleanCmd))

	// Skip empty commands
	if cleanCmd == "" {
		d.logger.Warn("Empty command after cleaning")
		return []byte("ERROR: Empty command")
	}

	// Parse and execute command
	d.logger.Info(fmt.Sprintf("Executing command: '%s'", cleanCmd))
	result := d.handlers.Execute(cleanCmd, nil)
	d.logger.Info(fmt.Sprintf("Command result: '%s'", result))

	return []byte(result)
}

// initHandlers initializes command handlers
func (d *Daemon) initHandlers() {
	// System commands
	d.handlers.Register("system", d.handleSystemCommand)
	d.handlers.Register("termlist", d.handleTermlistCommand)
	d.handlers.Register("settings", d.handleSettingsCommand)
	d.handlers.Register("stats", d.handleStatsCommand)
}

// handleSystemCommand handles system commands
func (d *Daemon) handleSystemCommand(params map[string]interface{}) string {
	// If no specific command, show system info
	if cmd, ok := params["cmd"].(string); ok {
		switch cmd {
		case "check_db":
			// Manually trigger terminal list check
			if d.config.HTTPServiceActive {
				terminals, err := d.httpClient.GetTerminalList()
				if err != nil {
					return fmt.Sprintf("Error getting terminal list: %v", err)
				}
				result := fmt.Sprintf("Terminal list check successful. Found %d terminals:\n", len(terminals))
				for i, term := range terminals {
					id := utils.GetStringValue(term, "ID", utils.GetStringValue(term, "id", "unknown"))
					ip := utils.GetStringValue(term, "IP", utils.GetStringValue(term, "ip", "unknown"))
					port := utils.GetStringValue(term, "PORT", utils.GetStringValue(term, "port", "0"))
					result += fmt.Sprintf("  %d. ID: %s, IP: %s, Port: %s\n", i+1, id, ip, port)
				}
				return result
			}
			return "HTTP service not active"
		case "parse":
			if term, ok := params["term"].(string); ok {
				settings, err := utils.ParseTerm(term)
				if err != nil {
					return fmt.Sprintf("Error: %v", err)
				}
				return fmt.Sprintf("Parsed: %+v", settings)
			}
			return "Usage: system parse term=<terminal_string>"
		case "info":
			return fmt.Sprintf("СКД - Система контроля доступа:\n  Версия: %s\n  PID: %d\n  Время работы: %.1f секунд\n  Версия Go: %s",
				"dev", os.Getpid(),
				time.Since(d.config.Stats["start_time"].(time.Time)).Seconds(),
				"go1.25")
		}
		return fmt.Sprintf("Unknown system command: %s\nAvailable: check_db, parse, info", cmd)
	}

	// Show available system commands
	return "System commands:\n  system info - Show system information\n  system check_db - Check database connectivity\n  system parse term=<string> - Parse terminal configuration"
}

// handleTermlistCommand handles termlist commands
func (d *Daemon) handleTermlistCommand(params map[string]interface{}) string {
	if cmd, ok := params["cmd"].(string); ok {
		switch cmd {
		case "add":
			return "Terminal added"
		case "del":
			return "Terminal removed"
		case "check":
			return "Terminal check initiated"
		}
	}

	// Return active connections
	connections := d.pool.GetConnections()
	result := "Active connections:\n"
	for key, conn := range connections {
		result += fmt.Sprintf("  %s: %s:%d (%s)\n", key, conn.Addr, conn.Port, conn.Settings.Type)
	}
	return result
}

// handleSettingsCommand handles settings commands
func (d *Daemon) handleSettingsCommand(params map[string]interface{}) string {
	if cmd, ok := params["cmd"].(string); ok {
		switch cmd {
		case "get":
			if key, ok := params["key"].(string); ok {
				// Return setting value
				return fmt.Sprintf("Setting %s: not implemented", key)
			}
		case "set":
			// Set setting value
			return "Setting updated"
		}
	}

	// Return all settings
	return fmt.Sprintf("Settings: %+v", d.config)
}

// handleStatsCommand handles stats command
func (d *Daemon) handleStatsCommand(params map[string]interface{}) string {
	sessionStats := d.sessionMgr.GetSessionStats()
	stats := map[string]interface{}{
		"connections":   len(d.pool.GetConnections()),
		"reconnections": len(d.pool.GetReconnections()),
		"http_requests": len(d.config.HTTPRequests),
		"current_time":  time.Now().Unix(),
		"uptime":        time.Since(d.config.Stats["start_time"].(time.Time)).Seconds(),
	}

	// Merge session stats
	for k, v := range sessionStats {
		stats[k] = v
	}

	return fmt.Sprintf("Stats: %+v", stats)
}

// handleSignals handles OS signals
func (d *Daemon) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case sig := <-sigChan:
			d.logger.Info(fmt.Sprintf("Received signal: %v", sig))
			d.Stop()
			return
		case <-d.shutdownCh:
			return
		}
	}
}

// IsRunning returns daemon running state
func (d *Daemon) IsRunning() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.running
}

// GetConfig returns daemon configuration
func (d *Daemon) GetConfig() *types.Config {
	return d.config
}

// GetPool returns connection pool
func (d *Daemon) GetPool() *connection.ConnectionPool {
	return d.pool
}

// processTerminalFrom1C processes terminal data from 1C and creates connection
func (d *Daemon) processTerminalFrom1C(termData map[string]interface{}) {
	// Extract terminal information (try both lowercase and uppercase keys)
	id := utils.GetStringValue(termData, "ID", utils.GetStringValue(termData, "id", ""))
	if id == "" {
		d.logger.Warn("Terminal data missing 'id' or 'ID' field")
		return
	}

	ipStr := utils.GetStringValue(termData, "IP", utils.GetStringValue(termData, "ip", ""))
	if ipStr == "" {
		d.logger.Warn(fmt.Sprintf("Terminal %s missing 'ip' or 'IP' field", id))
		return
	}

	// Parse terminal string (format: "IP:PORT:type=pocket:deny_ct=true" or "IP:PORT" or just "IP")
	// ParseTerm expects format: "ID:IP:PORT:params" or "IP:PORT:params", but we have just "IP:PORT:params"
	// So we prepend ID to make it parseable, or parse manually
	var settings *types.TerminalSettings
	var err error
	
	// Try to parse with ID prefix first
	termStrWithID := id + ":" + ipStr
	if settings, err = utils.ParseTerm(termStrWithID); err != nil || settings == nil {
		// Try without ID prefix
		if settings, err = utils.ParseTerm(ipStr); err != nil || settings == nil {
			d.logger.Warn(fmt.Sprintf("Failed to parse terminal string for %s: %v", id, err))
			// Fallback: use simple IP parsing
			parts := strings.Split(ipStr, ":")
			ip := parts[0]
			if net.ParseIP(ip) == nil {
				d.logger.Warn(fmt.Sprintf("Terminal %s has invalid IP address: %s", id, ip))
				return
			}
			port := 8080
			if len(parts) > 1 {
				if portInt, err := strconv.Atoi(parts[1]); err == nil {
					port = portInt
				}
			}
			settings = &types.TerminalSettings{
				ID:           id,
				IP:           ip,
				Port:         port,
				Type:         types.TTYPE_POCKET, // Default to POCKET as in PHP
				UTF:          true,
				ConfigString: ipStr,
				Extra:        make(map[string]interface{}),
			}
			// Parse additional parameters
			for i := 2; i < len(parts); i++ {
				part := strings.TrimSpace(parts[i])
				if strings.Contains(part, "=") {
					kv := strings.SplitN(part, "=", 2)
					key := strings.ToLower(strings.TrimSpace(kv[0]))
					value := utils.ParseAval(strings.TrimSpace(kv[1]))
					settings.Extra[key] = value
					if key == "type" {
						if strVal, ok := value.(string); ok {
							settings.Type = utils.ParseTType(strVal)
						}
					}
				}
			}
		}
	}
	
	// Always use ID from termData (override parsed ID if any)
	settings.ID = id
	
	// Extract deny_ct, ctrole, apkey, memreg_* from Extra and set to settings fields
	if denyCT, ok := settings.Extra["deny_ct"]; ok {
		if boolVal, ok := denyCT.(bool); ok {
			settings.DenyCT = boolVal
		}
	}
	if ctrole, ok := settings.Extra["ctrole"].(string); ok {
		settings.CTRole = ctrole
	}
	if apkey, ok := settings.Extra["apkey"].(string); ok {
		// Store apkey in Extra, TerminalSettings doesn't have apkey field directly
		settings.Extra["apkey"] = apkey
	}
	if memregDev, ok := settings.Extra["memreg_dev"].(string); ok {
		settings.MemRegDev = memregDev
	}
	if memregDeny, ok := settings.Extra["memreg_deny"].(string); ok {
		settings.MemRegDeny = memregDeny
	}
	if memregRole, ok := settings.Extra["role"].(string); ok {
		settings.MemRegRole = memregRole
	}

	// Extract port from separate field if available (overrides parsed value)
	if portVal, ok := termData["PORT"]; ok {
		if portFloat, ok := portVal.(float64); ok {
			settings.Port = int(portFloat)
		} else if portInt, ok := portVal.(int); ok {
			settings.Port = portInt
		} else if portStr, ok := portVal.(string); ok {
			if portInt, err := strconv.Atoi(portStr); err == nil {
				settings.Port = portInt
			}
		}
	} else if portVal, ok := termData["port"]; ok {
		if portFloat, ok := portVal.(float64); ok {
			settings.Port = int(portFloat)
		} else if portInt, ok := portVal.(int); ok {
			settings.Port = portInt
		} else if portStr, ok := portVal.(string); ok {
			if portInt, err := strconv.Atoi(portStr); err == nil {
				settings.Port = portInt
			}
		}
	}

	// Override type from separate field if available
	if typeStr := utils.GetStringValue(termData, "TYPE", utils.GetStringValue(termData, "type", "")); typeStr != "" {
		settings.Type = utils.ParseTType(typeStr)
	}
	
	ip := settings.IP
	port := settings.Port
	termType := settings.Type

	// Extract additional settings from termData
	if utfVal, ok := termData["utf"].(bool); ok {
		settings.UTF = utfVal
	}
	if regQueryVal, ok := termData["reg_query"].(bool); ok {
		settings.RegQuery = regQueryVal
	}
	if configStr, ok := termData["config_string"].(string); ok && configStr != "" {
		settings.ConfigString = configStr
	}
	
	// Extract deny_ct, ctrole, apkey from parsed config string or termData
	// These are parsed from the IP string like "type=pocket:deny_ct=true:ctrole=card_taker"
	// Additional parsing can be done here if needed

	d.logger.Info(fmt.Sprintf("Processing terminal: %s (%s:%d, type: %s)", id, ip, port, termType))

	// Create connection key
	connKey := fmt.Sprintf("%s_%s_%d", termType, ip, port)

	// Check if connection already exists
	if _, exists := d.pool.GetConnections()[connKey]; exists {
		d.logger.Info(fmt.Sprintf("Connection %s already exists", connKey))
		return
	}

	// Try to connect to terminal
	errCode := 0
	errStr := ""
	key, err := d.pool.StartClient(ip, port, d.config.TerminalConnectTimeout, &errCode, &errStr)
	if err != nil {
		d.logger.Error(fmt.Sprintf("Failed to connect to terminal %s (%s:%d): %v", id, ip, port, err))
		
		// Create reconnection entry for automatic retry
		d.mutex.Lock()
		reconnKey := fmt.Sprintf("%s_%s_%d", termType, ip, port)
		reconn := &types.Reconnection{
			Key:      reconnKey,
			ConKey:   reconnKey,
			IP:       ip,
			Port:     port,
			Time:     time.Now(),
			NTime:    time.Now().Add(d.calculateReconnectionDelay(1)),
			Count:    1,
			Settings: settings, // Store settings for reconnection
		}
		d.config.Reconnections[reconnKey] = reconn
		d.mutex.Unlock()
		
		// Update terminal list with connection error info
		d.updateTerminalConnectionStatus(id, ip, port, false, err.Error())
		return
	}

	// Set terminal settings for connection
	if conn := d.pool.GetConnections()[key]; conn != nil {
		conn.Settings = settings
	}

	d.logger.Info(fmt.Sprintf("Successfully connected to terminal %s as %s", id, key))
	
	// Update terminal list with successful connection
	d.updateTerminalConnectionStatus(id, ip, port, true, "")
}

// calculateReconnectionDelay calculates delay for reconnection
func (d *Daemon) calculateReconnectionDelay(count int) time.Duration {
	delay := float64(count) * d.config.ReconnectionWaitTimeStep
	if delay > d.config.ReconnectionWaitTimeMax {
		delay = d.config.ReconnectionWaitTimeMax
	}
	if delay < 0.1 {
		delay = 0.1
	}
	return time.Duration(delay * float64(time.Second))
}

// updateTerminalConnectionStatus updates connection status in terminal list
func (d *Daemon) updateTerminalConnectionStatus(id, ip string, port int, connected bool, errorMsg string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Find terminal in list and update status
	for _, termData := range d.config.TerminalList {
		termID := utils.GetStringValue(termData, "ID", utils.GetStringValue(termData, "id", ""))
		termIP := utils.GetStringValue(termData, "IP", utils.GetStringValue(termData, "ip", ""))
		termPort := 8080
		if portVal, ok := termData["PORT"]; ok {
			if portFloat, ok := portVal.(float64); ok {
				termPort = int(portFloat)
			} else if portInt, ok := portVal.(int); ok {
				termPort = portInt
			}
		} else if portVal, ok := termData["port"]; ok {
			if portFloat, ok := portVal.(float64); ok {
				termPort = int(portFloat)
			} else if portInt, ok := portVal.(int); ok {
				termPort = portInt
			}
		}

		if termID == id && termIP == ip && termPort == port {
			termData["_connected"] = connected
			termData["_connection_error"] = errorMsg
			termData["_last_connection_attempt"] = time.Now()
			break
		}
	}
}

// ProcessTagRead processes RFID/biometric tag read event
func (d *Daemon) ProcessTagRead(connKey string, uid string, readerType uint8, auth bool) {
	d.logger.Info(fmt.Sprintf("Tag read: conn=%s, uid=%s, reader_type=%d, auth=%v", connKey, uid, readerType, auth))

	// Get connection
	conn := d.pool.GetConnection(connKey)
	if conn == nil || conn.Settings == nil {
		d.logger.Warn(fmt.Sprintf("Connection not found or settings missing: %s", connKey))
		return
	}

	// Check gmclist (global card deny list) FIRST
	uidHex := strings.ToUpper(uid)
	if d.cardList != nil {
		if msg := d.cardList.CheckGlobal(uidHex); msg != "" {
			d.logger.Info(fmt.Sprintf("Card deny (gmclist): uid=%s, message=%s", uidHex, msg))
			if conn.Settings.Type == types.TTYPE_POCKET {
				interactivePayload := pocket.CreateInteractivePacket(msg, 3000, 4, true)
				pkt := pocket.CreatePacket(pocket.POCKET_CMD_INTERACTIVE, 0x00, interactivePayload)
				d.pool.Send(connKey, pkt)
			}
			return
		}
	}

	// Check MEMREG deny (block access if storage has value)
	if conn.Settings.MemRegDeny != "" {
		memregStorage := utils.GetMemRegStorage()
		hasValue, err := memregStorage.Has(conn.Settings.MemRegDeny, uid)
		if err == nil && hasValue {
			// Deny access - storage has value (e.g., towel not returned)
			message := getMemRegDenyMessage(conn.Settings.MemRegDeny)
			d.logger.Warn(fmt.Sprintf("MEMREG deny: storage=%s, uid=%s - access denied", conn.Settings.MemRegDeny, uid))
			
			// Send denial message to terminal
			if conn.Settings.Type == types.TTYPE_POCKET {
				interactivePayload := pocket.CreateInteractivePacket(message, 3000, 4, true) // Sound 4 = error
				pkt := pocket.CreatePacket(0x06, 0x00, interactivePayload) // 0x00 = RT_MAIN flags
				d.pool.Send(connKey, pkt)
			} else if conn.Settings.Type == types.TTYPE_JSP {
				// Send denial message to JSP terminal
				if err := d.pool.SendJSPMessage(connKey, message, 3000); err != nil {
					d.logger.Warn(fmt.Sprintf("Failed to send JSP denial message: %v", err))
				}
			}
			
			// If role=checkout and ctrole=card_taker, take the card
			if conn.Settings.MemRegRole == "checkout" && conn.Settings.CTRole == "card_taker" {
				// Send card taker signal (POCKET_SIGNAL_LOCKED with TAKE_CARD flag)
				d.logger.Info(fmt.Sprintf("MEMREG checkout: taking card for uid=%s", uid))
				// Send signal to take card (Signal command = 0x08)
				signalPayload := []byte{pocket.POCKET_SIGNAL_LOCKED, 0x00}
				timeout := []byte{0xDC, 0x05, 0x00, 0x00} // 1500 ms in little-endian
				payload := append(signalPayload, timeout...)
				signalPkt := pocket.CreatePacket(0x08, 0x01, payload) // 0x01 = RT_USART flags
				d.pool.Send(connKey, signalPkt)
			}
			
			return // Don't create session - access denied
		}
	}

	// Get lockers and temp_card from connection if available (for ReadTagExtended)
	var lockers []types.LockerInfo
	var tempCard bool
	if extra := conn.Settings.Extra; extra != nil {
		if lastLockers, ok := extra["last_lockers"].([]types.LockerInfo); ok {
			if lastUID, ok := extra["last_uid"].(string); ok && lastUID == uid {
				lockers = lastLockers
				d.logger.Info(fmt.Sprintf("Found lockers for UID %s: count=%d", uid, len(lockers)))
				// Clear after use
				delete(extra, "last_lockers")
				delete(extra, "last_uid")
			}
		}
		if lastTempCard, ok := extra["last_temp_card"].(bool); ok {
			if lastUID, ok := extra["last_uid"].(string); ok && lastUID == uid {
				tempCard = lastTempCard
				delete(extra, "last_temp_card")
			}
		}
	}

	// Check mclist (secondary card deny list)
	if d.cardList != nil {
		if msg := d.cardList.CheckSecondary(uidHex); msg != "" {
			d.logger.Info(fmt.Sprintf("Card deny (mclist): uid=%s, message=%s", uidHex, msg))
			if conn.Settings.Type == types.TTYPE_POCKET {
				interactivePayload := pocket.CreateInteractivePacket(msg, 3000, 4, true)
				pkt := pocket.CreatePacket(pocket.POCKET_CMD_INTERACTIVE, 0x00, interactivePayload)
				d.pool.Send(connKey, pkt)
			}
			return
		}
	}

	// Start new access session
	session, err := d.sessionMgr.StartSession(uid, connKey, "MAIN", lockers)
	if err != nil {
		d.logger.Error(fmt.Sprintf("Failed to start session for UID %s: %v", uid, err))
		return
	}

	// Store temp_card in session if available
	if tempCard {
		if rfidData, ok := session.Data["rfid"].(map[string]interface{}); ok {
			rfidData["temp_card"] = true
		}
	}

	// Check for GAT Solar (TTYPE_TIME) data
	if extra := conn.Settings.Extra; extra != nil {
		if gatType, ok := extra["gat_terminal_type"].(uint8); ok && gatType == gat.GAT_TTYPE_TIME {
			solarData := map[string]interface{}{
				"terminal_type": gatType,
			}
			if solarTime, ok := extra["gat_solar_time"].(int); ok {
				solarData["time"] = solarTime
			}
			if price, ok := extra["gat_solar_price"].(int); ok {
				solarData["price"] = price
			}
			if vendor, ok := extra["gat_solar_vendor"].(int); ok {
				solarData["vendor"] = vendor
			}
			solarData["reg_query"] = conn.Settings.RegQuery
			session.Data["gat_solar"] = solarData

			d.logger.Info(fmt.Sprintf("GAT Solar session: uid=%s, time=%v", uid, solarData["time"]))

			// Log GTime event (SQLite or CSV)
			gtimeData := map[string]string{
				"timestamp": time.Now().Format("02.01.06 15:04:05"),
				"id":        conn.Settings.ID,
				"addres":    fmt.Sprintf("%s:%d", conn.IP, conn.Port),
				"type":      "Solar",
				"uid":       uidHex,
			}
			if st, ok := solarData["time"].(int); ok {
				gtimeData["time"] = fmt.Sprintf("%d", st)
			}
			if price, ok := solarData["price"].(int); ok {
				gtimeData["price"] = fmt.Sprintf("%d", price)
			}
			if d.storageStore != nil {
				if err := d.storageStore.RegisterGTimeEvent(gtimeData); err != nil {
					d.logger.Warn(fmt.Sprintf("GTime (SQLite) log error: %v", err))
				}
			} else if d.gtimeLogger != nil {
				if err := d.gtimeLogger.RegisterEvent(gtimeData); err != nil {
					d.logger.Warn(fmt.Sprintf("GTime log error: %v", err))
				}
			}

			// Clear after use
			delete(extra, "gat_terminal_type")
			delete(extra, "gat_solar_time")
			delete(extra, "gat_solar_price")
			delete(extra, "gat_solar_vendor")
		}
	}

	d.logger.Info(fmt.Sprintf("Started session %s for UID %s", session.ID, uid))

	// Send event to web interface
	d.sendEvent("session_created", map[string]interface{}{
		"session_id": session.ID,
		"uid":        uid,
		"key":        connKey,
	})
}

// getMemRegDenyMessage returns denial message for MEMREG storage
func getMemRegDenyMessage(storage string) string {
	messages := map[string]string{
		"towel": "СДАЙТЕ\nПОЛОТЕНЦЕ",
	}
	if msg, ok := messages[storage]; ok {
		return msg
	}
	return "СНИМИТЕ\nОТМЕТКУ"
}

// ProcessBarcodeRead processes barcode/QR code read event
func (d *Daemon) ProcessBarcodeRead(connKey string, data string) {
	d.logger.Info(fmt.Sprintf("Barcode read: conn=%s, data=%s", connKey, data))

	// Get connection
	conn := d.pool.GetConnection(connKey)
	if conn == nil || conn.Settings == nil {
		d.logger.Warn(fmt.Sprintf("Connection not found or settings missing: %s", connKey))
		return
	}

	// Validate barcode data (must be 4-32 digits)
	if len(data) < 1 || len(data) > 32 {
		d.logger.Warn(fmt.Sprintf("Barcode data invalid length from %s: %d", connKey, len(data)))
		return
	}

	// Start session with barcode data
	session, err := d.sessionMgr.StartSession(data, connKey, "MAIN", nil)
	if err != nil {
		d.logger.Error(fmt.Sprintf("Failed to start barcode session: %v", err))
		return
	}

	// Mark session as barcode type
	session.Data["barcode"] = map[string]interface{}{
		"reader_type":      0,
		"reader_type_name": "BARCODE",
		"data":             data,
	}
	session.Data["is_barcode"] = true
	session.Data["tag_type"] = "qr"

	// Send waiting interactive message to terminal
	if conn.Settings.Type == types.TTYPE_POCKET {
		waitPayload := pocket.CreateInteractivePacket("Подождите...", 7000, 0, true)
		pkt := pocket.CreatePacket(pocket.POCKET_CMD_INTERACTIVE, 0x00, waitPayload)
		d.pool.Send(connKey, pkt)
	}

	d.logger.Info(fmt.Sprintf("Started barcode session %s for data=%s on %s", session.ID, data, connKey))

	// Send event to web interface
	d.sendEvent("barcode_read", map[string]interface{}{
		"session_id": session.ID,
		"data":       data,
		"conn_key":   connKey,
	})
}

// handleHeliosEvent handles Helios WebSocket events
func (d *Daemon) handleHeliosEvent(request *helios.HeliosRequest, eventType helios.HeliosEventType, data map[string]interface{}) {
	session := d.sessionMgr.GetSession(request.SessionID)
	if session == nil {
		d.logger.Warn(fmt.Sprintf("Session not found for Helios event: %s", request.SessionID))
		return
	}

	// Check if this request matches session's camera request
	camData, ok := session.Data["cam"].(map[string]interface{})
	if !ok {
		camData = make(map[string]interface{})
		session.Data["cam"] = camData
	}

	camRKey, _ := camData["rkey"].(string)
	if camRKey != request.ID {
		d.logger.Warn(fmt.Sprintf("Helios request ID mismatch: session=%s, request=%s, expected=%s", session.ID, request.ID, camRKey))
		return
	}

	// Process event based on type
	switch eventType {
	case helios.HELIOS_EVENT_YES:
		d.logger.Info(fmt.Sprintf("Helios person VERIFIED for session %s", session.ID))
		camData["result"] = types.CAM_RES_YES
		camData["answer_data"] = data
		d.sessionMgr.ProcessSessionStage(session.ID)
		d.sendEvent("helios_event", map[string]interface{}{
			"session_id": session.ID,
			"event_type": "YES",
			"data":       data,
		})

	case helios.HELIOS_EVENT_NO:
		d.logger.Info(fmt.Sprintf("Helios person NOT RECOGNIZED for session %s", session.ID))
		camData["result"] = types.CAM_RES_NO
		camData["answer_data"] = data
		d.sessionMgr.ProcessSessionStage(session.ID)
		d.sendEvent("helios_event", map[string]interface{}{
			"session_id": session.ID,
			"event_type": "NO",
			"data":       data,
		})

	case helios.HELIOS_EVENT_NF:
		d.logger.Info(fmt.Sprintf("Helios person NOT FOUND for session %s", session.ID))
		camData["result"] = types.CAM_RES_NF
		camData["answer_data"] = data
		d.sessionMgr.ProcessSessionStage(session.ID)
		d.sendEvent("helios_event", map[string]interface{}{
			"session_id": session.ID,
			"event_type": "NF",
			"data":       data,
		})

	case helios.HELIOS_EVENT_COR:
		// Correlation update - show progress
		if correlations, ok := data["correlations"].(map[string]interface{}); ok {
			corKeys := make([]string, 0, len(correlations))
			for k := range correlations {
				corKeys = append(corKeys, k)
			}
			if len(corKeys) > 0 {
				matches, _ := correlations[corKeys[0]].(map[string]interface{})["matches"].([]interface{})
				if len(matches) > 0 {
					maxKoef := 0
					for _, m := range matches {
						if match, ok := m.(map[string]interface{}); ok {
							if corr, ok := match["correlation"].(float64); ok {
								koef := int(corr * 100)
								if koef > maxKoef {
									maxKoef = koef
								}
							}
						}
					}
					// Update max correlation
					if maxCorr, ok := camData["max_correlation"].(int); !ok || maxKoef > maxCorr {
						camData["max_correlation"] = maxKoef
					}
					d.logger.Info(fmt.Sprintf("Helios correlation update for session %s: %d%%", session.ID, maxKoef))
					d.sendEvent("helios_correlation", map[string]interface{}{
						"session_id":  session.ID,
						"correlation": maxKoef,
					})
				}
			}
		}

	case helios.HELIOS_EVENT_FAIL:
		d.logger.Warn(fmt.Sprintf("Helios request FAILED for session %s", session.ID))
		camData["result"] = types.CAM_RES_FAIL
		camData["answer_data"] = data
		d.sessionMgr.ProcessSessionStage(session.ID)
		d.sendEvent("helios_event", map[string]interface{}{
			"session_id": session.ID,
			"event_type": "FAIL",
			"data":       data,
		})
	}

	// Mark request as processed
	request.Processed = true
}

// validateFaceIDData validates face ID person data
// In PHP this was dmnh_faceid_read_try_deny (disabled, but validates PID format 3-15 digits)
func validateFaceIDData(pid string) (bool, string) {
	pid = strings.TrimSpace(pid)
	if len(pid) < 3 || len(pid) > 15 {
		return false, "Лицо\nне распознано"
	}
	// Check that PID contains only digits
	for _, c := range pid {
		if c < '0' || c > '9' {
			return false, "Лицо\nне распознано"
		}
	}
	return true, ""
}

// handleCRTIdentification handles CRT person identification event
func (d *Daemon) handleCRTIdentification(terminalID string, personID string, fio string, camID string, score float64, data map[string]interface{}) {
	d.logger.Info(fmt.Sprintf("CRT identification: terminal=%s, person=%s, fio=%s, cam=%s, score=%.2f", terminalID, personID, fio, camID, score))

	// Validate FaceID PID data
	if valid, errMsg := validateFaceIDData(personID); !valid {
		d.logger.Warn(fmt.Sprintf("CRT: invalid FaceID PID=%s: %s", personID, errMsg))
		return
	}

	// Find connection by terminal ID
	var connKey string
	connections := d.pool.GetConnections()
	for key, conn := range connections {
		if conn.Settings != nil && conn.Settings.ID == terminalID {
			connKey = key
			break
		}
	}

	if connKey == "" {
		d.logger.Warn(fmt.Sprintf("CRT: terminal %s not found in connections", terminalID))
		return
	}

	conn := d.pool.GetConnection(connKey)
	if conn == nil || conn.Settings == nil {
		d.logger.Warn(fmt.Sprintf("CRT: connection %s not found", connKey))
		return
	}

	// Check if there's already an active session for this connection
	sessions := d.sessionMgr.GetAllSessions()
	for _, s := range sessions {
		if s.Key == connKey && !s.Processed && !s.Completed {
			d.logger.Info(fmt.Sprintf("CRT: session already active for terminal %s, skipping", terminalID))
			return
		}
	}

	// Create FaceID session
	session, err := d.sessionMgr.StartSession(personID, connKey, "MAIN", nil)
	if err != nil {
		d.logger.Error(fmt.Sprintf("CRT: failed to start session: %v", err))
		return
	}

	// Set FaceID data in session
	session.Data["faceid"] = map[string]interface{}{
		"reader_type":      16,
		"reader_type_name": "MAIN",
		"pid":              personID,
		"fio":              fio,
		"cam_id":           camID,
		"data":             personID,
		"crt":              data,
	}

	// If crt_no_kpo_pass: auto-grant access without KPO check
	if d.config.CRTNoKpoPass {
		msg := strings.TrimSpace(fio) + ", " + crt.FormatScore(score)
		if len(msg) < 3 {
			msg = d.config.ServiceFixedMsg
		}

		session.Data["kpo"] = map[string]interface{}{
			"result":          types.KPO_RES_YES,
			"message":         msg,
			"kpo_answer_data": "autofix",
		}
		session.Stage = types.SESSION_STAGE_KPO_RESULT
		session.Data["ap_mode"] = true
		session.Data["no_report"] = true
	}

	d.logger.Info(fmt.Sprintf("CRT: session %s created for person %s on terminal %s", session.ID, personID, terminalID))

	// Send event to web interface
	d.sendEvent("crt_identification", map[string]interface{}{
		"session_id":  session.ID,
		"terminal_id": terminalID,
		"person_id":   personID,
		"fio":         fio,
		"cam_id":      camID,
		"score":       score,
	})
}

// ProcessPassEvent processes person passed event
func (d *Daemon) ProcessPassEvent(connKey string, passed bool) {
	d.logger.Info(fmt.Sprintf("Pass event: conn=%s, passed=%v", connKey, passed))

	// Find active session for this connection
	sessions := d.sessionMgr.GetAllSessions()
	var sessionID string
	for _, session := range sessions {
		if session.Key == connKey && !session.Completed {
			sessionID = session.ID
			// Update session based on current stage
			if session.Stage == types.SESSION_STAGE_OPEN_FIRST {
				session.Data["passed_first"] = passed
				d.sessionMgr.ProcessSessionStage(session.ID) // Trigger stage processing
			} else if session.Stage == types.SESSION_STAGE_OPEN_SECOND {
				session.Data["passed_second"] = passed
				d.sessionMgr.ProcessSessionStage(session.ID) // Trigger stage processing
			}

			// CRT: set ban after pass if applicable
			if passed && d.crtClient != nil {
				if faceData, ok := session.Data["faceid"].(map[string]interface{}); ok {
					camID, _ := faceData["cam_id"].(string)
					pid, _ := faceData["pid"].(string)
					d.crtClient.TryBanAfterPass(camID, pid)
				}
			}
			break
		}
	}

	// Send event to web interface
	if sessionID != "" {
		d.sendEvent("pass_event", map[string]interface{}{
			"session_id": sessionID,
			"conn_key":   connKey,
			"passed":     passed,
		})
	}
}

// sendEvent sends event to web interface via eventCh
func (d *Daemon) sendEvent(eventType string, data map[string]interface{}) {
	event := map[string]interface{}{
		"type":      eventType,
		"timestamp": time.Now().Unix(),
		"data":      data,
	}

	// Non-blocking send
	select {
	case d.eventCh <- event:
		// Event sent successfully
	default:
		// Channel is full, skip event (don't block)
		d.logger.Debug(fmt.Sprintf("Event channel full, skipping event: %s", eventType))
	}
}

// processSessions processes all active sessions
func (d *Daemon) processSessions() {
	// Get all active sessions from session manager
	sessions := d.sessionMgr.GetAllSessions()

	for _, session := range sessions {
		if !session.Completed && !session.Processed {
			err := d.sessionMgr.ProcessSessionStage(session.ID)
			if err != nil {
				d.logger.Error(fmt.Sprintf("Failed to process session %s: %v", session.ID, err))
			}
		}
	}
}

// startWebServer starts the web interface server
func (d *Daemon) startWebServer() error {
	mux := http.NewServeMux()

	// Register web handlers
	mux.HandleFunc("/", d.handleWebIndex)
	mux.HandleFunc("/api/stats", d.handleAPIStats)
	mux.HandleFunc("/api/connections", d.handleAPIConnections)
	mux.HandleFunc("/api/sessions", d.handleAPISessions)
	mux.HandleFunc("/api/session/", d.handleAPISessionDetail)
	mux.HandleFunc("/api/terminals", d.handleAPITerminals)
	mux.HandleFunc("/api/terminal/", d.handleAPITerminalDetail)
	mux.HandleFunc("/api/logs", d.handleAPILogs)
	mux.HandleFunc("/api/config", d.handleAPIConfig)
	mux.HandleFunc("/api/events", d.handleAPIEvents) // SSE для real-time обновлений
	mux.HandleFunc("/api/cardlist", d.handleAPICardList)
	mux.HandleFunc("/api/cardlist/", d.handleAPICardList)
	mux.HandleFunc("/api/system/halt", d.handleAPIHalt)
	mux.HandleFunc("/api/system/settings", d.handleAPISettings)
	mux.HandleFunc("/api/system/settings/", d.handleAPISettings)
	mux.HandleFunc("/api/terminals/add", d.handleAPITerminalsAdd)
	mux.HandleFunc("/api/terminals/del", d.handleAPITerminalsDel)
	mux.HandleFunc("/api/terminals/check", d.handleAPITerminalsCheck)
	mux.HandleFunc("/api/tlogs", d.handleAPITermLogs)
	mux.HandleFunc("/api/tlogs/", d.handleAPITermLogs)

	// Create server
	d.webServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", d.config.WebAddr, d.config.WebPort),
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := d.webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			d.logger.Error(fmt.Sprintf("Web server error: %v", err))
		}
	}()

	return nil
}

// handleWebIndex is now implemented in web_ui.go with modern Russian interface
// Old implementation removed - see web_ui.go for the new version

// handleAPIStats serves statistics via JSON API
func (d *Daemon) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	d.mutex.RLock()
	running := d.running
	uptime := time.Since(d.startTime).Seconds()
	d.mutex.RUnlock()

	connections := d.pool.GetConnections()
	reconnections := d.pool.GetReconnections()
	sessions := d.sessionMgr.GetAllSessions()

	// Count HTTP requests (from config if available)
	httpRequestsCount := 0
	if d.config.HTTPRequests != nil {
		httpRequestsCount = len(d.config.HTTPRequests)
	}

	stats := map[string]interface{}{
		"running":       running,
		"connections":   len(connections),
		"reconnections": len(reconnections),
		"http_requests": httpRequestsCount,
		"sessions":      len(sessions),
		"start_time":    d.startTime.Unix(),
		"uptime":        uptime,
	}

	json.NewEncoder(w).Encode(stats)
}

// handleAPIConnections serves connections list via JSON API
func (d *Daemon) handleAPIConnections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	connections := d.pool.GetConnections()
	var result []map[string]interface{}

	for _, conn := range connections {
		result = append(result, map[string]interface{}{
			"key":           conn.Key,
			"ip":            conn.Addr,
			"port":          conn.Port,
			"type":          string(conn.Settings.Type),
			"start_time":    conn.StartTime.Unix() * 1000,
			"last_activity": conn.LastActivity.Unix() * 1000,
			"connected":     conn.Connected,
		})
	}

	json.NewEncoder(w).Encode(result)
}

// handleAPISessions serves sessions list via JSON API
func (d *Daemon) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sessions := d.sessionMgr.GetAllSessions()
	var result []map[string]interface{}

	for _, session := range sessions {
		if !session.Completed {
			result = append(result, map[string]interface{}{
				"id":        session.ID,
				"key":       session.Key,
				"uid":       session.UID,
				"stage":     session.Stage.String(),
				"req_time":  session.ReqTime.Unix() * 1000,
				"processed": session.Processed,
				"completed": session.Completed,
			})
		}
	}

	json.NewEncoder(w).Encode(result)
}
