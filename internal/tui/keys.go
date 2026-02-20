package tui

import tea "github.com/charmbracelet/bubbletea"

type keyMap struct {
	Quit        tea.KeyType
	ToggleFocus tea.KeyType
	Up          tea.KeyType
	Down        tea.KeyType
	Select      tea.KeyType
}

var keys = keyMap{
	Quit:        tea.KeyCtrlC,
	ToggleFocus: tea.KeyEscape,
	Up:          tea.KeyUp,
	Down:        tea.KeyDown,
	Select:      tea.KeyEnter,
}

func isKey(msg tea.KeyMsg, k tea.KeyType) bool {
	return msg.Type == k
}

func isRuneKey(msg tea.KeyMsg, r rune) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}

// isAltRune returns true if the key is Alt+<r> (Cmd+<r> on macOS terminals).
func isAltRune(msg tea.KeyMsg, r rune) bool {
	return msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}
