package handler

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

// HandlerFunc represents command handler function
type HandlerFunc func(params map[string]interface{}) string

// HandlerManager manages command handlers
type HandlerManager struct {
	handlers map[string]HandlerFunc
}

// NewHandlerManager creates new handler manager
func NewHandlerManager() *HandlerManager {
	return &HandlerManager{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register registers command handler
func (hm *HandlerManager) Register(command string, handler HandlerFunc) {
	hm.handlers[strings.ToLower(command)] = handler
}

// Execute executes command
func (hm *HandlerManager) Execute(command string, params map[string]interface{}) string {
	if params == nil {
		params = make(map[string]interface{})
	}

	// Parse command
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return "ERROR: Empty command"
	}

	cmdName := strings.ToLower(parts[0])

	// Parse parameters
	for i := 1; i < len(parts); i++ {
		if strings.Contains(parts[i], "=") {
			kv := strings.SplitN(parts[i], "=", 2)
			if len(kv) == 2 {
				params[strings.ToLower(kv[0])] = kv[1]
			}
		} else {
			params[fmt.Sprintf("arg%d", i)] = parts[i]
		}
	}

	// Execute handler
	if handler, exists := hm.handlers[cmdName]; exists {
		return handler(params)
	}

	return fmt.Sprintf("ERROR: Unknown command '%s'", cmdName)
}

// GetCommands returns list of available commands
func (hm *HandlerManager) GetCommands() []string {
	commands := make([]string, 0, len(hm.handlers))
	for cmd := range hm.handlers {
		commands = append(commands, cmd)
	}
	return commands
}

// AutoRegister automatically registers all handler methods
func (hm *HandlerManager) AutoRegister(handler interface{}) {
	v := reflect.ValueOf(handler)
	t := reflect.TypeOf(handler)

	for i := 0; i < v.NumMethod(); i++ {
		method := v.Method(i)
		methodType := t.Method(i)

		// Check if method has correct signature
		if methodType.Type.NumIn() == 2 &&
			methodType.Type.NumOut() == 1 &&
			methodType.Type.In(1).String() == "map[string]interface {}" &&
			methodType.Type.Out(0).String() == "string" {

			// Get method name
			methodName := strings.ToLower(methodType.Name)

			// Create wrapper function
			wrapper := func(params map[string]interface{}) string {
				args := []reflect.Value{reflect.ValueOf(params)}
				result := method.Call(args)
				return result[0].String()
			}

			hm.Register(methodName, wrapper)
		}
	}
}

// GetFunctionName gets function name from function pointer
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
