// Command tuidesk is the TUI desktop environment (SPEC). Lite build: no
// network code; AI/MCP features are added only with -tags full.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/desktop"
	"tuidesk/internal/tui"
)

func main() {
	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "터미널을 초기화할 수 없습니다:", err)
		os.Exit(1)
	}
	s.EnableMouse(tcell.MouseDragEvents)
	s.EnablePaste()
	s.HideCursor()
	s.SetTitle("tuidesk")

	w, h := s.Size()
	host, _ := os.Hostname()
	d := desktop.New(w, h, host, time.Now())
	d.OpenWindow(d.NewApp(1))
	d.OpenWindow(d.NewApp(2))
	d.Notify("F9 메뉴 · F1 도움말 · Ctrl+Q 종료", false)

	tui.NewLoop(s, d).Run()
	s.Fini()
}
