package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"nd-go/pkg/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ConfigFile represents JSON configuration file structure
type ConfigFile struct {
	Server struct {
		Addr string `json:"addr"`
		Port int    `json:"port"`
	} `json:"server"`
	Web struct {
		Addr    string `json:"addr"`
		Port    int    `json:"port"`
		Enabled bool   `json:"enabled"`
	} `json:"web"`
	HTTPService struct {
		Active              bool     `json:"active"`
		IP                  string   `json:"ip"`
		Port                int      `json:"port"`
		Name                string   `json:"name"`
		TermlistPath        string   `json:"termlist_path"`
		IdentPath           string   `json:"ident_path"`
		SolarPath           string   `json:"solar_path"`
		UIDPath             string   `json:"uid_path"`
		URLFmtSuff          string   `json:"url_fmt_suff"`
		RequestExtraHeaders []string `json:"request_extra_headers"`
	} `json:"http_service"`
	Timeouts struct {
		ServiceRequestExpireTime float64 `json:"service_request_expire_time"`
		SessionExpireTime        float64 `json:"session_expire_time"`
		TerminalConnectTimeout   float64 `json:"terminal_connect_timeout"`
	} `json:"timeouts"`
	ErrorHandling struct {
		ServiceAutofixExpired bool    `json:"service_autofix_expired"`
		ServiceLinkErrMsg     string  `json:"service_link_err_msg"`
		HTTPRequestRetryCount int     `json:"http_request_retry_count"`
		HTTPRequestRetryDelay float64 `json:"http_request_retry_delay"`
	} `json:"error_handling"`
	Messages struct {
		ServiceErrMsg    string `json:"service_err_msg"`
		ServiceFixedMsg  string `json:"service_fixed_msg"`
		ServiceDeniedMsg string `json:"service_denied_msg"`
	} `json:"messages"`
	JSP struct {
		ListenerPort       interface{} `json:"listener_port"` // Can be bool or int
		DevAutoPingEnabled bool        `json:"dev_auto_ping_enabled"`
	} `json:"jsp"`
	Camera struct {
		ResultMsgNo   string `json:"result_msg_no"`
		ResultMsgNf   string `json:"result_msg_nf"`
		ResultMsgFail string `json:"result_msg_fail"`
	} `json:"camera"`
	TerminalList struct {
		CheckTime    float64 `json:"check_time"`
		Filter       string  `json:"filter"`
		FilterAbsent bool    `json:"filter_absent"`
	} `json:"terminal_list"`
	Logging struct {
		LogFile       string `json:"log_file"`
		LogFileScreen string `json:"log_file_screen"`
		LogFileLow    string `json:"log_file_low"`
		LogFileErr    string `json:"log_file_err"`
		Rotation      struct {
			Enabled  bool  `json:"enabled"`
			MaxSize  int64 `json:"max_size"`  // Max file size in bytes
			MaxFiles int   `json:"max_files"` // Max number of rotated files
			MaxDays  int   `json:"max_days"`  // Max age in days
		} `json:"rotation"`
	} `json:"logging"`
	Helios struct {
		Enabled bool    `json:"enabled"`
		URL     string  `json:"url"`
		Timeout float64 `json:"timeout"`
	} `json:"helios"`
	PhrasesFixes map[string]string `json:"phrases_fixes"` // message corrections for terminal display
	Storage struct {
		SqlitePath string `json:"sqlite_path"` // if set, use SQLite instead of CSV (e.g. "./data/skud.db")
	} `json:"storage"`
	Email struct {
		Enabled    bool     `json:"enabled"`
		Host       string   `json:"host"`
		Port       int      `json:"port"`
		User       string   `json:"user"`
		Password   string   `json:"password"`
		From       string   `json:"from"`
		Recipients []string `json:"recipients"`
		SendTimes  []string `json:"send_times"` // daily send at these times, "HH:MM" (e.g. ["08:00", "20:00"])
		Subject    string   `json:"subject"`    // subject template, e.g. "СКД отчёт за %s"
	} `json:"email"`
	CRT struct {
		Active             bool              `json:"active"`
		IdentificationMode bool              `json:"identification_mode"`
		IP                 string            `json:"ip"`
		Port               int               `json:"port"`
		Name               string            `json:"name"`
		URL                string            `json:"url"`
		ConnectTime1       float64           `json:"connect_time1"`
		ConnectTime2       float64           `json:"connect_time2"`
		ConnectTime3       float64           `json:"connect_time3"`
		ExpireTime         float64           `json:"expire_time"`
		ExtraHeaders       []string          `json:"extra_headers"`
		CheckTime          float64           `json:"check_time"`
		BanCamPidTime      float64           `json:"ban_cam_pid_time"`
		BanPassOnly        bool              `json:"ban_pass_only"`
		BanFromCatch       bool              `json:"ban_from_catch"`
		AutoFixMessage     bool              `json:"auto_fix_message"`
		NoKpoPass          bool              `json:"no_kpo_pass"`
		SeenTimeout        float64           `json:"seen_timeout"`
		CamLinks           map[string]string `json:"cam_links"`
	} `json:"crt"`
}

