package main

import (
	"fmt"
	"nd-go/config"
	"nd-go/internal/httpclient"
	"nd-go/pkg/utils"
	"os"
	"strings"
)

func main() {
	fmt.Println("=== Testing Terminal List Retrieval from 1C ===\n")

	// Load configuration
	cfg := config.LoadConfig()

	// Display configuration
	fmt.Println("Configuration:")
	fmt.Printf("  HTTPServiceActive: %v\n", cfg.HTTPServiceActive)
	fmt.Printf("  HTTPServiceName: %s\n", cfg.HTTPServiceName)
	fmt.Printf("  HTTPServicePort: %d\n", cfg.HTTPServicePort)
	fmt.Printf("  HTTPServiceTermlistPath: %s\n", cfg.HTTPServiceTermlistPath)
	fmt.Printf("  HTTPServiceUrlFmtSuff: %s\n", cfg.HTTPServiceUrlFmtSuff)
	fmt.Printf("  Extra Headers: %d header(s)\n", len(cfg.HTTPServiceRequestExtraHeaders))
	for i, header := range cfg.HTTPServiceRequestExtraHeaders {
		// Mask authorization header
		if len(header) > 20 {
			masked := header[:20] + "..."
			fmt.Printf("    [%d] %s\n", i+1, masked)
		} else {
			fmt.Printf("    [%d] %s\n", i+1, header)
		}
	}
	fmt.Println()

	// Create HTTP client
	client := httpclient.NewHTTPClient(cfg)

	// Test URL construction
	testURL := fmt.Sprintf("http://%s%s", cfg.HTTPServiceName, cfg.HTTPServiceTermlistPath)
	fmt.Printf("Request URL: %s\n", testURL)
	fmt.Println()

	// Try to get terminal list
	fmt.Println("Attempting to get terminal list from 1C...")
	terminals, err := client.GetTerminalList()
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to get terminal list: %v\n", err)
		os.Exit(1)
	}

	// Display results
	fmt.Printf("✅ SUCCESS: Received %d terminal(s) from 1C\n\n", len(terminals))

	if len(terminals) == 0 {
		fmt.Println("⚠️  WARNING: Terminal list is empty!")
		os.Exit(0)
	}

	// Apply filter if configured
	filter := cfg.TermListFilter
	filterAbsent := cfg.TermListFilterAbsent
	
	if filter != "" {
		fmt.Printf("Filter: %s (invert: %v)\n", filter, filterAbsent)
		fmt.Println()
	}

	filteredTerminals := make([]map[string]interface{}, 0)
	for _, term := range terminals {
		ip := getStringValue(term, "IP", "ip", "")
		if ip == "" {
			continue
		}

		// Apply filter
		if !utils.FilterTerminalList(ip, filter, filterAbsent) {
			continue
		}

		filteredTerminals = append(filteredTerminals, term)
	}

	fmt.Printf("After filtering: %d terminal(s)\n\n", len(filteredTerminals))

	if len(filteredTerminals) == 0 {
		fmt.Println("⚠️  WARNING: No terminals match the filter!")
		os.Exit(0)
	}

	fmt.Println("Terminals:")
	fmt.Println("─" + strings.Repeat("─", 80))
	for i, term := range filteredTerminals {
		fmt.Printf("\n[%d] Terminal Data:\n", i+1)
		
		// Try to extract common fields
		id := getStringValue(term, "ID", "id", "unknown")
		ip := getStringValue(term, "IP", "ip", "unknown")
		port := getStringValue(term, "PORT", "port", "0")
		termType := getStringValue(term, "TYPE", "type", "unknown")
		
		fmt.Printf("  ID:   %s\n", id)
		fmt.Printf("  IP:   %s\n", ip)
		fmt.Printf("  Port: %s\n", port)
		fmt.Printf("  Type: %s\n", termType)
		
		// Show all fields
		fmt.Printf("  All fields:\n")
		for key, value := range term {
			fmt.Printf("    %s: %v\n", key, value)
		}
	}
	fmt.Println("\n" + strings.Repeat("─", 80))
	fmt.Println("✅ Test completed successfully!")
}

func getStringValue(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok {
			if str, ok := val.(string); ok {
				return str
			}
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

