package connection

import (
	"fmt"
	"nd-go/internal/protocols/pocket"
	"nd-go/pkg/types"
	"nd-go/pkg/utils"
)

// handleMemRegDevice handles MEMREG device (towel/add, towel/take) terminals
func (cp *ConnectionPool) handleMemRegDevice(conn *Connection, packet *types.Packet) {
	if conn.Settings == nil || conn.Settings.MemRegDev == "" {
		return
	}

	uid, ok := packet.Data["uid"].(string)
	if !ok || uid == "" {
		fmt.Printf("No UID in MEMREG device read from %s\n", conn.Key)
		return
	}

	// Parse MEMREG device key (e.g., "towel/add", "towel/take")
	memregKey, err := utils.ParseMemRegKey(conn.Settings.MemRegDev)
	if err != nil {
		fmt.Printf("Invalid MEMREG device key %s: %v\n", conn.Settings.MemRegDev, err)
		return
	}

	memregStorage := utils.GetMemRegStorage()

	fmt.Printf("MEMREG device: key=%s, storage=%s, mode=%d, uid=%s\n",
		conn.Key, memregKey.Storage, memregKey.Mode, uid)

	// Parse UID key
	uidKey, err := utils.ParseMemRegKey(uid)
	if err != nil {
		fmt.Printf("Invalid UID key for MEMREG: %v\n", err)
		return
	}

	// Check current value in storage
	hasValue, err := memregStorage.Has(memregKey.Storage, uidKey.Storage)
	if err != nil {
		fmt.Printf("Error checking MEMREG storage: %v\n", err)
		return
	}

	// Determine action based on mode
	var message string
	var result bool
	var sound int = 3 // Default sound (error)

	switch memregKey.Mode {
	case utils.MEMREG_MODE_AUTO:
		// Auto mode: toggle state
		if !hasValue {
			// Set (add)
			if err := memregStorage.Set(memregKey.Storage, uidKey.Storage, true); err == nil {
				message = getMemRegMessage(memregKey.Storage, "set")
				result = true
				sound = 1 // Success sound
			} else {
				message = "Ошибка установки отметки"
			}
		} else {
			// Clear (take)
			if err := memregStorage.Del(memregKey.Storage, uidKey.Storage); err == nil {
				message = getMemRegMessage(memregKey.Storage, "clr")
				result = true
				sound = 1 // Success sound
			} else {
				message = "Ошибка снятия отметки"
			}
		}

	case utils.MEMREG_MODE_SET, utils.MEMREG_MODE_DISP:
		// Set mode (add): only if not already set
		if !hasValue {
			if err := memregStorage.Set(memregKey.Storage, uidKey.Storage, true); err == nil {
				message = getMemRegMessage(memregKey.Storage, "set")
				result = true
				sound = 1
			} else {
				message = "Ошибка установки отметки"
			}
		} else {
			message = getMemRegMessage(memregKey.Storage, "info_set")
			result = false
			sound = 3
		}

	case utils.MEMREG_MODE_CLR, utils.MEMREG_MODE_TAKE:
		// Clear mode (take): only if already set
		if hasValue {
			if err := memregStorage.Del(memregKey.Storage, uidKey.Storage); err == nil {
				message = getMemRegMessage(memregKey.Storage, "clr")
				result = true
				sound = 1
			} else {
				message = "Ошибка снятия отметки"
			}
		} else {
			message = getMemRegMessage(memregKey.Storage, "info_clr")
			result = false
			sound = 3
		}

	default:
		message = "Неизвестный режим MEMREG"
		result = false
	}

	// Send interactive message to terminal
	readerType, _ := packet.Data["reader_type"].(uint8)
	_ = readerType // May be used later
	interactivePayload := pocket.CreateInteractivePacket(message, 3000, sound, true)
	// Interactive command = 0x06, flags = reader type (use from packet)
	var flags uint8 = 0x00
	if readerType != 0 {
		flags = readerType
	}
	pkt := pocket.CreatePacket(0x06, flags, interactivePayload)
	cp.Send(conn.Key, pkt)

	fmt.Printf("MEMREG processed: storage=%s, uid=%s, result=%v, message=%s\n",
		memregKey.Storage, uid, result, message)
}

// getMemRegMessage returns localized message for MEMREG operation
func getMemRegMessage(storage, msgType string) string {
	// Default messages
	messages := map[string]map[string]string{
		"towel": {
			"set":      "Полотенце\n[ВЫДАНО]\nУСПЕШНО",
			"clr":      "Полотенце\n[СДАНО]\nУСПЕШНО",
			"info_set": "Ошибка\nПолотенце:\nуже было ВЫДАНО",
			"info_clr": "Полотенце:\n[НЕ ВЫДАНО]",
			"do_set":   "Возьмите полотенце",
			"do_clr":   "Сдайте полотенце",
			"denied":   "СДАЙТЕ\nПОЛОТЕНЦЕ",
		},
		"default": {
			"set":      "Отметка\n[УСТАНОВЛЕНА]\nУСПЕШНО",
			"clr":      "Отметка\n[СНЯТА]\nУСПЕШНО",
			"info_set": "Статус отметки:\n[УСТАНОВЛЕНА]",
			"info_clr": "Статус отметки:\n[СНЯТА]",
			"do_set":   "Совершите действие",
			"do_clr":   "Совершите действие",
			"denied":   "СНИМИТЕ\nОТМЕТКУ",
		},
	}

	if storageMsgs, ok := messages[storage]; ok {
		if msg, ok := storageMsgs[msgType]; ok {
			return msg
		}
	}

	if msg, ok := messages["default"][msgType]; ok {
		return msg
	}

	return "Ошибка"
}

