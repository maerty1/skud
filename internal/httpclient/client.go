package httpclient

import (
	"encoding/json"
	"fmt"
	"io"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
	"net/http"
	"strings"
	"time"
)

// HTTPClientInterface defines HTTP client methods
type HTTPClientInterface interface {
	CheckAccess(uid string, terminalID string, tagType string, lockers []types.LockerInfo) (*types.KPOResult, string, error)
	SendAccessReport(uid string, terminalID string, result bool, message string) error
	GetUserCID(uid string) (string, error)
}

// HTTPClient represents HTTP client for external services
type HTTPClient struct {
	client *http.Client
	config *types.Config
}

// HTTPResponse represents HTTP response
type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Data       map[string]interface{}
}

// NewHTTPClient creates new HTTP client
func NewHTTPClient(config *types.Config) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: time.Duration(config.ServiceRequestExpireTime * float64(time.Second)),
		},
		config: config,
	}
}

// Request1C sends request to 1C service with retry mechanism
func (hc *HTTPClient) Request1C(path string, params map[string]interface{}) (*HTTPResponse, error) {
	var lastErr error
	maxRetries := hc.config.HTTPRequestRetryCount
	if maxRetries < 0 {
		maxRetries = 0
	}
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			delay := time.Duration(hc.config.HTTPRequestRetryDelay * float64(time.Second))
			time.Sleep(delay)
		}
		
		resp, err := hc.request1COnce(path, params)
		if err == nil {
			return resp, nil
		}
		
		lastErr = err
		// Log retry attempt
		if attempt < maxRetries {
			fmt.Printf("HTTP request failed (attempt %d/%d), retrying: %v\n", attempt+1, maxRetries+1, err)
		}
	}
	
	return nil, fmt.Errorf("HTTP request failed after %d attempts: %v", maxRetries+1, lastErr)
}

// request1COnce sends single HTTP request to 1C service
func (hc *HTTPClient) request1COnce(path string, params map[string]interface{}) (*HTTPResponse, error) {
	url := fmt.Sprintf("http://%s%s", hc.config.HTTPServiceName, path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set default headers
	req.Header.Set("Host", hc.config.HTTPServiceName)
	req.Header.Set("Connection", "close")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.99 Safari/537.36")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.8,en-US;q=0.6,en;q=0.4")

	// Add extra headers from configuration (including Authorization)
	if hc.config.HTTPServiceActive && len(hc.config.HTTPServiceRequestExtraHeaders) > 0 {
		for _, header := range hc.config.HTTPServiceRequestExtraHeaders {
			// Parse header in format "Key: Value"
			parts := strings.SplitN(header, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				req.Header.Set(key, value)
			}
		}
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse JSON response
	var data map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &data); err != nil {
			// If not JSON, create basic response
			data = map[string]interface{}{
				"raw_response": string(body),
			}
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Data:       data,
	}, nil
}