// LoadConfig loads configuration from file or creates default
// configPath: path to config file (empty = search for config.json next to executable)
// cmdArgs: command line arguments to override config values
func LoadConfig(configPath string, cmdArgs map[string]string) *types.Config {
	fmt.Println("LoadConfig called")

	cfg := getDefaultConfig()

	// Try to load from file
	if configPath == "" {
		// Try to find config.json next to executable
		exePath, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exePath)
			configPath = filepath.Join(exeDir, "config.json")
		}
	}

	if configPath != "" {
		if err := loadConfigFromFile(configPath, cfg); err != nil {
			fmt.Printf("Warning: Failed to load config from %s: %v\n", configPath, err)
			fmt.Println("Using default configuration")
		} else {
			fmt.Printf("Configuration loaded from %s\n", configPath)
		}
	}

	// Apply command line arguments (override file config)
	applyCmdArgs(cfg, cmdArgs)

	return cfg
}

// getDefaultConfig returns default configuration
// getEnvString returns environment variable value or default
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns environment variable value as int or default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool returns environment variable value as bool or default
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvFloat returns environment variable value as float64 or default
func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getDefaultConfig() *types.Config {
	// Get HTTP Service credentials from environment, fallback to working defaults
	httpServiceUser := getEnvString("HTTP_SERVICE_USER", "ServiceSkud")
	httpServicePass := getEnvString("HTTP_SERVICE_PASS", "EA780E")
	var httpServiceHeaders []string
	if httpServiceUser != "" && httpServicePass != "" {
		authHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(httpServiceUser+":"+httpServicePass))
		httpServiceHeaders = []string{authHeader}
	}

	return &types.Config{
		ServerAddr: getEnvString("SERVER_ADDR", "0.0.0.0"),
		ServerPort: getEnvInt("SERVER_PORT", 8999),
		WebAddr:    getEnvString("WEB_ADDR", "0.0.0.0"),
		WebPort:    getEnvInt("WEB_PORT", 8080),
		WebEnabled: getEnvBool("WEB_ENABLED", true),

		// HTTP Service (1C integration) - from environment or working defaults
		HTTPServiceActive:              getEnvBool("HTTP_SERVICE_ACTIVE", true),
		HTTPServiceIP:                  getEnvString("HTTP_SERVICE_IP", "virt201.worldclass.nnov.ru"),
		HTTPServicePort:                getEnvInt("HTTP_SERVICE_PORT", 80),
		HTTPServiceName:                getEnvString("HTTP_SERVICE_NAME", "virt201.worldclass.nnov.ru"),
		HTTPServiceTermlistPath:        getEnvString("HTTP_SERVICE_TERMLIST_PATH", "/gymdb/hs/ACS/terminals"),
		HTTPServiceIdentPath:           getEnvString("HTTP_SERVICE_IDENT_PATH", "/gymdb/hs/ACS/checking"),
		HTTPServiceSolarPath:           getEnvString("HTTP_SERVICE_SOLAR_PATH", "/gymdb/hs/ACS/solarium"),
		HTTPServiceUIDPath:             getEnvString("HTTP_SERVICE_UID_PATH", "/gymdb/hs/ACS/uid"),
		HTTPServiceUrlFmtSuff:          getEnvString("HTTP_SERVICE_URL_FMT_SUFF", "wc1c"),
		HTTPServiceRequestExtraHeaders: httpServiceHeaders,

		// Timeouts
		ServiceRequestExpireTime: getEnvFloat("SERVICE_REQUEST_EXPIRE_TIME", 5.0),
		SessionExpireTime:        getEnvFloat("SESSION_EXPIRE_TIME", 300.0),
		TerminalConnectTimeout:   getEnvFloat("TERMINAL_CONNECT_TIMEOUT", 10.0),

		// Error handling
		ServiceAutofixExpired: getEnvBool("SERVICE_AUTOFIX_EXPIRED", false),
		ServiceLinkErrMsg:     getEnvString("SERVICE_LINK_ERR_MSG", "Ошибка связи. Обратитесь на рецепцию."),
		HTTPRequestRetryCount: getEnvInt("HTTP_REQUEST_RETRY_COUNT", 2),
		HTTPRequestRetryDelay: getEnvFloat("HTTP_REQUEST_RETRY_DELAY", 0.5),

		// Messages
		ServiceErrMsg:    getEnvString("SERVICE_ERR_MSG", "Ошибка связи с БД"),
		ServiceFixedMsg:  getEnvString("SERVICE_FIXED_MSG", "Проходите"),
		ServiceDeniedMsg: getEnvString("SERVICE_DENIED_MSG", "Доступ запрещен"),

		// JSP settings
		JSPListenerPort:       getEnvBool("JSP_LISTENER_PORT", false),
		JSPDevAutoPingEnabled: getEnvBool("JSP_DEV_AUTO_PING_ENABLED", true),

		// Camera service messages
		CamServiceResultMsgNo:   getEnvString("CAM_SERVICE_RESULT_MSG_NO", "Лицо не распознано"),
		CamServiceResultMsgNf:   getEnvString("CAM_SERVICE_RESULT_MSG_NF", "НЕТ ФОТО !!! Обратитесь в отдел продаж"),
		CamServiceResultMsgFail: getEnvString("CAM_SERVICE_RESULT_MSG_FAIL", "Ошибка распознавания"),

		// Terminal list checking
		TermListCheckTime:    getEnvFloat("TERM_LIST_CHECK_TIME", 60.0),
		TermListFilter:       getEnvString("TERM_LIST_FILTER", `/192\.168\.12\.2(3|4)(2|3|4|5|6|7|8)/`),
		TermListFilterAbsent: getEnvBool("TERM_LIST_FILTER_ABSENT", false),
		TerminalList:         make([]map[string]interface{}, 0),

		LogFile:             getEnvString("LOG_FILE", "log_bin.txt"),
		LogRotationEnabled:  getEnvBool("LOG_ROTATION_ENABLED", true),
		LogRotationMaxSize:  int64(getEnvInt("LOG_ROTATION_MAX_SIZE", 10*1024*1024)),
		LogRotationMaxFiles: getEnvInt("LOG_ROTATION_MAX_FILES", 10),
		LogRotationMaxDays:  getEnvInt("LOG_ROTATION_MAX_DAYS", 30),
		StorageSqlitePath:   "",
		EmailEnabled:        false,
		EmailHost:           "",
		EmailPort:           587,
		EmailFrom:           "",
		EmailSubject:        "СКД отчёт за %s",
		EmailSendTimes:      []string{"08:00"},
		Stats:               map[string]interface{}{"start_time": time.Now()},
		IDGen:               0,
		Connections:         make(map[string]*types.Connection),
		Reconnections:       make(map[string]*types.Reconnection),
		Sessions:            make(map[string]*types.Session),
		HTTPRequests:        make(map[string]*types.HTTPRequest),
	}
}

