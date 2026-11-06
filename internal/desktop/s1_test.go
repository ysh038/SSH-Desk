package desktop

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	editorapp "tuidesk/internal/apps/editor"
	"tuidesk/internal/config"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// fakeRunner stands in for sudo and check commands.
type fakeRunner struct {
	calls  []string
	stdins [][]byte
	// failCheck makes "sh -c <check>" fail with this output.
	failCheck string
	// nopasswd emulates a NOPASSWD sudoers entry: sudo never reads a password.
	nopasswd bool
}

func (r *fakeRunner) run(_ context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	r.stdins = append(r.stdins, append([]byte(nil), stdin...))
	if name == "sudo" && args[0] == "-n" && !r.nopasswd {
		return []byte("sudo: a password is required\n"), &exitErr{}
	}
	sh := -1
	for i := range args {
		if args[i] == "sh" && i+1 < len(args) && args[i+1] == "-c" {
			sh = i
			break
		}
	}
	if name == "sudo" && sh >= 0 && len(args) == sh+6 && strings.Contains(args[sh+2], "tee") {
		// Emulate sudo: it reads the password line only when it needs one,
		// so with NOPASSWD everything on stdin reaches tee.
		body := stdin
		if !r.nopasswd {
			_, body, _ = bytes.Cut(stdin, []byte("\n"))
		}
		path := args[sh+4]
		os.Chmod(path, 0o644)
		err := os.WriteFile(path, body, 0o644)
		os.Chmod(path, 0o444)
		return nil, err
	}
	if r.failCheck != "" {
		return []byte(r.failCheck), &exitErr{}
	}
	return []byte("syntax is ok"), nil
}

type exitErr struct{}

func (*exitErr) Error() string { return "exit status 1" }

func (f *fmHarness) withEditor(r *fakeRunner) {
	f.d.OpenText = func(path string) {
		for _, w := range f.d.M.Windows() {
			if e, ok := w.App.(*editorapp.App); ok {
				if err := e.Open(path); err != nil {
					f.t.Fatal(err)
				}
				f.d.M.Raise(w)
				return
			}
		}
		e := editorapp.New(f.d)
		e.SetRunner(r.run)
		if path == "" {
			e.NewDoc()
		} else if err := e.Open(path); err != nil {
			f.t.Fatal(err)
		}
		w := f.d.OpenWindow(e)
		w.Rect = wm.Rect{X: 1, Y: 2, W: 96, H: 24}
	}
}

func (f *fmHarness) editor() *editorapp.App {
	for _, w := range f.d.M.Windows() {
		if e, ok := w.App.(*editorapp.App); ok {
			return e
		}
	}
	f.t.Fatal("no editor window")
	return nil
}

func (f *fmHarness) screenHas(s string) bool {
	c := f.draw()
	for y := 0; y < c.H; y++ {
		if strings.Contains(c.Row(y), s) {
			return true
		}
	}
	return false
}

