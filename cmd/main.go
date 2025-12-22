package main

import (
	"flag"
	"fmt"
	"nd-go/config"
	"nd-go/internal/daemon"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("PANIC: %v\n", r)
			os.Exit(1)
		}
	}()

	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file (default: config.json next to executable)")
	showHelp := flag.Bool("help", false, "Show help message")
	showVersion := flag.Bool("version", false, "Show version")
	createConfig := flag.Bool("create-config", false, "Create example config.json file next to executable")

	// Parse all arguments (including non-flag arguments)
	flag.Parse()

	if *showVersion {
		fmt.Println("СКД - Система контроля доступа")
		fmt.Println("Версия: 1.0.0")
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Create example config if requested
	if *createConfig {
		configPath := "config.json"
		if err := config.SaveConfigExample(configPath); err != nil {
			fmt.Printf("Error: Failed to create config.json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Example configuration file created: %s\n", configPath)
		fmt.Println("Edit this file to customize your settings.")
		os.Exit(0)
	}

	// Parse additional command line arguments (key=value format)
	cmdArgs := make(map[string]string)
	for _, arg := range flag.Args() {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				cmdArgs[key] = value
			}
		}
	}

	fmt.Println("СКД - Система контроля доступа")
	fmt.Println("==============================")

	// Update daemon to use new LoadConfig signature
	fmt.Println("Loading configuration...")

	fmt.Println("Creating daemon...")
	// Create daemon with config
	d := daemon.NewDaemonWithConfig(*configFile, cmdArgs)
	if d == nil {
		fmt.Println("ERROR: Daemon is nil!")
		os.Exit(1)
	}
	fmt.Println("Daemon created successfully")

	// Set up event handlers
	d.GetPool().SetEventHandlers(d.ProcessTagRead, d.ProcessPassEvent)
	fmt.Println("Event handlers set")

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("Signal handlers registered")

	// Start daemon
	fmt.Println("Starting daemon...")
	if err := d.Start(); err != nil {
		fmt.Printf("Failed to start daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Daemon started successfully")

	// Wait for signal
	fmt.Println("Waiting for shutdown signal...")
	select {
	case <-sigChan:
		fmt.Println("\nReceived shutdown signal")
	case <-time.After(10 * time.Second):
		fmt.Println("\nTimeout reached, shutting down...")
	}

	// Stop daemon
	d.Stop()

	fmt.Println("СКД остановлена")
}

// printHelp prints help message
func printHelp() {
	fmt.Println("СКД - Система контроля доступа")
	fmt.Println("")
	fmt.Println("Использование:")
	fmt.Println("  skd.exe [опции] [параметры]")
	fmt.Println("")
	fmt.Println("Опции:")
	fmt.Println("  -config <путь>        Путь к файлу конфигурации (по умолчанию: config.json рядом с exe)")
	fmt.Println("  -create-config         Создать пример файла config.json")
	fmt.Println("  -help                 Показать эту справку")
	fmt.Println("  -version              Показать версию")
	fmt.Println("")
	fmt.Println("Параметры (формат: ключ=значение):")
	fmt.Println("  server.addr=0.0.0.0           Адрес TCP сервера")
	fmt.Println("  server.port=8999              Порт TCP сервера")
	fmt.Println("  web.addr=0.0.0.0              Адрес Web интерфейса")
	fmt.Println("  web.port=8080                 Порт Web интерфейса")
	fmt.Println("  web.enabled=true              Включить Web интерфейс")
	fmt.Println("  http_service.ip=...          IP адрес 1C сервиса")
	fmt.Println("  term_list.filter=...          Фильтр терминалов (regex)")
	fmt.Println("  log.file=log_bin.txt         Файл логов")
	fmt.Println("")
	fmt.Println("Примеры:")
	fmt.Println("  skd.exe -config myconfig.json")
	fmt.Println("  skd.exe server.port=9000 web.port=8081")
	fmt.Println("  skd.exe -create-config")
}
