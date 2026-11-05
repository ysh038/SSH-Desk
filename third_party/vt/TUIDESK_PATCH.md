# tuidesk patch of github.com/charmbracelet/x/vt

Upstream: github.com/charmbracelet/x/vt v0.1.1-0.20261101151554-27ecf3e4d454 (MIT, see LICENSE).
Used through a `replace` directive in tuidesk's go.mod. Test files are removed.

Changes (utf8.go, handleGrapheme):
- A wide character (e.g. Hangul) printed when fewer columns than its width are
  left on the line wraps to the next line, as xterm does. Upstream drops it.
- The pending-wrap (phantom) state is entered when a wide character ends at the
  last column, not only when the cursor is on it.

Change (emulator.go): the `closed` flag is an `atomic.Bool`, because Read
is called on a reader goroutine while Close runs on the UI goroutine (found
with `go test -race`).

Reason: docs/adr/0004-terminal-emulator.md. Drop this copy once upstream has the fix.