// GetTerminalList requests terminal list from 1C
func (hc *HTTPClient) GetTerminalList() ([]map[string]interface{}, error) {
	path := hc.config.HTTPServiceTermlistPath
	resp, err := hc.Request1C(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get terminal list: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("terminal list request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}

	// Try different response formats
	var terminals []interface{}
	var ok bool

	// Format 1: {"terminals": [...]}
	if terminals, ok = resp.Data["terminals"].([]interface{}); !ok {
		// Format 2: {"DEVICES": [...]}
		if terminals, ok = resp.Data["DEVICES"].([]interface{}); !ok {
			// Format 3: Direct array response
			if arr, isArr := resp.Data[""].([]interface{}); isArr {
				terminals = arr
				ok = true
			} else {
				// Format 4: Try to parse as array directly
				var dataArray []interface{}
				if err := json.Unmarshal(resp.Body, &dataArray); err == nil {
					terminals = dataArray
					ok = true
				}
			}
		}
	}

	if !ok || terminals == nil {
		return nil, fmt.Errorf("no terminals array in response. Response data: %+v, Body: %s", resp.Data, string(resp.Body))
	}

	result := make([]map[string]interface{}, 0, len(terminals))
	for i, t := range terminals {
		if termData, ok := t.(map[string]interface{}); ok {
			// Normalize field names to uppercase (as in PHP json_as_rows)
			normalized := make(map[string]interface{})
			for k, v := range termData {
				normalized[strings.ToUpper(k)] = v
			}
			// Also keep original keys
			for k, v := range termData {
				normalized[k] = v
			}
			result = append(result, normalized)
		} else {
			return nil, fmt.Errorf("terminal at index %d is not an object: %+v", i, t)
		}
	}

	return result, nil
}

// CheckAccess checks user access via 1C
// tagType: "rfid", "qr", "faceid" - determines data type
// role: optional role parameter for craft format
func (hc *HTTPClient) CheckAccess(uid string, terminalID string, tagType string, lockers []types.LockerInfo) (*types.KPOResult, string, error) {
	return hc.CheckAccessWithRole(uid, terminalID, tagType, "", lockers)
}

// CheckAccessWithRole checks user access via 1C with optional role parameter
func (hc *HTTPClient) CheckAccessWithRole(uid string, terminalID string, tagType string, role string, lockers []types.LockerInfo) (*types.KPOResult, string, error) {
	var path string

	// Normalize tagType (default to rfid)
	if tagType == "" {
		tagType = "rfid"
	}

	// Format lockers based on URL format
	var lockersStr string
	switch hc.config.HTTPServiceUrlFmtSuff {
	case "wc1c":
		// Format: /id/uid/0/0/0/lockers/0
		// Parameters: tagtype(0=rfid), bio(0), other(0), lockers, other(0)
		_, lockersDataF := utils.ProcessLockersData(lockers)
		lockersStr = utils.FormatLockersList(lockersDataF)
		// Map tagType to number: rfid=0, qr=1, faceid=2 (assuming)
		tagTypeNum := "0"
		if tagType == "qr" || tagType == "barcode" {
			tagTypeNum = "1"
		} else if tagType == "faceid" {
			tagTypeNum = "2"
		}
		path = fmt.Sprintf("%s/%s/%s/%s/0/0/%s/0", hc.config.HTTPServiceIdentPath, terminalID, uid, tagTypeNum, lockersStr)
	case "a&a":
		// Format: /verify/id/uid
		path = fmt.Sprintf("%s/verify/%s/%s", hc.config.HTTPServiceIdentPath, terminalID, uid)
	case "1c_m":
		// Format: /checkaccess?id=...&uid=...&tagtype=...
		path = fmt.Sprintf("%s/checkaccess?id=%s&uid=%s&tagtype=%s", hc.config.HTTPServiceIdentPath, terminalID, uid, tagType)
		// Note: 1c_m format doesn't use lockers in URL (cells parameter not in original)
	case "1c_m_":
		// Format: /checkaccess?id=...&uid=...&cells=...
		lockersStr = utils.FormatLockersList1CM(lockers)
		path = fmt.Sprintf("%s/checkaccess?id=%s&uid=%s&cells=%s", hc.config.HTTPServiceIdentPath, terminalID, uid, lockersStr)
	case "craft":
		// Format: /pass_request?id=...&uid=...&role=...&locks=...
		lockersStr = utils.FormatLockersListCraft(lockers)
		if role == "" {
			role = "0" // Default role
		}
		path = fmt.Sprintf("/pass_request?id=%s&uid=%s&role=%s&locks=%s", terminalID, uid, role, lockersStr)
	default:
		// Format: /checking.php?id=...&uid=...&lockers=...
		_, lockersDataF := utils.ProcessLockersData(lockers)
		lockersStr = utils.FormatLockersList(lockersDataF)
		path = fmt.Sprintf("%s/checking.php?id=%s&uid=%s&lockers=%s", hc.config.HTTPServiceIdentPath, terminalID, uid, lockersStr)
	}

	resp, err := hc.Request1C(path, nil)
	if err != nil {
		return nil, "", fmt.Errorf("access check failed: %v", err)
	}

	// Parse response based on format
	var result types.KPOResult
	var message string

	if resp.StatusCode == 500 {
		result = types.KPO_RES_NO
		message = hc.config.ServiceDeniedMsg
	} else {
		// Try to parse different response formats
		if resultVal, ok := resp.Data["RESULTVAL"].(float64); ok {
			if int(resultVal) > 0 {
				result = types.KPO_RES_YES
			} else {
				result = types.KPO_RES_NO
			}
			message = utils.GetStringValue(resp.Data, "MSGSTR", hc.config.ServiceFixedMsg)
		} else if resultVal, ok := resp.Data["RESULT"].(float64); ok {
			if int(resultVal) > 0 {
				result = types.KPO_RES_YES
			} else {
				result = types.KPO_RES_NO
			}
			if msg, ok := resp.Data["MESSAGE"].(string); ok {
				message = msg
			} else if msg, ok := resp.Data["DENYREASON"].(string); ok {
				message = msg
			} else {
				message = hc.config.ServiceFixedMsg
			}
		} else if grantAccess, ok := resp.Data["GRANT_ACCESS"].(float64); ok {
			if int(grantAccess) > 0 {
				result = types.KPO_RES_YES
			} else {
				result = types.KPO_RES_NO
			}
			message = utils.GetStringValue(resp.Data, "TEXT", hc.config.ServiceFixedMsg)
		} else {
			// Unknown format, assume success
			result = types.KPO_RES_YES
			message = hc.config.ServiceFixedMsg
		}
	}

	if message == "" {
		if result == types.KPO_RES_YES {
			message = hc.config.ServiceFixedMsg
		} else {
			message = hc.config.ServiceDeniedMsg
		}
	}

	return &result, message, nil
}

// SendAccessReport sends access event report to 1C
// tagType: "rfid", "qr", "faceid" - determines data type
// role: optional role parameter for craft format
func (hc *HTTPClient) SendAccessReport(uid string, terminalID string, result bool, message string) error {
	return hc.SendAccessReportWithParams(uid, terminalID, result, message, "rfid", "")
}

// SendAccessReportWithParams sends access event report to 1C with additional parameters
func (hc *HTTPClient) SendAccessReportWithParams(uid string, terminalID string, result bool, message string, tagType string, role string) error {
	var path string

	// Normalize tagType
	if tagType == "" {
		tagType = "rfid"
	}

	switch hc.config.HTTPServiceUrlFmtSuff {
	case "wc1c":
		// Format: /id/uid/1/0/0/0/0 (1 = report, 0/0/0/0 = other params)
		regParam := "0"
		if result {
			regParam = "1"
		}
		path = fmt.Sprintf("%s/%s/%s/%s/0/0/0/0", hc.config.HTTPServiceIdentPath, terminalID, uid, regParam)
	case "a&a":
		// Format: /check/id/uid
		path = fmt.Sprintf("%s/check/%s/%s", hc.config.HTTPServiceIdentPath, terminalID, uid)
	case "1c_m":
		// Format: /event?id=...&uid=...&tagtype=...
		path = fmt.Sprintf("%s/event?id=%s&uid=%s&tagtype=%s", hc.config.HTTPServiceIdentPath, terminalID, uid, tagType)
	case "1c_m_":
		// Format: /event?id=...&uid=...
		path = fmt.Sprintf("%s/event?id=%s&uid=%s", hc.config.HTTPServiceIdentPath, terminalID, uid)
	case "craft":
		// Format: /pass_register?id=...&uid=...&role=...
		if role == "" {
			role = "0" // Default role
		}
		path = fmt.Sprintf("/pass_register?id=%s&uid=%s&role=%s", terminalID, uid, role)
	default:
		// Format: /checking.php?id=...&uid=...&reg=1
		regParam := ""
		if result {
			regParam = "&reg=1"
		}
		path = fmt.Sprintf("%s/checking.php?id=%s&uid=%s%s", hc.config.HTTPServiceIdentPath, terminalID, uid, regParam)
	}

	resp, err := hc.Request1C(path, nil)
	if err != nil {
		return fmt.Errorf("access report failed: %v", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("access report failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetUserCID gets user client ID from 1C
func (hc *HTTPClient) GetUserCID(uid string) (string, error) {
	path := fmt.Sprintf("%s/%s", hc.config.HTTPServiceUIDPath, uid)

	resp, err := hc.Request1C(path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get user CID: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("user CID request failed with status %d", resp.StatusCode)
	}

	// Try different field names
	if cid, ok := resp.Data["CID"].(string); ok {
		return cid, nil
	}
	if cid, ok := resp.Data["CLIENT_ID"].(string); ok {
		return cid, nil
	}
	if cid, ok := resp.Data["BIOID"].(string); ok {
		return cid, nil
	}

	return "", fmt.Errorf("CID not found in response")
}
