package hotkey

// Linux input subsystem event types (from linux/input-event-codes.h)
const (
	EvdevTypeSyn uint16 = 0x00
	EvdevTypeKey uint16 = 0x01
)

// Linux key press values
const (
	EvdevValueRelease    int32 = 0
	EvdevValuePress      int32 = 1
	EvdevValueAutorepeat int32 = 2
)

// Linux evdev standard keycodes
const (
	KeyEsc        uint16 = 1
	Key1          uint16 = 2
	Key2          uint16 = 3
	Key3          uint16 = 4
	Key4          uint16 = 5
	Key5          uint16 = 6
	Key6          uint16 = 7
	Key7          uint16 = 8
	Key8          uint16 = 9
	Key9          uint16 = 10
	Key0          uint16 = 11
	KeyMinus      uint16 = 12
	KeyEqual      uint16 = 13
	KeyBackspace  uint16 = 14
	KeyTab        uint16 = 15
	KeyQ          uint16 = 16
	KeyW          uint16 = 17
	KeyE          uint16 = 18
	KeyR          uint16 = 19
	KeyT          uint16 = 20
	KeyY          uint16 = 21
	KeyU          uint16 = 22
	KeyI          uint16 = 23
	KeyO          uint16 = 24
	KeyP          uint16 = 25
	KeyLeftBrace  uint16 = 26
	KeyRightBrace uint16 = 27
	KeyEnter      uint16 = 28
	KeyLeftCtrl   uint16 = 29
	KeyA          uint16 = 30
	KeyS          uint16 = 31
	KeyD          uint16 = 32
	KeyF          uint16 = 33
	KeyG          uint16 = 34
	KeyH          uint16 = 35
	KeyJ          uint16 = 36
	KeyK          uint16 = 37
	KeyL          uint16 = 38
	KeySemicolon  uint16 = 39
	KeyApostrophe uint16 = 40
	KeyGrave      uint16 = 41
	KeyLeftShift  uint16 = 42
	KeyBackslash  uint16 = 43
	KeyZ          uint16 = 44
	KeyX          uint16 = 45
	KeyC          uint16 = 46
	KeyV          uint16 = 47
	KeyB          uint16 = 48
	KeyN          uint16 = 49
	KeyM          uint16 = 50
	KeyComma      uint16 = 51
	KeyDot        uint16 = 52
	KeySlash      uint16 = 53
	KeyRightShift uint16 = 54
	KeyKPAsterisk uint16 = 55
	KeyLeftAlt    uint16 = 56
	KeySpace      uint16 = 57
	KeyCapsLock   uint16 = 58
	KeyF1         uint16 = 59
	KeyF2         uint16 = 60
	KeyF3         uint16 = 61
	KeyF4         uint16 = 62
	KeyF5         uint16 = 63
	KeyF6         uint16 = 64
	KeyF7         uint16 = 65
	KeyF8         uint16 = 66
	KeyF9         uint16 = 67
	KeyF10        uint16 = 68
	KeyF11        uint16 = 87
	KeyF12        uint16 = 88
	KeyRightCtrl  uint16 = 97
	KeyRightAlt   uint16 = 100
	KeyLeftMeta   uint16 = 125 // Super / Windows / Cmd
	KeyRightMeta  uint16 = 126
)

var evdevKeyNames = map[uint16]string{
	KeyEsc:       "ESC",
	Key1:         "1",
	Key2:         "2",
	Key3:         "3",
	Key4:         "4",
	Key5:         "5",
	Key6:         "6",
	Key7:         "7",
	Key8:         "8",
	Key9:         "9",
	Key0:         "0",
	KeyMinus:     "-",
	KeyEqual:     "=",
	KeyBackspace: "BACKSPACE",
	KeyTab:       "TAB",
	KeyQ:         "Q",
	KeyW:         "W",
	KeyE:         "E",
	KeyR:         "R",
	KeyT:         "T",
	KeyY:         "Y",
	KeyU:         "U",
	KeyI:         "I",
	KeyO:         "O",
	KeyP:         "P",
	KeyEnter:     "ENTER",
	KeyA:         "A",
	KeyS:         "S",
	KeyD:         "D",
	KeyF:         "F",
	KeyG:         "G",
	KeyH:         "H",
	KeyJ:         "J",
	KeyK:         "K",
	KeyL:         "L",
	KeyZ:         "Z",
	KeyX:         "X",
	KeyC:         "C",
	KeyV:         "V",
	KeyB:         "B",
	KeyN:         "N",
	KeyM:         "M",
	KeySpace:     "SPACE",
	KeyCapsLock:  "CAPSLOCK",
	KeyF1:        "F1",
	KeyF2:        "F2",
	KeyF3:        "F3",
	KeyF4:        "F4",
	KeyF5:        "F5",
	KeyF6:        "F6",
	KeyF7:        "F7",
	KeyF8:        "F8",
	KeyF9:        "F9",
	KeyF10:       "F10",
	KeyF11:       "F11",
	KeyF12:       "F12",
}

// EvdevCodeToKey translates an evdev keycode into its canonical key name.
func EvdevCodeToKey(code uint16) string {
	if name, ok := evdevKeyNames[code]; ok {
		return name
	}
	return ""
}

// EvdevModifierType represents which modifier a keycode belongs to.
type EvdevModifierType int

const (
	ModifierNone EvdevModifierType = iota
	ModifierCtrl
	ModifierAlt
	ModifierShift
	ModifierMeta // Super / Cmd
)

// ClassifyEvdevModifier checks if a keycode is a modifier key.
func ClassifyEvdevModifier(code uint16) EvdevModifierType {
	switch code {
	case KeyLeftCtrl, KeyRightCtrl:
		return ModifierCtrl
	case KeyLeftAlt, KeyRightAlt:
		return ModifierAlt
	case KeyLeftShift, KeyRightShift:
		return ModifierShift
	case KeyLeftMeta, KeyRightMeta:
		return ModifierMeta
	default:
		return ModifierNone
	}
}
