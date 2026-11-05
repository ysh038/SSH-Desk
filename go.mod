module tuidesk

go 1.24.2

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/gdamore/tcell/v2 v2.13.10
	github.com/mattn/go-runewidth v0.0.31
)

require (
	github.com/charmbracelet/colorprofile v0.4.2 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260303162955-0b88c25f3fff // indirect
	github.com/charmbracelet/x/ansi v0.11.7 // indirect
	github.com/charmbracelet/x/exp/ordered v0.1.0 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/vt v0.1.1-0.20261101151554-27ecf3e4d454 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/term v0.37.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)

// Patched copy: wide characters at the right margin (third_party/vt/TUIDESK_PATCH.md).
replace github.com/charmbracelet/x/vt => ./third_party/vt