// loadConfigFromFile loads configuration from JSON file
func loadConfigFromFile(path string, cfg *types.Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var fileCfg ConfigFile
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Apply file configuration
	if fileCfg.Server.Addr != "" {
		cfg.ServerAddr = fileCfg.Server.Addr
	}
	if fileCfg.Server.Port > 0 {
		cfg.ServerPort = fileCfg.Server.Port
	}

	if fileCfg.Web.Addr != "" {
		cfg.WebAddr = fileCfg.Web.Addr
	}
	if fileCfg.Web.Port > 0 {
		cfg.WebPort = fileCfg.Web.Port
	}
	cfg.WebEnabled = fileCfg.Web.Enabled

	// HTTP Service
	cfg.HTTPServiceActive = fileCfg.HTTPService.Active
	if fileCfg.HTTPService.IP != "" {
		cfg.HTTPServiceIP = fileCfg.HTTPService.IP
	}
	if fileCfg.HTTPService.Port > 0 {
		cfg.HTTPServicePort = fileCfg.HTTPService.Port
	}
	if fileCfg.HTTPService.Name != "" {
		cfg.HTTPServiceName = fileCfg.HTTPService.Name
	}
	if fileCfg.HTTPService.TermlistPath != "" {
		cfg.HTTPServiceTermlistPath = fileCfg.HTTPService.TermlistPath
	}
	if fileCfg.HTTPService.IdentPath != "" {
		cfg.HTTPServiceIdentPath = fileCfg.HTTPService.IdentPath
	}
	if fileCfg.HTTPService.SolarPath != "" {
		cfg.HTTPServiceSolarPath = fileCfg.HTTPService.SolarPath
	}
	if fileCfg.HTTPService.UIDPath != "" {
		cfg.HTTPServiceUIDPath = fileCfg.HTTPService.UIDPath
	}
	if fileCfg.HTTPService.URLFmtSuff != "" {
		cfg.HTTPServiceUrlFmtSuff = fileCfg.HTTPService.URLFmtSuff
	}
	if len(fileCfg.HTTPService.RequestExtraHeaders) > 0 {
		cfg.HTTPServiceRequestExtraHeaders = fileCfg.HTTPService.RequestExtraHeaders
	}

	// Timeouts
	if fileCfg.Timeouts.ServiceRequestExpireTime > 0 {
		cfg.ServiceRequestExpireTime = fileCfg.Timeouts.ServiceRequestExpireTime
	}
	if fileCfg.Timeouts.SessionExpireTime > 0 {
		cfg.SessionExpireTime = fileCfg.Timeouts.SessionExpireTime
	}
	if fileCfg.Timeouts.TerminalConnectTimeout > 0 {
		cfg.TerminalConnectTimeout = fileCfg.Timeouts.TerminalConnectTimeout
	}

	// Error handling
	cfg.ServiceAutofixExpired = fileCfg.ErrorHandling.ServiceAutofixExpired
	if fileCfg.ErrorHandling.ServiceLinkErrMsg != "" {
		cfg.ServiceLinkErrMsg = fileCfg.ErrorHandling.ServiceLinkErrMsg
	}
	if fileCfg.ErrorHandling.HTTPRequestRetryCount > 0 {
		cfg.HTTPRequestRetryCount = fileCfg.ErrorHandling.HTTPRequestRetryCount
	}
	if fileCfg.ErrorHandling.HTTPRequestRetryDelay > 0 {
		cfg.HTTPRequestRetryDelay = fileCfg.ErrorHandling.HTTPRequestRetryDelay
	}

	// Messages
	if fileCfg.Messages.ServiceErrMsg != "" {
		cfg.ServiceErrMsg = fileCfg.Messages.ServiceErrMsg
	}
	if fileCfg.Messages.ServiceFixedMsg != "" {
		cfg.ServiceFixedMsg = fileCfg.Messages.ServiceFixedMsg
	}
	if fileCfg.Messages.ServiceDeniedMsg != "" {
		cfg.ServiceDeniedMsg = fileCfg.Messages.ServiceDeniedMsg
	}

	// JSP
	if fileCfg.JSP.ListenerPort != nil {
		switch v := fileCfg.JSP.ListenerPort.(type) {
		case bool:
			cfg.JSPListenerPort = v
		case float64:
			cfg.JSPListenerPort = int(v)
		case int:
			cfg.JSPListenerPort = v
		}
	}
	cfg.JSPDevAutoPingEnabled = fileCfg.JSP.DevAutoPingEnabled

	// Camera
	if fileCfg.Camera.ResultMsgNo != "" {
		cfg.CamServiceResultMsgNo = fileCfg.Camera.ResultMsgNo
	}
	if fileCfg.Camera.ResultMsgNf != "" {
		cfg.CamServiceResultMsgNf = fileCfg.Camera.ResultMsgNf
	}
	if fileCfg.Camera.ResultMsgFail != "" {
		cfg.CamServiceResultMsgFail = fileCfg.Camera.ResultMsgFail
	}

	// Terminal list
	if fileCfg.TerminalList.CheckTime > 0 {
		cfg.TermListCheckTime = fileCfg.TerminalList.CheckTime
	}
	if fileCfg.TerminalList.Filter != "" {
		cfg.TermListFilter = fileCfg.TerminalList.Filter
	}
	cfg.TermListFilterAbsent = fileCfg.TerminalList.FilterAbsent

	// Logging
	if fileCfg.Logging.LogFile != "" {
		cfg.LogFile = fileCfg.Logging.LogFile
	}

	// Helios - check if fields exist in Config
	// Note: Helios configuration may be stored differently in Config
	// For now, skip if fields don't exist

	// Phrase fixes
	if len(fileCfg.PhrasesFixes) > 0 {
		cfg.PhrasesFixes = fileCfg.PhrasesFixes
	}

	// CRT (Vizir)
	cfg.CRTServiceActive = fileCfg.CRT.Active
	cfg.CRTServiceIdentificationMode = fileCfg.CRT.IdentificationMode
	if fileCfg.CRT.IP != "" {
		cfg.CRTServiceIP = fileCfg.CRT.IP
	}
	if fileCfg.CRT.Port > 0 {
		cfg.CRTServicePort = fileCfg.CRT.Port
	}
	if fileCfg.CRT.Name != "" {
		cfg.CRTServiceName = fileCfg.CRT.Name
	}
	if fileCfg.CRT.URL != "" {
		cfg.CRTServiceURL = fileCfg.CRT.URL
	}
	if fileCfg.CRT.ConnectTime1 > 0 {
		cfg.CRTServiceConnectTime1 = fileCfg.CRT.ConnectTime1
	}
	if fileCfg.CRT.ConnectTime2 > 0 {
		cfg.CRTServiceConnectTime2 = fileCfg.CRT.ConnectTime2
	}
	if fileCfg.CRT.ConnectTime3 > 0 {
		cfg.CRTServiceConnectTime3 = fileCfg.CRT.ConnectTime3
	}
	if fileCfg.CRT.ExpireTime > 0 {
		cfg.CRTServiceExpireTime = fileCfg.CRT.ExpireTime
	}
	if len(fileCfg.CRT.ExtraHeaders) > 0 {
		cfg.CRTServiceExtraHeaders = fileCfg.CRT.ExtraHeaders
	}
	if fileCfg.CRT.CheckTime >= 0 {
		cfg.CRTCheckTime = fileCfg.CRT.CheckTime
	}
	if fileCfg.CRT.BanCamPidTime > 0 {
		cfg.CRTBanCamPidTime = fileCfg.CRT.BanCamPidTime
	}
	cfg.CRTBanPassOnly = fileCfg.CRT.BanPassOnly
	cfg.CRTBanFromCatch = fileCfg.CRT.BanFromCatch
	cfg.CRTAutoFixMessage = fileCfg.CRT.AutoFixMessage
	cfg.CRTNoKpoPass = fileCfg.CRT.NoKpoPass
	if fileCfg.CRT.SeenTimeout > 0 {
		cfg.CRTSeenTimeout = fileCfg.CRT.SeenTimeout
	}
	if len(fileCfg.CRT.CamLinks) > 0 {
		cfg.CRTCamLinks = fileCfg.CRT.CamLinks
	}

	// Storage
	if fileCfg.Storage.SqlitePath != "" {
		cfg.StorageSqlitePath = fileCfg.Storage.SqlitePath
	}

	// Email
	cfg.EmailEnabled = fileCfg.Email.Enabled
	if fileCfg.Email.Host != "" {
		cfg.EmailHost = fileCfg.Email.Host
	}
	if fileCfg.Email.Port > 0 {
		cfg.EmailPort = fileCfg.Email.Port
	}
	cfg.EmailUser = fileCfg.Email.User
	cfg.EmailPassword = fileCfg.Email.Password
	if fileCfg.Email.From != "" {
		cfg.EmailFrom = fileCfg.Email.From
	}
	if len(fileCfg.Email.Recipients) > 0 {
		cfg.EmailRecipients = fileCfg.Email.Recipients
	}
	if len(fileCfg.Email.SendTimes) > 0 {
		cfg.EmailSendTimes = fileCfg.Email.SendTimes
	}
	if fileCfg.Email.Subject != "" {
		cfg.EmailSubject = fileCfg.Email.Subject
	}

	return nil
}

