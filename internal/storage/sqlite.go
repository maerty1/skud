package storage

import (
	"database/sql"
	"fmt"
	"nd-go/pkg/types"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore stores sessions and gtime events in SQLite (replaces CSV).
type SQLiteStore struct {
	dbPath string
	db     *sql.DB
	mutex  sync.Mutex
}

// NewSQLiteStore creates a new SQLite store. Open() must be called before use.
func NewSQLiteStore(dbPath string) *SQLiteStore {
	if dbPath == "" {
		dbPath = "./data/skud.db"
	}
	return &SQLiteStore{dbPath: dbPath}
}

// Open opens or creates the database and initializes tables.
func (s *SQLiteStore) Open() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	dir := filepath.Dir(s.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	s.db = db

	if err := s.initSchema(); err != nil {
		db.Close()
		return err
	}
	return nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *SQLiteStore) initSchema() error {
	// Sessions: same fields as CSV logger
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_time TEXT NOT NULL,
			term_id TEXT,
			term_addr TEXT,
			term_role TEXT,
			uid TEXT,
			kpo_result TEXT,
			kpo_msg TEXT,
			cam_result TEXT,
			cam_cid TEXT,
			final_result TEXT,
			final_msg TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
	`)
	if err != nil {
		return fmt.Errorf("create sessions table: %w", err)
	}

	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS gtime_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT,
			term_id TEXT,
			addres TEXT,
			type TEXT,
			uid TEXT,
			time_val TEXT,
			price TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_gtime_created_at ON gtime_events(created_at);
	`)
	if err != nil {
		return fmt.Errorf("create gtime_events table: %w", err)
	}

	return nil
}

// LogSession writes session data (implements CSVLoggerInterface).
func (s *SQLiteStore) LogSession(session *types.Session, conn *types.Connection) error {
	data := s.prepareSessionData(session, conn)

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.db == nil {
		return fmt.Errorf("db closed")
	}

	_, err := s.db.Exec(`
		INSERT INTO sessions (session_time, term_id, term_addr, term_role, uid, kpo_result, kpo_msg, cam_result, cam_cid, final_result, final_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		data["session_time"], data["term_id"], data["term_addr"], data["term_role"], data["uid"],
		data["kpo_result"], data["kpo_msg"], data["cam_result"], data["cam_cid"], data["final_result"], data["final_msg"],
	)
	return err
}

func (s *SQLiteStore) prepareSessionData(session *types.Session, conn *types.Connection) map[string]string {
	data := make(map[string]string)
	if !session.ReqTime.IsZero() {
		data["session_time"] = session.ReqTime.Format("02.01.06 15:04:05")
	} else {
		data["session_time"] = time.Now().Format("02.01.06 15:04:05")
	}
	data["term_id"] = ""
	data["term_addr"] = ""
	data["term_role"] = ""
	if conn != nil && conn.Settings != nil {
		data["term_id"] = conn.Settings.ID
		data["term_addr"] = fmt.Sprintf("%s:%d", conn.Settings.IP, conn.Settings.Port)
		if role, ok := conn.Settings.Extra["role"].(string); ok {
			data["term_role"] = role
		}
	} else if conn != nil {
		data["term_addr"] = conn.Key
	}
	data["uid"] = session.UID

	kpoResult := "UNDEF"
	if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
		if result, ok := kpoData["result"].(types.KPOResult); ok {
			kpoResult = kpoResultStr(result)
		}
	}
	data["kpo_result"] = kpoResult
	kpoMsg := ""
	if kpoData, ok := session.Data["kpo"].(map[string]interface{}); ok {
		if msg, ok := kpoData["message"].(string); ok {
			kpoMsg = nl2comma(msg)
		}
	}
	data["kpo_msg"] = kpoMsg

	camResult := "UNDEF"
	if camData, ok := session.Data["cam"].(map[string]interface{}); ok {
		if result, ok := camData["result"].(types.CamResult); ok {
			camResult = camResultStr(result)
		}
	}
	data["cam_result"] = camResult
	data["cam_cid"] = session.CID

	finalResult := "NO"
	if result, ok := session.Data["result"].(int); ok && result > 0 {
		finalResult = "YES"
	}
	data["final_result"] = finalResult
	finalMsg := ""
	if msg, ok := session.Data["message"].(string); ok {
		finalMsg = nl2comma(msg)
	}
	data["final_msg"] = finalMsg
	return data
}

func kpoResultStr(r types.KPOResult) string {
	switch r {
	case types.KPO_RES_YES: return "YES"
	case types.KPO_RES_NO: return "NO"
	case types.KPO_RES_FAIL: return "FAIL"
	default: return "UNDEF"
	}
}

func camResultStr(r types.CamResult) string {
	switch r {
	case types.CAM_RES_YES: return "YES"
	case types.CAM_RES_NO: return "NO"
	case types.CAM_RES_NF: return "NF"
	case types.CAM_RES_FAIL: return "FAIL"
	default: return "UNDEF"
	}
}

func nl2comma(s string) string {
	s = strings.ReplaceAll(s, "\r\n", ";")
	s = strings.ReplaceAll(s, "\n", ";")
	s = strings.ReplaceAll(s, "\r", ";")
	return s
}

// RegisterGTimeEvent logs a GTime (solar) event.
func (s *SQLiteStore) RegisterGTimeEvent(data map[string]string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.db == nil {
		return nil
	}

	ts := data["timestamp"]
	tid := data["id"]
	addr := data["addres"]
	typ := data["type"]
	uid := data["uid"]
	timeVal := data["time"]
	price := data["price"]

	_, err := s.db.Exec(`
		INSERT INTO gtime_events (timestamp, term_id, addres, type, uid, time_val, price) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ts, tid, addr, typ, uid, timeVal, price)
	return err
}

// GetSessionsSince returns session rows since the given time (for email digest).
func (s *SQLiteStore) GetSessionsSince(since time.Time) ([]map[string]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT session_time, term_id, term_addr, term_role, uid, kpo_result, kpo_msg, cam_result, cam_cid, final_result, final_msg
		FROM sessions WHERE datetime(created_at) >= datetime(?) ORDER BY created_at ASC`, since.Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := []string{"session_time", "term_id", "term_addr", "term_role", "uid", "kpo_result", "kpo_msg", "cam_result", "cam_cid", "final_result", "final_msg"}
	var result []map[string]string
	for rows.Next() {
		row := make(map[string]string)
		vals := make([]interface{}, len(cols))
		for i := range vals {
			var s string
			vals[i] = &s
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, err
		}
		for i, c := range cols {
			if v, ok := vals[i].(*string); ok && v != nil {
				row[c] = *v
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
