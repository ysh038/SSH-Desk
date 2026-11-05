// Command tuidesk is the TUI desktop environment (SPEC). Lite build: no
// network code; AI/MCP features are added only with -tags full.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/apps/files"
	"tuidesk/internal/desktop"
	"tuidesk/internal/tui"
	"tuidesk/internal/wm"
)

func main() {
	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tuidesk: cannot initialize the terminal:", err)
		os.Exit(1)
	}
	s.EnableMouse(tcell.MouseDragEvents)
	s.EnablePaste()
	s.HideCursor()
	s.SetTitle("tuidesk")

	w, h := s.Size()
	host, _ := os.Hostname()
	d := desktop.New(w, h, host, time.Now())
	d.SetConsole(os.Getenv("TERM") == "linux")
	d.HomeDir, _ = os.Getwd()
	if d.HomeDir == "" {
		d.HomeDir, _ = os.UserHomeDir()
	}
	d.NewFiles = func(dir string) wm.App { return files.New(d, dir) }
	fw := d.OpenWindow(d.NewFiles(d.HomeDir))
	d.M.ResizeTo(fw, min(90, w-4), min(24, h-4))
	d.Notify("F9 Menu · F1 Help · Ctrl+Q Quit", false)

	tui.NewLoop(s, d).Run()
	s.Fini()
}
