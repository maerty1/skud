package types

import (
	"time"
)

// Terminal Types
type TerminalType string

const (
	TTYPE_GAT    TerminalType = "gat"
	TTYPE_POCKET TerminalType = "pocket"
	TTYPE_SPHINX TerminalType = "sphinx"
	TTYPE_JSP    TerminalType = "jsp"
)

// Session Processing Stages
type SessionStage int

const (
	SESSION_STAGE_INIT SessionStage = iota
	SESSION_STAGE_KPO_RESULT
	SESSION_STAGE_KPO_DIRECT
	SESSION_STAGE_CAM_RESULT
	SESSION_STAGE_OPEN_FIRST
	SESSION_STAGE_FIRST_PASSED
	SESSION_STAGE_OPEN_SECOND
	SESSION_STAGE_SECOND_PASSED
	SESSION_STAGE_PASSED
	SESSION_STAGE_LAST_ANSWER
	SESSION_STAGE_DONE
)

// String returns string representation of SessionStage
func (s SessionStage) String() string {
	switch s {
	case SESSION_STAGE_INIT:
		return "INIT"
	case SESSION_STAGE_KPO_RESULT:
		return "KPO_RESULT"
	case SESSION_STAGE_KPO_DIRECT:
		return "KPO_DIRECT"
	case SESSION_STAGE_CAM_RESULT:
		return "CAM_RESULT"
	case SESSION_STAGE_OPEN_FIRST:
		return "OPEN_FIRST"
	case SESSION_STAGE_FIRST_PASSED:
		return "FIRST_PASSED"
	case SESSION_STAGE_OPEN_SECOND:
		return "OPEN_SECOND"
	case SESSION_STAGE_SECOND_PASSED:
		return "SECOND_PASSED"
	case SESSION_STAGE_PASSED:
		return "PASSED"
	case SESSION_STAGE_LAST_ANSWER:
		return "LAST_ANSWER"
	case SESSION_STAGE_DONE:
		return "DONE"
	default:
		return "UNKNOWN"
	}
}

// SessionWait represents waiting state for session
type SessionWait struct {
	ProcType   int                    `json:"proc_type"`   // SESSION_PROC_*
	ExpireTime time.Time              `json:"expire_time"` // When wait expires
	DstStage   SessionStage           `json:"dst_stage"`   // Destination stage after wait
	Params     map[string]interface{} `json:"params"`      // Additional parameters
}

// Request Tags
type RequestTag uint8

const (
	REQ_TAG_TLIST RequestTag = 0x00
	REQ_TAG_QRY   RequestTag = 0x01
	REQ_TAG_REG   RequestTag = 0x02
	REQ_TAG_CAM   RequestTag = 0x03
	REQ_TAG_ID    RequestTag = 0x04
	REQ_TAG_CRT   RequestTag = 0x05
)

// KPO Results
type KPOResult int

const (
	KPO_RES_UNDEF KPOResult = 0x00
	KPO_RES_YES   KPOResult = 0x01
	KPO_RES_NO    KPOResult = 0x02
	KPO_RES_FAIL  KPOResult = 0x03
)

// Camera Results
type CamResult int

const (
	CAM_RES_UNDEF CamResult = 0x00
	CAM_RES_YES   CamResult = 0x01
	CAM_RES_NO    CamResult = 0x02
	CAM_RES_FAIL  CamResult = 0x03
	CAM_RES_NF    CamResult = 0x04
)

// Terminal Settings
type TerminalSettings struct {
	ID           string                       `json:"id"`
	IP           string                       `json:"ip"`
	Port         int                          `json:"port"`
	Type         TerminalType                 `json:"type"`
	UTF          bool                         `json:"utf"`
	RegQuery     bool                         `json:"reg_query"`
	Proxy        *int                         `json:"proxy,omitempty"`
	Apkeys       map[string]*TerminalSettings `json:"apkeys,omitempty"`
	ConfigString string                       `json:"config_string"`
	DenyLockers  bool                         `json:"deny_lockers"` // Deny access if lockers present
	DenyCT       bool                         `json:"deny_ct"`      // Deny access for temporary cards
	CTRole       string                       `json:"ctrole"`       // Card taker role ("card_taker" or empty)
	MemRegDev    string                       `json:"memreg_dev"`   // MEMREG device mode (e.g., "towel/add", "towel/take")
	MemRegDeny   string                       `json:"memreg_deny"`  // MEMREG deny storage (e.g., "towel")
	MemRegRole   string                       `json:"memreg_role"`  // MEMREG role (e.g., "checkout")
	Extra        map[string]interface{}       `json:"extra,omitempty"`
}

