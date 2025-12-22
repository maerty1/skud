package pocket

// POCKET_CMD_INTERACTIVE is the Interactive command code
const POCKET_CMD_INTERACTIVE = 0x0A

// POCKET_PK_FLAGS_RT_MAIN is the main reader type flag
const POCKET_PK_FLAGS_RT_MAIN = 0x10

// CreateLockPacket creates a lock packet for POCKET terminal
// text: optional text to display (empty string for unlock)
func CreateLockPacket(text string) []byte {
	var payload []byte
	
	if text != "" {
		// Lock: show waiting with optional text
		payload = InteractiveWaiting(1500, true, false)
		if text != "" {
			payload = append(InteractiveText(text), payload...)
		}
	} else {
		// Unlock: clear waiting
		payload = InteractiveWaiting(0, false, false)
	}
	
	// Create packet with Interactive command
	flags := POCKET_PK_FLAGS_RT_MAIN
	return EncodePacket(POCKET_CMD_INTERACTIVE, uint8(flags), string(payload))
}

// CreateUnlockPacket creates an unlock packet for POCKET terminal
func CreateUnlockPacket() []byte {
	return CreateLockPacket("")
}