// applyCmdArgs applies command line arguments to configuration
func applyCmdArgs(cfg *types.Config, cmdArgs map[string]string) {
	for key, value := range cmdArgs {
		switch strings.ToLower(key) {
		case "server.addr", "server_addr":
			cfg.ServerAddr = value
		case "server.port", "server_port":
			if port, err := strconv.Atoi(value); err == nil {
				cfg.ServerPort = port
			}
		case "web.addr", "web_addr":
			cfg.WebAddr = value
		case "web.port", "web_port":
			if port, err := strconv.Atoi(value); err == nil {
				cfg.WebPort = port
			}
		case "web.enabled", "web_enabled":
			if enabled, err := strconv.ParseBool(value); err == nil {
				cfg.WebEnabled = enabled
			}
		case "http_service.active", "http_service_active":
			if active, err := strconv.ParseBool(value); err == nil {
				cfg.HTTPServiceActive = active
			}
		case "http_service.ip", "http_service_ip":
			cfg.HTTPServiceIP = value
		case "http_service.port", "http_service_port":
			if port, err := strconv.Atoi(value); err == nil {
				cfg.HTTPServicePort = port
			}
		case "http_service.name", "http_service_name":
			cfg.HTTPServiceName = value
		case "term_list.filter", "term_list_filter":
			cfg.TermListFilter = value
		case "term_list.filter_absent", "term_list_filter_absent":
			if absent, err := strconv.ParseBool(value); err == nil {
				cfg.TermListFilterAbsent = absent
			}
		case "log.file", "log_file":
			cfg.LogFile = value
		}
	}
}

