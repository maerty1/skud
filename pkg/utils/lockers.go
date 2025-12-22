package utils

import (
	"fmt"
	"nd-go/pkg/types"
	"regexp"
	"strconv"
	"strings"
)

// ProcessLockersData processes lockers data and formats it for different output formats
func ProcessLockersData(lockersData []types.LockerInfo) ([]string, []string) {
	// lockers_data: array of [block_no, cab_no] pairs
	// lockers_data_f: array of formatted strings
	lockersDataArray := []string{}
	lockersDataF := []string{}

	for _, locker := range lockersData {
		// Only include lockers with no errors and locked status
		if locker.AuthErr == 0 && locker.ReadErr == 0 && locker.Locked && locker.CabNo > 0 {
			var blockNoStr string
			if locker.IsPasstech {
				blockNoStr = locker.Litera
			} else {
				blockNoStr = strconv.Itoa(int(locker.BlockNo))
			}
			
			// Format: [block_no, cab_no]
			lockersDataArray = append(lockersDataArray, fmt.Sprintf("%s:%d", blockNoStr, locker.CabNo))
			
			// Format for lockers_data_f: string representation
			if locker.IsPasstech {
				lockersDataF = append(lockersDataF, fmt.Sprintf("%s%d", locker.Litera, locker.CabNo))
			} else {
				lockersDataF = append(lockersDataF, strconv.Itoa(int(locker.CabNo)))
			}
		}
	}

	return lockersDataArray, lockersDataF
}

// FormatLockersList formats lockers list for wc1c format
// Returns comma-separated list of numbers only, or "0" if empty
func FormatLockersList(lockersDataF []string) string {
	if len(lockersDataF) == 0 {
		return "0"
	}
	
	// Extract only digits from each item
	result := []string{}
	for _, item := range lockersDataF {
		// Remove all non-digit characters
		re := regexp.MustCompile(`[^\d]`)
		digits := re.ReplaceAllString(item, "")
		if digits != "" {
			result = append(result, digits)
		}
	}
	
	if len(result) == 0 {
		return "0"
	}
	
	return strings.Join(result, ",")
}

// FormatLockersListCraft formats lockers list for craft format
// Returns comma-separated list of "block_no:cab_no" pairs
func FormatLockersListCraft(lockersData []types.LockerInfo) string {
	if len(lockersData) == 0 {
		return ""
	}
	
	result := []string{}
	for _, locker := range lockersData {
		if locker.AuthErr == 0 && locker.ReadErr == 0 && locker.Locked && locker.CabNo > 0 {
			var blockNoStr string
			if locker.IsPasstech {
				blockNoStr = locker.Litera
			} else {
				blockNoStr = strconv.Itoa(int(locker.BlockNo))
			}
			result = append(result, fmt.Sprintf("%s:%d", blockNoStr, locker.CabNo))
		}
	}
	
	return strings.Join(result, ",")
}

// FormatLockersList1CM formats lockers list for 1c_m format
// Returns comma-separated list of "block_no:cab_no" pairs, or "0" if empty
func FormatLockersList1CM(lockersData []types.LockerInfo) string {
	if len(lockersData) == 0 {
		return "0"
	}
	
	result := []string{}
	for _, locker := range lockersData {
		if locker.AuthErr == 0 && locker.ReadErr == 0 && locker.Locked && locker.CabNo > 0 {
			var blockNoStr string
			if locker.IsPasstech {
				blockNoStr = locker.Litera
			} else {
				blockNoStr = strconv.Itoa(int(locker.BlockNo))
			}
			result = append(result, fmt.Sprintf("%s:%d", blockNoStr, locker.CabNo))
		}
	}
	
	if len(result) == 0 {
		return "0"
	}
	
	return strings.Join(result, ",")
}

// TransformJSPLockersData transforms JSP lockers data from string to array format
// Input: "62:180,33:26" or similar
// Output: array of LockerInfo
func TransformJSPLockersData(lockersStr string) []types.LockerInfo {
	if lockersStr == "" {
		return nil
	}
	
	// Split by comma or semicolon
	re := regexp.MustCompile(`[,;]+`)
	parts := re.Split(lockersStr, -1)
	
	result := []types.LockerInfo{}
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		// Match pattern: (([A-Z\d]+):)?([\d]+)
		re := regexp.MustCompile(`^(([A-Z\d]+):)?([\d]+)$`)
		matches := re.FindStringSubmatch(part)
		if len(matches) < 4 {
			continue
		}
		
		litera := matches[2] // Optional letter/block
		cabNoStr := matches[3]
		
		cabNo, err := strconv.ParseUint(cabNoStr, 10, 16)
		if err != nil {
			continue
		}
		
		locker := types.LockerInfo{
			AuthErr:   0,
			ReadErr:   0,
			IsPasstech: litera != "",
			BlockNo:   0,
			Litera:    litera,
			Locked:    true,
			CabNo:     uint16(cabNo),
		}
		
		if litera != "" {
			// Try to parse as block number
			if blockNo, err := strconv.ParseUint(litera, 10, 8); err == nil {
				locker.BlockNo = uint8(blockNo)
				locker.IsPasstech = false
			} else if len(litera) == 1 && litera[0] >= 'A' && litera[0] <= 'Z' {
				locker.BlockNo = uint8(litera[0] - 'A' + 1)
				locker.IsPasstech = true
			}
		}
		
		result = append(result, locker)
	}
	
	return result
}

