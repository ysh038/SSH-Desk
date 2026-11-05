package editor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"tuidesk/internal/config"
	core "tuidesk/internal/editor"
	"tuidesk/internal/fsops"
	"tuidesk/internal/syntax"
	"tuidesk/internal/ui"
)

// saveRunner runs a program with stdin and returns its combined output.
// Tests replace it; production uses execRunner.
type saveRunner func(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)

func execRunner(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	return cmd.CombinedOutput()
}

// save writes the active document. saveAs asks for a path. after runs once
// the document is saved.
func (a *App) save(saveAs bool, after func()) {
	d := a.Doc()
	if d == nil || d.ReadOnly {
		return
	}
	if saveAs || d.Path == "" {
		dir, _ := os.Getwd()
		if d.Path != "" {
			dir = filepath.Dir(d.Path)
		}
		p := ui.NewPrompt("Save as", "File path:", filepath.Join(dir, d.Name()), func(v string) {
			abs, _ := filepath.Abs(expandHome(v))
			d.Path, d.ModTime, d.Sudo = abs, d.ModTime.AddDate(-100, 0, 0), false
			d.Lang = syntax.Detect(abs, "")
			if _, err := os.Stat(abs); err == nil {
				d.ModTime = statTime(abs)
			}
			a.save(false, after)
		})
		p.Validate = func(v string) error {
			if strings.TrimSpace(v) == "" || strings.HasSuffix(v, "/") {
				return errors.New("enter a file path")
			}
			return nil
		}
		a.h.Prompt(p)
		return
	}
	if info, err := os.Stat(d.Path); err == nil && !d.ModTime.IsZero() && !info.ModTime().Equal(d.ModTime) {
		a.h.Confirm("File changed on disk", a.display(d.Name())+" was changed by another program after you opened it.\nOverwrite it with your version?",
			[]string{"Overwrite", "Cancel"}, func(c int) {
				if c == 0 {
					a.write(d, after)
				}
			})
		return
	}
	a.write(d, after)
}

func statTime(p string) (t timeT) {
	if info, err := os.Stat(p); err == nil {
		return info.ModTime()
	}
	return t
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return h + p[1:]
		}
	}
	return p
}

func (a *App) write(d *Doc, after func()) {
	data := []byte(d.Ed.Text())
	if !d.Sudo {
		err := writeFile(d.Path, data, a.h.Config().Backup)
		if err == nil {
			a.saved(d, false)
			a.validate(d, nil, after)
			return
		}
		if !errors.Is(err, fs.ErrPermission) {
			a.h.ShowError("Save failed", a.display(d.Name())+": "+errText(err), err.Error())
			return
		}
		d.Sudo = true
	}
	a.askSudo(d, data, after)
}