// Connection Info
type Connection struct {
	Key          string            `json:"key"`
	ConKey       string            `json:"con_key"`
	IP           string            `json:"ip"`
	Port         int               `json:"port"`
	Settings     *TerminalSettings `json:"settings"`
	StartTime    time.Time         `json:"start_time"`
	LastActivity time.Time         `json:"last_activity_time"`
	Connected    bool              `json:"connected"`
	ReaderLocked string            `json:"reader_locked,omitempty"` // Session ID that locked this reader
	RC           *Reconnection     `json:"rc,omitempty"`
}

// Reconnection Info
type Reconnection struct {
	Key      string            `json:"key"`
	ConKey   string            `json:"con_key"`
	IP       string            `json:"ip"`
	Port     int               `json:"port"`
	Time     time.Time         `json:"time"`
	NTime    time.Time         `json:"ntime"`
	Count    int               `json:"count"`
	Settings *TerminalSettings `json:"settings,omitempty"` // Terminal settings for reconnection
}

// LockerInfo represents locker information from POCKET
type LockerInfo struct {
	AuthErr    uint8  `json:"auth_err"`    // Authentication error (4 bits)
	ReadErr    uint8  `json:"read_err"`    // Read error (4 bits)
	IsPasstech bool   `json:"is_passtech"` // Is Passtech format
	BlockNo    uint8  `json:"block_no"`    // Block number
	Litera     string `json:"litera"`      // Letter for Passtech (A-Z)
	Locked     bool   `json:"locked"`      // Is locker locked
	CabNo      uint16 `json:"cab_no"`      // Cabinet number (15 bits)
}

// Session Data
type Session struct {
	ID         string                 `json:"s_id"`
	Key        string                 `json:"key"`
	Apkey      string                 `json:"apkey"`
	UID        string                 `json:"uid"`
	UIDRaw     string                 `json:"uid_raw"`
	CID        string                 `json:"cid,omitempty"`
	Data       map[string]interface{} `json:"data"`
	Stage      SessionStage           `json:"stage"`
	ReqTime    time.Time              `json:"req_time"`
	Processed  bool                   `json:"processed"`
	Completed  bool                   `json:"completed"`
	ReportSent bool                   `json:"report_sent"`
	Alive      bool                   `json:"alive"`
	Wait       *SessionWait           `json:"wait,omitempty"` // Waiting state
}

// HTTP Request
type HTTPRequest struct {
	Key       string                 `json:"key"`
	Tag       RequestTag             `json:"tag"`
	Time      time.Time              `json:"time"`
	Params    map[string]interface{} `json:"params"`
	Buffer    string                 `json:"buffer"`
	Processed bool                   `json:"processed"`
	Completed bool                   `json:"completed"`
	LinkedKey string                 `json:"linked_key"`
	Path      string                 `json:"path"`
	SQL       string                 `json:"sql,omitempty"`
}

