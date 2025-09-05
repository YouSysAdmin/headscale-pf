package term_color

import (
	"os"

	"github.com/jagottsicher/termcolor"
)

// CheckTerminalColorSupport checking terminal color support
func CheckTerminalColorSupport() bool {
	var termColorSupport bool
	switch l := termcolor.SupportLevel(os.Stderr); l {
	case termcolor.Level16M:
		termColorSupport = true
	case termcolor.Level256:
		termColorSupport = true
	case termcolor.LevelBasic:
		termColorSupport = true
	case termcolor.LevelNone:
		termColorSupport = false
	default:
		termColorSupport = false
	}

	return termColorSupport
}
