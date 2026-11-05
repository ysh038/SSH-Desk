# Third-Party Notices

tuidesk includes the following third-party Go modules. Full license texts are
distributed with each module's source (see `go mod vendor` output in source
tarballs).

| Module | Version | License | Why |
|---|---|---|---|
| github.com/gdamore/tcell/v2 | v2.13.10 | Apache-2.0 | Terminal I/O: mouse (SGR 1006, drag 1002), bracketed paste, OSC52, wide characters |
| github.com/gdamore/encoding | v1.0.1 | Apache-2.0 | tcell dependency (character set encodings) |
| github.com/mattn/go-runewidth | v0.0.31 | MIT | Display width of Hangul / East Asian characters |
| github.com/rivo/uniseg | v0.4.7 | MIT | tcell/runewidth dependency (grapheme clusters, widths) |
| github.com/clipperhouse/uax29/v2 | v2.7.0 | MIT | runewidth / x/vt dependency (grapheme segmentation) |
| github.com/lucasb-eyer/go-colorful | v1.4.0 | MIT | tcell / x/vt dependency (color conversion) |
| golang.org/x/sys | v0.41.0 | BSD-3-Clause | tcell dependency (terminal syscalls); terminal window foreground process (TIOCGPGRP) |
| golang.org/x/term | v0.37.0 | BSD-3-Clause | tcell dependency (raw mode) |
| golang.org/x/text | v0.31.0 | BSD-3-Clause | tcell dependency (encodings) |
| github.com/BurntSushi/toml | v1.6.0 | MIT | Reading/writing config.toml and snippet files (SPEC 7.4, 7.7) |
| github.com/charmbracelet/x/vt | v0.1.1-0.20261101151554-27ecf3e4d454 (patched copy in `third_party/vt`) | MIT | Terminal window VT emulator (ADR 0004): xterm sequences, alternate screen, scrollback, wide characters |
| github.com/charmbracelet/ultraviolet | v0.0.0-20260303162955-0b88c25f3fff | MIT | x/vt dependency (cell buffer) |
| github.com/charmbracelet/x/ansi | v0.11.7 | MIT | x/vt dependency (escape sequence parser) |
| github.com/charmbracelet/colorprofile | v0.4.2 | MIT | x/vt dependency |
| github.com/charmbracelet/x/term, x/termios, x/windows, x/exp/ordered | v0.2.2, v0.1.1, v0.2.2, v0.1.0 | MIT | x/vt dependencies |
| github.com/clipperhouse/displaywidth | v0.11.0 | MIT | x/vt dependency (display width) |
| github.com/xo/terminfo | v0.0.0-20220910002029-abceb7e1c41e | MIT | x/vt dependency |
| github.com/muesli/cancelreader | v0.2.2 | MIT | x/vt dependency |
| golang.org/x/sync | v0.19.0 | BSD-3-Clause | x/vt dependency |
| github.com/creack/pty | v1.1.24 | MIT | Pseudo-terminals for the terminal window (pure Go, no cgo) |

`third_party/vt` is a modified copy of github.com/charmbracelet/x/vt; the
changes are listed in `third_party/vt/TUIDESK_PATCH.md` and its MIT license
is kept in `third_party/vt/LICENSE`.