// SPEC scenario S1: open nginx.conf from the file manager, insert a server
// snippet, save; the file is not writable so tuidesk offers sudo, asks for
// the password, saves with sudo tee and runs nginx -t.
func TestScenarioS1EditNginxWithSudo(t *testing.T) {
	f := newFM(t)
	conf := "nginx/nginx.conf"
	f.touch(conf, "http {\n    include mime.types;\n\n}\n")
	path := filepath.Join(f.root, conf)
	os.Chmod(path, 0o444) // owned by "root" in real life: not writable
	cfg := config.Default()
	cfg.Validators = []config.Validator{{Pattern: f.root + "/nginx/**", Command: "nginx -t", Sudo: true}}
	cfg.Keyboard = "pc"
	f.d.SetConfig(cfg, "")
	r := &fakeRunner{}
	f.withEditor(r)

	// 1. File manager → nginx/ → double-click nginx.conf.
	_, fm := f.open(filepath.Join(f.root, "nginx"), wm.Rect{X: 1, Y: 2, W: 70, H: 14})
	f.click(fm.Inner().X+4, row(fm, 0))
	f.click(fm.Inner().X+4, row(fm, 0))
	f.settle()
	ed := f.editor()
	d := ed.Doc()
	if d == nil || d.Lang.Key != "nginx" || !d.Sudo {
		t.Fatalf("doc %+v", d)
	}

	// 2. Go to the empty line inside http { } and expand the server snippet.
	f.rune('g', term.ModCtrl)
	f.typeText("3")
	f.key("Enter", 0)
	f.key("Tab", 0) // indent
	f.typeText("server")
	f.key("Tab", 0) // expand
	f.typeText("8080")
	f.key("Tab", 0) // → server_name
	f.typeText("api.example.com")
	f.key("Tab", 0) // → $0 inside location
	f.typeText("proxy_pass http://127.0.0.1:9000;")
	want := "http {\n    include mime.types;\n    server {\n        listen 8080;\n        server_name api.example.com;\n\n        location / {\n            proxy_pass http://127.0.0.1:9000;\n        }\n    }\n}\n"
	if got := d.Ed.Text(); got != want {
		t.Fatalf("buffer:\n%s\nwant:\n%s", got, want)
	}

	// 3. Ctrl+S → permission denied → Save with sudo → password → nginx -t.
	f.rune('s', term.ModCtrl)
	if !f.screenHas("Save it with sudo?") {
		t.Fatal("expected the sudo question")
	}
	f.key("Enter", 0) // "Save with sudo"
	f.settle()        // sudo probe: a password is required
	if !f.screenHas("(sudo)") {
		t.Fatal("expected the password prompt")
	}
	f.typeText("s3cret")
	if f.screenHas("s3cret") {
		t.Fatal("password must not be visible")
	}
	f.key("Enter", 0)
	f.settle()

	data, _ := os.ReadFile(path)
	if string(data) != want {
		t.Fatalf("file:\n%s", data)
	}
	if d.Ed.Modified() {
		t.Fatal("document should be saved")
	}
	if len(r.calls) != 3 || r.calls[0] != "sudo -n -k sh -c :" ||
		!strings.HasPrefix(r.calls[1], "sudo -S -k -p  sh -c") || r.calls[2] != "sudo -S -k -p  sh -c nginx -t" {
		t.Fatalf("calls %q", r.calls)
	}
	if len(r.stdins[0]) != 0 {
		t.Fatal("the sudo probe must not get the password")
	}
	if !bytes.HasPrefix(r.stdins[1], []byte("s3cret\n")) || string(r.stdins[2]) != "s3cret\n" {
		t.Fatal("password must be passed on stdin to sudo")
	}
	if !f.screenHas("Saved nginx.conf with sudo") && !f.screenHas("nginx -t: OK") {
		t.Fatal("expected save/check toasts")
	}
}

// With NOPASSWD sudo, sudo never reads a password from stdin, so a password
// sent there would be written into the file. tuidesk must not ask for one.
func TestEditorSudoSaveNoPasswordNeeded(t *testing.T) {
	f := newFM(t)
	f.touch("etc/한글 hosts", "127.0.0.1 localhost\n")
	path := filepath.Join(f.root, "etc/한글 hosts")
	os.Chmod(path, 0o444)
	cfg := config.Default()
	cfg.Validators = []config.Validator{{Pattern: f.root + "/etc/*", Command: "check {file}", Sudo: true}}
	cfg.Keyboard = "pc"
	f.d.SetConfig(cfg, "")
	r := &fakeRunner{nopasswd: true}
	f.withEditor(r)
	f.d.OpenText(path)
	f.settle()
	d := f.editor().Doc()
	if d == nil || !d.Sudo {
		t.Fatalf("doc %+v", d)
	}

	f.typeText("# 서버\n")
	want := d.Ed.Text()
	f.rune('s', term.ModCtrl)
	if !f.screenHas("Save it with sudo?") {
		t.Fatal("expected the sudo question")
	}
	f.key("Enter", 0) // "Save with sudo"
	f.settle()
	if f.screenHas("(sudo)") {
		t.Fatal("no password prompt expected with NOPASSWD sudo")
	}

	data, _ := os.ReadFile(path)
	if string(data) != want {
		t.Fatalf("file:\n%q\nwant:\n%q", data, want)
	}
	if d.Ed.Modified() {
		t.Fatal("document should be saved")
	}
	if len(r.calls) != 3 || r.calls[0] != "sudo -n -k sh -c :" ||
		!strings.HasPrefix(r.calls[1], "sudo -n -k sh -c") || !strings.HasPrefix(r.calls[2], "sudo -n -k sh -c check ") {
		t.Fatalf("calls %q", r.calls)
	}
	if string(r.stdins[1]) != want || len(r.stdins[2]) != 0 {
		t.Fatalf("stdins %q", r.stdins)
	}
}