// Global Configuration
type Config struct {
	// Server settings
	ServerAddr string `json:"server_addr"`
	ServerPort int    `json:"server_port"`

	// Web interface settings
	WebAddr    string `json:"web_addr"`
	WebPort    int    `json:"web_port"`
	WebEnabled bool   `json:"web_enabled"`

	// Service connections
	HTTPServiceActive              bool     `json:"http_service_active"`
	HTTPServiceIP                  string   `json:"http_service_ip"`
	HTTPServicePort                int      `json:"http_service_port"`
	HTTPServiceName                string   `json:"http_service_name"`
	HTTPServiceTermlistPath        string   `json:"http_service_termlist_path"`
	HTTPServiceIdentPath           string   `json:"http_service_ident_path"`
	HTTPServiceSolarPath           string   `json:"http_service_solar_path"`
	HTTPServiceUIDPath             string   `json:"http_service_uid_path"`
	HTTPServiceUrlFmtSuff          string   `json:"http_service_url_fmt_suff"`
	HTTPServiceRequestExtraHeaders []string `json:"http_service_request_extra_headers"`

	CamServiceActive              bool              `json:"cam_service_active"`
	CamServiceIP                  string            `json:"cam_service_ip"`
	CamServicePort                int               `json:"cam_service_port"`
	CamServiceRequestExtraHeaders map[string]string `json:"cam_service_request_extra_headers"`
	CamAlwaysPass                 bool              `json:"cam_always_pass"`
	CamServiceResultMsgNo         string            `json:"cam_service_result_msg_no"`
	CamServiceResultMsgNf         string            `json:"cam_service_result_msg_nf"`
	CamServiceResultMsgFail       string            `json:"cam_service_result_msg_fail"`

	// Database
	DBServiceIP     string            `json:"db_service_ip"`
	DBServicePort   int               `json:"db_service_port"`
	DBServiceName   string            `json:"db_service_name"`
	DBServicePath   string            `json:"db_service_path"`
	DBServiceParams map[string]string `json:"db_service_params"`

	// Timeouts
	TerminalConnectTimeout   float64 `json:"terminal_connect_timeout"`
	ServiceRequestExpireTime float64 `json:"service_request_expire_time"`
	SessionExpireTime        float64 `json:"session_expire_time"`
	ReconnectionWaitTimeStep float64 `json:"reconnection_wait_time_step"`
	ReconnectionWaitTimeMax  float64 `json:"reconnection_wait_time_max"`

	// Error handling
	ServiceAutofixExpired bool    `json:"service_autofix_expired"`  // Auto-fix on expired requests
	HTTPRequestRetryCount int     `json:"http_request_retry_count"` // Number of retries for HTTP requests
	HTTPRequestRetryDelay float64 `json:"http_request_retry_delay"` // Delay between retries in seconds

	// Messages
	ServiceErrMsg     string `json:"service_err_msg"`
	ServiceFixedMsg   string `json:"service_fixed_msg"`
	ServiceDeniedMsg  string `json:"service_denied_msg"`
	ServiceLinkErrMsg string `json:"service_link_err_msg"`

	// Logging
	LogFile          string  `json:"log_file"`
	LogFileLow       string  `json:"log_file_low"`
	LogFileScreen    string  `json:"log_file_screen"`
	LogFileErr       string  `json:"log_file_err"`
	LogEventCount    int     `json:"log_event_count"`
	LogDevEventCount int     `json:"log_dev_event_count"`
	// Log rotation
	LogRotationEnabled  bool    `json:"log_rotation_enabled"`  // Enable log rotation
	LogRotationMaxSize  int64   `json:"log_rotation_max_size"` // Max file size in bytes (0 = no size limit)
	LogRotationMaxFiles int     `json:"log_rotation_max_files"` // Max number of rotated files to keep (0 = keep all)
	LogRotationMaxDays  int     `json:"log_rotation_max_days"`  // Max age of rotated files in days (0 = no age limit)

	// Global state
	Stats                map[string]interface{} `json:"stats"`
	IDGen                int                    `json:"id_gen"`
	TermListCheckTime    float64                `json:"term_list_check_time"`
	TermListLastCheck    time.Time              `json:"term_list_last_check"`
	TermListFilter       string                 `json:"term_list_filter"`        // Regex filter for terminal IPs
	TermListFilterAbsent bool                   `json:"term_list_filter_absent"` // Invert filter (exclude matching)
	TerminalList         []map[string]interface{} `json:"terminal_list"`         // List of terminals from 1C (filtered)

	// Runtime data
	Connections   map[string]*Connection   `json:"connections"`
	Reconnections map[string]*Reconnection `json:"reconnections"`
	Sessions      map[string]*Session      `json:"sessions"`
	HTTPRequests  map[string]*HTTPRequest  `json:"http_requests"`

	// JSP settings
	JSPListenerPort       interface{} `json:"jsp_listener_port"` // false or int
	JSPDevAutoPingEnabled bool        `json:"jsp_dev_autoping_enabled"`

	// CRT (Vizir) settings
	CRTServiceActive             bool              `json:"crt_service_active"`
	CRTServiceIdentificationMode bool              `json:"crt_service_identification_mode"` // true=identification, false=verification
	CRTServiceIP                 string            `json:"crt_service_ip"`
	CRTServicePort               int               `json:"crt_service_port"`
	CRTServiceName               string            `json:"crt_service_name"`
	CRTServiceURL                string            `json:"crt_service_url"` // e.g. "/vizir/v1/api/"
	CRTServiceConnectTime1       float64           `json:"crt_service_connect_time1"`
	CRTServiceConnectTime2       float64           `json:"crt_service_connect_time2"`
	CRTServiceConnectTime3       float64           `json:"crt_service_connect_time3"`
	CRTServiceExpireTime         float64           `json:"crt_service_expire_time"`
	CRTServiceExtraHeaders       []string          `json:"crt_service_extra_headers"`
	CRTCheckTime                 float64           `json:"crt_check_time"`   // polling interval (0 = disabled)
	CRTLastCheck                 time.Time         `json:"-"`                // runtime: last poll time
	CRTBanCamPidTime             float64           `json:"crt_ban_cam_pid_time"`
	CRTBanPassOnly               bool              `json:"crt_ban_pass_only"`
	CRTBanFromCatch              bool              `json:"crt_ban_from_catch"`
	CRTAutoFixMessage            bool              `json:"crt_auto_fix_message"`
	CRTNoKpoPass                 bool              `json:"crt_no_kpo_pass"`
	CRTSeenTimeout               float64           `json:"crt_seen_timeout"`
	CRTCamLinks                  map[string]string `json:"crt_cam_links"` // camera_id -> terminal_id

	// Phrase fixes: message correction before sending to terminal display
	PhrasesFixes map[string]string `json:"phrases_fixes"` // original -> corrected
}

// Protocol Packet
type Packet struct {
	Cmd     uint8                  `json:"cmd"`
	Code    *uint8                 `json:"code,omitempty"`
	Flags   *uint8                 `json:"flags,omitempty"`
	Payload string                 `json:"payload"`
	Data    map[string]interface{} `json:"data,omitempty"`
}