// writeFile overwrites path in place (keeping owner, mode and links),
// after copying the old content to path.bak when backup is on. The file is
// only truncated once it is known to be writable.
func writeFile(path string, data []byte, backup bool) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, mode)
	if err != nil {
		return err
	}
	if backup {
		if old, err := os.ReadFile(path); err == nil && len(old) > 0 {
			_ = os.WriteFile(path+".bak", old, mode) // best effort: the folder may not be writable
		}
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func errText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

func (a *App) saved(d *Doc, viaSudo bool) {
	d.Ed.MarkSaved()
	if info, err := os.Stat(d.Path); err == nil {
		d.ModTime, d.Size = info.ModTime(), info.Size()
	}
	how := ""
	if viaSudo {
		how = " with sudo"
	}
	a.h.Notify("Saved "+a.display(d.Name())+how, false)
	a.h.FilesChanged()
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "you"
}

// askSudo confirms, asks for the password and saves through `sudo tee`.
func (a *App) askSudo(d *Doc, data []byte, after func()) {
	a.h.Confirm("Permission denied", fmt.Sprintf("%s cannot write %s.\nSave it with sudo?", currentUser(), a.display(d.Path)),
		[]string{"Save with sudo", "Cancel"}, func(c int) {
			if c != 0 {
				return
			}
			p := ui.NewPrompt("sudo", "Password for "+currentUser()+" (sudo):", "", nil)
			p.Input.Password = true
			p.OnSecret = func(pw []rune) { a.sudoWrite(d, data, pw, after) }
			a.h.Prompt(p)
		})
}

// sudoScript keeps a .bak copy (when $2 is 1) and writes stdin to $1. tee
// writes in place, so owner and mode of the existing file are kept.
const sudoScript = `if [ "$2" = 1 ] && [ -f "$1" ]; then cp -p -- "$1" "$1.bak" 2>/dev/null; fi; tee -- "$1" >/dev/null`

// secretInput builds password + "\n" + rest without creating strings.
func secretInput(pw []rune, rest []byte) []byte {
	buf := make([]byte, 0, len(pw)*3+1+len(rest))
	for _, r := range pw {
		buf = utf8.AppendRune(buf, r)
	}
	buf = append(buf, '\n')
	return append(buf, rest...)
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func wipeRunes(r []rune) {
	for i := range r {
		r[i] = 0
	}
}

type checkResult struct {
	cmd string
	out []byte
	err error
}

func (a *App) sudoWrite(d *Doc, data []byte, pw []rune, after func()) {
	backup := "0"
	if a.h.Config().Backup {
		backup = "1"
	}
	path := d.Path
	checks := a.validators(path)
	var saveOut []byte
	var results []checkResult
	run := a.runSave
	a.h.Go("Saving with sudo", func(ctx context.Context, _ func(fsops.Progress)) error {
		defer wipeRunes(pw)
		in := secretInput(pw, data)
		defer wipe(in)
		var err error
		saveOut, err = run(ctx, "sudo", []string{"-S", "-k", "-p", "", "sh", "-c", sudoScript, "sh", path, backup}, in)
		if err != nil {
			return err
		}
		for _, v := range checks {
			cmd := expandCmd(v.Command, path)
			var out []byte
			var cerr error
			if v.Sudo {
				pin := secretInput(pw, nil)
				out, cerr = run(ctx, "sudo", []string{"-S", "-k", "-p", "", "sh", "-c", cmd}, pin)
				wipe(pin)
			} else {
				out, cerr = run(ctx, "sh", []string{"-c", cmd}, nil)
			}
			results = append(results, checkResult{cmd: cmd, out: out, err: cerr})
		}
		return nil
	}, func(err error) {
		if err != nil {
			msg := "sudo could not save the file."
			if bytes.Contains(saveOut, []byte("incorrect password")) || bytes.Contains(saveOut, []byte("Sorry, try again")) {
				msg = "The sudo password was not accepted."
			} else if bytes.Contains(saveOut, []byte("not in the sudoers")) || bytes.Contains(saveOut, []byte("may not run sudo")) {
				msg = currentUser() + " is not allowed to use sudo on this server."
			}
			a.h.ShowError("Save with sudo failed", msg, strings.TrimSpace(string(saveOut))+"\n"+err.Error())
			return
		}
		a.saved(d, true)
		a.report(results)
		if after != nil {
			after()
		}
	})
}

// ---- on-save validators (SPEC 7.3) ----

func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case c == '*' && i+1 < len(pattern) && pattern[i+1] == '*':
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func (a *App) validators(path string) []config.Validator {
	var out []config.Validator
	for _, v := range a.h.Config().Validators {
		if v.Pattern != "" && v.Command != "" && globRegexp(v.Pattern).MatchString(path) {
			out = append(out, v)
		}
	}
	return out
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func expandCmd(cmd, path string) string { return strings.ReplaceAll(cmd, "{file}", shellQuote(path)) }

// validate runs checks for a file saved without sudo (they run as the user).
func (a *App) validate(d *Doc, _ []rune, after func()) {
	checks := a.validators(d.Path)
	if len(checks) == 0 {
		if after != nil {
			after()
		}
		return
	}
	path := d.Path
	run := a.runSave
	var results []checkResult
	a.h.Go("Checking "+a.display(d.Name()), func(ctx context.Context, _ func(fsops.Progress)) error {
		for _, v := range checks {
			cmd := expandCmd(v.Command, path)
			out, err := run(ctx, "sh", []string{"-c", cmd}, nil)
			results = append(results, checkResult{cmd: cmd, out: out, err: err})
		}
		return nil
	}, func(error) {
		a.report(results)
		if after != nil {
			after()
		}
	})
}

func (a *App) report(results []checkResult) {
	for _, r := range results {
		if r.err == nil {
			a.h.Notify(r.cmd+": OK", false)
			continue
		}
		first := strings.TrimSpace(string(r.out))
		if i := strings.IndexByte(first, '\n'); i > 0 {
			first = first[:i]
		}
		a.h.Notify(r.cmd+": failed", true)
		a.h.ShowError("Check failed: "+r.cmd, first, string(r.out))
	}
}

// ---- open / reload / indentation ----

func (a *App) promptOpen() {
	dir, _ := os.Getwd()
	if d := a.Doc(); d != nil && d.Path != "" {
		dir = filepath.Dir(d.Path) + "/"
	}
	p := ui.NewPrompt("Open file", "Path:", dir, func(v string) {
		if err := a.Open(expandHome(v)); err != nil {
			a.h.ShowError("Cannot open file", errText(err), "")
		}
	})
	p.Validate = func(v string) error {
		info, err := os.Stat(expandHome(v))
		if err != nil {
			return errors.New(errText(err))
		}
		if info.IsDir() {
			return errors.New("that is a folder")
		}
		if !fsops.IsText(expandHome(v)) {
			return errors.New("not a text file")
		}
		return nil
	}
	a.h.Prompt(p)
}

func (a *App) reload() {
	d := a.Doc()
	if d == nil || d.Path == "" {
		return
	}
	do := func() {
		data, err := os.ReadFile(d.Path)
		if err != nil {
			a.h.ShowError("Reload failed", errText(err), "")
			return
		}
		cur := d.Ed.Cur
		ts, et := d.Ed.TabSize, d.Ed.ExpandTabs
		d.Ed = core.New(string(data))
		d.Ed.TabSize, d.Ed.ExpandTabs, d.Ed.ColonIndent = ts, et, d.Lang.ColonIndent
		d.Ed.SetCursor(cur, false)
		d.ModTime = statTime(d.Path)
	}
	if d.Ed.Modified() {
		a.h.Confirm("Reload", "Discard your changes and reload "+a.display(d.Name())+" from disk?", []string{"Reload", "Cancel"}, func(c int) {
			if c == 0 {
				do()
			}
		})
		return
	}
	do()
}

func (a *App) promptIndent() {
	d := a.Doc()
	if d == nil {
		return
	}
	cur := strconv.Itoa(d.Ed.TabSize)
	if !d.Ed.ExpandTabs {
		cur = "t" + cur
	}
	p := ui.NewPrompt("Indentation", "Spaces per level (e.g. 4), or t4 for tab characters:", cur, func(v string) {
		v = strings.TrimSpace(v)
		d.Ed.ExpandTabs = !strings.HasPrefix(v, "t")
		n, _ := strconv.Atoi(strings.TrimPrefix(v, "t"))
		d.Ed.TabSize = n
	})
	p.Validate = func(v string) error {
		n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(v), "t"))
		if err != nil || n < 1 || n > 16 {
			return errors.New("enter 1-16, optionally prefixed with t")
		}
		return nil
	}
	a.h.Prompt(p)
}

// applyEditorConfig reads .editorconfig files from the file's folder up to
// the root (or root = true) for indent_style and indent_size / tab_width.
func applyEditorConfig(d *Doc) {
	var files []string
	for dir := filepath.Dir(d.Path); ; dir = filepath.Dir(dir) {
		p := filepath.Join(dir, ".editorconfig")
		if _, err := os.Stat(p); err == nil {
			files = append([]string{p}, files...) // nearer files override
			if isRoot(p) {
				break
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	base := filepath.Base(d.Path)
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		applyEditorConfigFile(d, fh, base)
		fh.Close()
	}
}

func isRoot(p string) bool {
	data, err := os.ReadFile(p)
	return err == nil && regexp.MustCompile(`(?mi)^\s*root\s*=\s*true\s*$`).Match(data)
}

func applyEditorConfigFile(d *Doc, r io.Reader, base string) {
	sc := bufio.NewScanner(r)
	match := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
		case strings.HasPrefix(line, "["):
			match = sectionMatches(strings.Trim(line, "[]"), base)
		case match:
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k, v = strings.ToLower(strings.TrimSpace(k)), strings.ToLower(strings.TrimSpace(v))
			switch k {
			case "indent_style":
				d.Ed.ExpandTabs = v == "space"
			case "indent_size", "tab_width":
				if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 16 {
					d.Ed.TabSize = n
				}
			}
		}
	}
}

// sectionMatches handles *, *.ext and *.{a,b}.
func sectionMatches(pat, base string) bool {
	if i := strings.Index(pat, "{"); i >= 0 {
		if j := strings.Index(pat[i:], "}"); j > 0 {
			for _, alt := range strings.Split(pat[i+1:i+j], ",") {
				if sectionMatches(pat[:i]+alt+pat[i+j+1:], base) {
					return true
				}
			}
			return false
		}
	}
	ok, _ := filepath.Match(strings.TrimPrefix(pat, "**/"), base)
	return ok
}

type timeT = time.Time