func TestEditorCheckFailureShowsOutput(t *testing.T) {
	f := newFM(t)
	f.touch("conf/site.conf", "x\n")
	cfg := config.Default()
	cfg.Validators = []config.Validator{{Pattern: f.root + "/conf/*.conf", Command: "check {file}"}}
	cfg.Keyboard = "pc"
	f.d.SetConfig(cfg, "")
	r := &fakeRunner{failCheck: "line 1: unknown directive \"x\"\nconfiguration test failed"}
	f.withEditor(r)
	f.d.OpenText(filepath.Join(f.root, "conf/site.conf"))
	f.typeText("y")
	f.rune('s', term.ModCtrl)
	f.settle()
	if !f.screenHas("Check failed") || !f.screenHas("unknown directive") {
		t.Fatal("check failure should be shown")
	}
	if !strings.Contains(r.calls[0], "check '"+f.root+"/conf/site.conf'") {
		t.Fatalf("command %q", r.calls[0])
	}
	f.rune('d', 0) // Details
	if !f.screenHas("configuration test failed") {
		t.Fatal("details window should show the full output")
	}
}

func TestEditorEditingFindAndUnsavedClose(t *testing.T) {
	f := newFM(t)
	f.touch("a.txt", "alpha\nbeta\ngamma\n")
	f.d.SetConfig(pcConfig(), "")
	f.withEditor(&fakeRunner{})
	f.d.OpenText(filepath.Join(f.root, "a.txt"))
	ed := f.editor()
	d := ed.Doc()

	// Find "gam" and replace the selection by typing.
	f.rune('f', term.ModCtrl)
	f.typeText("gam")
	f.key("Esc", 0)
	f.typeText("GAM")
	if d.Ed.Text() != "alpha\nbeta\nGAMma\n" {
		t.Fatalf("text %q", d.Ed.Text())
	}
	// Option+Up / Option+Down = document start / end; Shift+End selects.
	f.key("Down", term.ModAlt)
	if d.Ed.Cur.Line != 2 {
		t.Fatalf("Alt+Down → %+v", d.Ed.Cur)
	}
	f.key("Up", term.ModAlt)
	f.key("End", term.ModShift)
	f.rune('c', term.ModCtrl)
	if f.d.ClipboardText() != "alpha" {
		t.Fatalf("clipboard %q", f.d.ClipboardText())
	}
	// Cmd+V (paste event) inserts text.
	f.key("End", 0)
	f.d.Handle(term.PasteEvent{Text: "!"})
	if !strings.HasPrefix(d.Ed.Text(), "alpha!") {
		t.Fatalf("paste %q", d.Ed.Text())
	}
	// Ctrl+W with unsaved changes asks; Cancel keeps the window.
	f.rune('w', term.ModCtrl)
	if !f.screenHas("unsaved changes") {
		t.Fatal("close should ask")
	}
	f.key("Esc", 0)
	if len(f.d.M.Windows()) != 1 {
		t.Fatal("cancel must keep the editor")
	}
	// Quit warns about unsaved files.
	f.rune('q', term.ModCtrl) // in the editor Ctrl+Q closes the tab → asks
	if !f.screenHas("has unsaved changes") {
		t.Fatal("closing the tab should ask")
	}
	f.rune('d', 0) // Discard
	if len(f.d.M.Windows()) != 0 {
		t.Fatal("last tab closed should close the window")
	}
}

func TestEditorMouseSelection(t *testing.T) {
	f := newFM(t)
	f.touch("m.txt", "hello world\nsecond line\n")
	f.d.SetConfig(pcConfig(), "")
	f.withEditor(&fakeRunner{})
	f.d.OpenText(filepath.Join(f.root, "m.txt"))
	ed := f.editor()
	w := f.d.M.Focused()
	f.draw()
	in := w.Inner()
	g := 3 // gutter: 1 digit + 2
	textY := in.Y + 2
	// Double-click "world".
	f.click(in.X+g+7, textY)
	f.click(in.X+g+7, textY)
	if s := ed.Doc().Ed.SelectedText(); s != "world" {
		t.Fatalf("double click → %q", s)
	}
	f.wait()
	// Drag from (0,0) to (0,1) col 6.
	f.drag(in.X+g, textY, in.X+g+6, textY+1)
	if s := ed.Doc().Ed.SelectedText(); s != "hello world\nsecond" {
		t.Fatalf("drag → %q", s)
	}
}

func pcConfig() config.Config {
	c := config.Default()
	c.Keyboard = "pc"
	return c
}