// SaveConfigExample saves example configuration file
func SaveConfigExample(path string) error {
	example := ConfigFile{}
	example.Server.Addr = "0.0.0.0"
	example.Server.Port = 8999
	example.Web.Addr = "0.0.0.0"
	example.Web.Port = 8080
	example.Web.Enabled = true
	example.HTTPService.Active = true
	example.HTTPService.IP = ""
	example.HTTPService.Port = 80
	example.HTTPService.Name = ""
	example.HTTPService.TermlistPath = ""
	example.HTTPService.IdentPath = ""
	example.HTTPService.SolarPath = ""
	example.HTTPService.UIDPath = ""
	example.HTTPService.URLFmtSuff = ""
	example.HTTPService.RequestExtraHeaders = []string{}
	example.Timeouts.ServiceRequestExpireTime = 5.0
	example.Timeouts.SessionExpireTime = 300.0
	example.Timeouts.TerminalConnectTimeout = 10.0
	example.ErrorHandling.ServiceAutofixExpired = false
	example.ErrorHandling.ServiceLinkErrMsg = "Ошибка связи. Обратитесь на рецепцию."
	example.ErrorHandling.HTTPRequestRetryCount = 2
	example.ErrorHandling.HTTPRequestRetryDelay = 0.5
	example.Messages.ServiceErrMsg = "Ошибка связи с БД"
	example.Messages.ServiceFixedMsg = "Проходите"
	example.Messages.ServiceDeniedMsg = "Доступ запрещен"
	example.JSP.ListenerPort = false
	example.JSP.DevAutoPingEnabled = true
	example.Camera.ResultMsgNo = "Лицо не распознано"
	example.Camera.ResultMsgNf = "НЕТ ФОТО !!! Обратитесь в отдел продаж"
	example.Camera.ResultMsgFail = "Ошибка распознавания"
	example.TerminalList.CheckTime = 60.0
	example.TerminalList.Filter = `/192\.168\.1\.(10|20|30)/`
	example.TerminalList.FilterAbsent = false
	example.Logging.LogFile = "log_bin.txt"
	example.Logging.LogFileScreen = "log_screen.txt"
	example.Logging.LogFileLow = "log_low.txt"
	example.Logging.LogFileErr = "log_err.txt"
	example.Logging.Rotation.Enabled = true
	example.Logging.Rotation.MaxSize = 10 * 1024 * 1024 // 10 MB
	example.Logging.Rotation.MaxFiles = 10
	example.Logging.Rotation.MaxDays = 30
	example.Helios.Enabled = false
	example.Helios.URL = "ws://localhost:8081"
	example.Helios.Timeout = 5.0
	example.PhrasesFixes = map[string]string{
		"Извините;клиент не идентифицирован;": "Извините;Клиент не;идентифицирован",
		"Извините;Клиент уже в клубе;":       "Извините;Клиент;уже в клубе",
	}
	example.CRT.Active = false
	example.CRT.IdentificationMode = true
	example.CRT.IP = "192.168.0.20"
	example.CRT.Port = 34015
	example.CRT.Name = "192.168.0.20"
	example.CRT.URL = "/vizir/v1/api/"
	example.CRT.ConnectTime1 = 0.5
	example.CRT.ConnectTime2 = 1.0
	example.CRT.ConnectTime3 = 1.0
	example.CRT.ExpireTime = 5.0
	example.CRT.ExtraHeaders = []string{}
	example.CRT.CheckTime = 0.0
	example.CRT.BanCamPidTime = 5.0
	example.CRT.BanPassOnly = true
	example.CRT.BanFromCatch = false
	example.CRT.AutoFixMessage = true
	example.CRT.NoKpoPass = true
	example.CRT.SeenTimeout = 10.0
	example.CRT.CamLinks = map[string]string{}
	example.Storage.SqlitePath = "./data/skud.db"
	example.Email.Enabled = false
	example.Email.Host = "smtp.example.com"
	example.Email.Port = 587
	example.Email.User = ""
	example.Email.Password = ""
	example.Email.From = "skud@example.com"
	example.Email.Recipients = []string{"admin@example.com"}
	example.Email.SendTimes = []string{"08:00", "20:00"}
	example.Email.Subject = "СКД отчёт за %s"

	data, err := json.MarshalIndent(example, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ValidateConfig validates configuration
func ValidateConfig(cfg *types.Config) error {
	// Add validation logic here
	return nil
}
