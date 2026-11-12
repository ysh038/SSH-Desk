// Package sudo builds and runs commands through sudo without ever running
// the app itself as root (CLAUDE.md rule 7). The password is passed once on
// stdin and wiped afterwards (SPEC 7.2.1); nothing here logs it.
package sudo

import (
	"bytes"
	"context"
	"os/exec"
	"unicode/utf8"
)

// Runner runs a program with stdin and returns its combined output. Tests
// replace it; production uses Exec.
type Runner func(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)

// Exec is the real Runner.
func Exec(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	return cmd.CombinedOutput()
}

// Args builds sudo's arguments. -k ignores cached credentials, so a probe
// and the real command see the same policy. With a password, sudo reads it
// as the first line of stdin (-S, no prompt text); without one, -n makes
// sudo fail instead of asking.
func Args(withPassword bool, cmd ...string) []string {
	args := []string{"-n", "-k"}
	if withPassword {
		args = []string{"-S", "-k", "-p", ""}
	}
	return append(args, cmd...)
}

// ProbeArgs is a command that does nothing: it succeeds when sudo needs no
// password (NOPASSWD). Callers must probe first, because a sudo that does
// not read a password would pass the password line on to the command.
func ProbeArgs() []string { return Args(false, "sh", "-c", ":") }

// Input is the stdin for a sudo command: the password line first when
// there is a password (pw != nil), then rest. It never builds a string.
func Input(pw []rune, rest []byte) []byte {
	if pw == nil {
		return append([]byte(nil), rest...)
	}
	buf := make([]byte, 0, len(pw)*utf8.UTFMax+1+len(rest))
	for _, r := range pw {
		buf = utf8.AppendRune(buf, r)
	}
	buf = append(buf, '\n')
	return append(buf, rest...)
}

// Wipe zeroes b.
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// WipeRunes zeroes r.
func WipeRunes(r []rune) {
	for i := range r {
		r[i] = 0
	}
}

// Explain turns sudo's own error output into a sentence for the user, or ""
// when the failure is not about sudo itself.
func Explain(out []byte, user string) string {
	switch {
	case bytes.Contains(out, []byte("incorrect password")), bytes.Contains(out, []byte("Sorry, try again")):
		return "The sudo password was not accepted."
	case bytes.Contains(out, []byte("not in the sudoers")), bytes.Contains(out, []byte("may not run sudo")):
		return user + " is not allowed to use sudo on this server."
	}
	return ""
}
