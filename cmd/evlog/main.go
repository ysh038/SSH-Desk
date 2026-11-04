// Command evlog is the Phase 0 spike: it shows every key, mouse, paste and
// focus event the terminal delivers, plus the derived gestures (click,
// double click, drag), so terminal compatibility can be checked over ssh.
// Everything shown on screen is also appended to a log file.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/term"
)

const maxLines = 2000

type mouseMode int

const (
	modeOff mouseMode = iota
	modeButton
	modeDrag
	modeMotion
)

var modeNames = [...]string{"off", "button(1000)", "drag(1002)", "motion(1003)"}

func parseMode(s string) (mouseMode, error) {
	switch s {
	case "off":
		return modeOff, nil
	case "button":
		return modeButton, nil
	case "drag":
		return modeDrag, nil
	case "motion":
		return modeMotion, nil
	}
	return 0, fmt.Errorf("unknown mouse mode %q (off|button|drag|motion)", s)
}

type app struct {
	s     tcell.Screen
	det   *term.Detector
	logf  *os.File
	label string
	mode  mouseMode

	lines         []string
	collapseLast  bool // last screen line is a collapsible drag-motion line
	collapseCount int

	mx, my      int
	mouseStatus string
	lastGesture string

	pasting    bool
	pasteBuf   strings.Builder
	pasteStart time.Time
	lastPaste  string

	quitArmed time.Time
	quit      bool
}

func main() {
	modeFlag := flag.String("mouse", "drag", "mouse tracking mode: off|button|drag|motion")
	labelFlag := flag.String("label", "", "terminal name used in the log file name (e.g. warp, iterm2, ghostty, terminal-app)")
	logFlag := flag.String("log", "", "log file path (default: evlog-<label>-<time>.log in the current directory)")
	dblFlag := flag.Duration("dblclick", term.DefaultMultiClickInterval, "double click interval")
	flag.Parse()

	mode, err := parseMode(*modeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	label := *labelFlag
	if label == "" {
		label = guessLabel()
	}
	logPath := *logFlag
	if logPath == "" {
		logPath = fmt.Sprintf("evlog-%s-%s.log", sanitize(label), time.Now().Format("20060102-150405"))
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "로그 파일을 열 수 없습니다:", err)
		os.Exit(1)
	}
	defer logf.Close()

	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "터미널을 초기화할 수 없습니다:", err)
		os.Exit(1)
	}

	a := &app{s: s, det: term.NewDetector(), logf: logf, label: label, mx: -1, my: -1}
	a.det.MultiClickInterval = *dblFlag
	s.EnablePaste()
	s.EnableFocus()
	a.setMode(mode, false)
	s.SetTitle("tuidesk evlog")

	a.writeHeader()
	a.draw()

	// Event driven: redraw only after an event arrives (CLAUDE.md rule 9).
	for !a.quit {
		ev := s.PollEvent()
		if ev == nil {
			break
		}
		a.handle(ev)
		if !a.quit {
			a.draw()
		}
	}
	s.Fini()
	fmt.Printf("로그 저장됨: %s\n", logPath)
}

func guessLabel() string {
	for _, k := range []string{"LC_TERMINAL", "TERM_PROGRAM"} {
		if v := os.Getenv(k); v != "" {
			return strings.ToLower(v)
		}
	}
	return "unknown"
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitize(s string) string { return unsafeChars.ReplaceAllString(s, "_") }

func (a *app) writeHeader() {
	w, h := a.s.Size()
	host, _ := os.Hostname()
	_, ssh := os.LookupEnv("SSH_CONNECTION")
	fmt.Fprintf(a.logf, "# tuidesk evlog %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(a.logf, "# label=%s host=%s os=%s/%s ssh=%v\n", a.label, host, runtime.GOOS, runtime.GOARCH, ssh)
	for _, k := range []string{"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "LC_TERMINAL", "LC_TERMINAL_VERSION", "LANG", "LC_ALL", "LC_CTYPE", "RUNEWIDTH_EASTASIAN", "TMUX", "STY"} {
		fmt.Fprintf(a.logf, "# %s=%q\n", k, os.Getenv(k))
	}
	fmt.Fprintf(a.logf, "# colors=%d size=%dx%d mouse=%s dblclick=%s ambiguousWide=%v\n",
		a.s.Colors(), w, h, modeNames[a.mode], a.det.MultiClickInterval, term.EastAsianAmbiguousWide())
}

func (a *app) setMode(m mouseMode, announce bool) {
	a.mode = m
	switch m {
	case modeOff:
		a.s.DisableMouse()
	case modeButton:
		a.s.EnableMouse(tcell.MouseButtonEvents)
	case modeDrag:
		a.s.EnableMouse(tcell.MouseDragEvents)
	case modeMotion:
		a.s.EnableMouse(tcell.MouseMotionEvents)
	}
	if announce {
		a.logLine("MODE   마우스 모드 → %s", modeNames[m])
	}
}

// logLine appends a line to the screen buffer and the log file.
func (a *app) logLine(format string, args ...any) {
	a.appendLine(fmt.Sprintf(format, args...), false)
}

func (a *app) appendLine(msg string, collapsible bool) {
	line := time.Now().Format("15:04:05.000") + " " + msg
	fmt.Fprintln(a.logf, line)
	if collapsible && a.collapseLast && len(a.lines) > 0 {
		a.collapseCount++
		a.lines[len(a.lines)-1] = fmt.Sprintf("%s  (연속 %d회)", line, a.collapseCount)
		return
	}
	a.collapseLast = collapsible
	a.collapseCount = 1
	a.lines = append(a.lines, line)
	if len(a.lines) > maxLines {
		a.lines = a.lines[len(a.lines)-maxLines:]
	}
}

func (a *app) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		w, h := ev.Size()
		a.logLine("RESIZE %dx%d", w, h)
		a.s.Sync()
	case *tcell.EventPaste:
		a.handlePaste(ev)
	case *tcell.EventKey:
		if a.pasting {
			a.collectPaste(ev)
			return
		}
		a.handleKey(ev)
	case *tcell.EventMouse:
		a.handleMouse(ev)
	case *tcell.EventFocus:
		if ev.Focused {
			a.logLine("FOCUS  창 포커스 얻음")
		} else {
			a.logLine("FOCUS  창 포커스 잃음")
		}
	case *tcell.EventClipboard:
		a.logLine("CLIP   OSC52 읽기 응답: %q", preview(string(ev.Data()), 60))
	}
}

func (a *app) handlePaste(ev *tcell.EventPaste) {
	if ev.Start() {
		a.pasting = true
		a.pasteBuf.Reset()
		a.pasteStart = time.Now()
		a.logLine("PASTE  ▶ bracketed paste 시작")
		return
	}
	a.pasting = false
	text := a.pasteBuf.String()
	lines := strings.Count(text, "\n") + 1
	if text == "" {
		lines = 0
	}
	first, _, _ := strings.Cut(text, "\n")
	a.lastPaste = fmt.Sprintf("%d자 %d줄", utf8.RuneCountInString(text), lines)
	a.logLine("PASTE  ■ 끝: %d자(rune) %d바이트 %d줄, 첫 줄=%q", utf8.RuneCountInString(text), len(text), lines, preview(first, 40))
	fmt.Fprintf(a.logf, "----- paste begin -----\n%s\n----- paste end -----\n", text)
}

func (a *app) collectPaste(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyRune:
		a.pasteBuf.WriteRune(ev.Rune())
	case tcell.KeyEnter, tcell.KeyLF:
		a.pasteBuf.WriteByte('\n')
	case tcell.KeyTab:
		a.pasteBuf.WriteByte('\t')
	default:
		fmt.Fprintf(&a.pasteBuf, "<%s>", ev.Name())
	}
}

func (a *app) handleKey(ev *tcell.EventKey) {
	r := ev.Rune()
	runeInfo := "-"
	if ev.Key() == tcell.KeyRune {
		runeInfo = fmt.Sprintf("%q U+%04X", r, r)
	}
	a.logLine("KEY    %s key=%-4d rune=%s mod=%s", term.PadRight(ev.Name(), 14), ev.Key(), runeInfo, mods(ev.Modifiers()))

	switch ev.Key() {
	case tcell.KeyF10:
		a.quit = true
	case tcell.KeyCtrlC:
		if time.Since(a.quitArmed) < 1500*time.Millisecond {
			a.quit = true
			return
		}
		a.quitArmed = time.Now()
		a.logLine("INFO   한 번 더 Ctrl+C를 누르면 종료합니다")
	case tcell.KeyF2:
		payload := "tuidesk OSC52 테스트 " + time.Now().Format("15:04:05")
		a.s.SetClipboard([]byte(payload))
		a.logLine("OSC52  클립보드로 전송: %q → 맥에서 아무 곳에 Cmd+V로 확인", payload)
	case tcell.KeyF3:
		a.lines = nil
		a.collapseLast = false
	case tcell.KeyF4:
		a.setMode((a.mode+1)%4, true)
	case tcell.KeyF5:
		a.s.GetClipboard()
		a.logLine("OSC52  클립보드 읽기 요청 보냄 (응답이 없으면 미지원/차단)")
	}
}

func (a *app) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	btns := ev.Buttons()
	in := term.MouseInput{X: x, Y: y, Mod: toMod(ev.Modifiers()), Time: ev.When()}
	switch {
	case btns&tcell.WheelUp != 0:
		in.Wheel = term.WheelUp
	case btns&tcell.WheelDown != 0:
		in.Wheel = term.WheelDown
	case btns&tcell.WheelLeft != 0:
		in.Wheel = term.WheelLeft
	case btns&tcell.WheelRight != 0:
		in.Wheel = term.WheelRight
	case btns&tcell.Button1 != 0:
		in.Button = term.ButtonLeft
	case btns&tcell.Button2 != 0:
		in.Button = term.ButtonRight
	case btns&tcell.Button3 != 0:
		in.Button = term.ButtonMiddle
	}

	a.mx, a.my = x, y
	a.mouseStatus = fmt.Sprintf("(%d,%d) btn=%s mod=%s", x, y, buttonNames(btns), in.Mod)

	gestures := a.det.Feed(in)

	// Pure pointer motion (1003) only updates the status line.
	onlyMove := len(gestures) > 0
	for _, g := range gestures {
		if g.Kind != term.GestureMove {
			onlyMove = false
		}
	}
	if onlyMove || (len(gestures) == 0 && in.Button == term.ButtonNone) {
		return
	}

	dragMotion := len(gestures) > 0 && gestures[0].Kind == term.GestureDrag && len(gestures) == 1
	raw := fmt.Sprintf("MOUSE  (%3d,%3d) btn=%-12s mod=%s", x, y, buttonNames(btns), in.Mod)
	if dragMotion {
		a.appendLine(raw+"  [드래그 중]", true)
	} else if len(gestures) > 0 || in.Button != term.ButtonNone {
		a.logLine("%s", raw)
	}
	for _, g := range gestures {
		switch g.Kind {
		case term.GestureMove, term.GestureDrag:
			continue
		case term.GestureClick, term.GestureDragStart, term.GestureDragEnd, term.GestureWheel:
			a.lastGesture = g.String()
			a.logLine("  ⇒    %s", g)
		}
	}
}

func toMod(m tcell.ModMask) term.Mod {
	var out term.Mod
	if m&tcell.ModShift != 0 {
		out |= term.ModShift
	}
	if m&tcell.ModCtrl != 0 {
		out |= term.ModCtrl
	}
	if m&tcell.ModAlt != 0 {
		out |= term.ModAlt
	}
	if m&tcell.ModMeta != 0 {
		out |= term.ModMeta
	}
	return out
}

func mods(m tcell.ModMask) string { return toMod(m).String() }

func buttonNames(b tcell.ButtonMask) string {
	if b == tcell.ButtonNone {
		return "none"
	}
	var parts []string
	for _, p := range []struct {
		m    tcell.ButtonMask
		name string
	}{
		{tcell.Button1, "Left"}, {tcell.Button2, "Right"}, {tcell.Button3, "Middle"},
		{tcell.Button4, "B4"}, {tcell.Button5, "B5"},
		{tcell.WheelUp, "WheelUp"}, {tcell.WheelDown, "WheelDown"},
		{tcell.WheelLeft, "WheelLeft"}, {tcell.WheelRight, "WheelRight"},
	} {
		if b&p.m != 0 {
			parts = append(parts, p.name)
		}
	}
	return strings.Join(parts, "+")
}

func preview(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// ---- drawing ----

var (
	stHeader = tcell.StyleDefault.Reverse(true).Bold(true)
	stHelp   = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	stStatus = tcell.StyleDefault.Foreground(tcell.ColorAqua)
	stBox    = tcell.StyleDefault.Foreground(tcell.ColorGreen)
	stDim    = tcell.StyleDefault.Foreground(tcell.ColorGray)
	stLog    = tcell.StyleDefault
)

// put draws s at (x,y) clipped to maxW columns, never splitting wide runes.
func (a *app) put(x, y int, s string, st tcell.Style, maxW int) {
	for _, c := range term.Layout(s, maxW, true) {
		a.s.SetContent(x+c.Col, y, c.Rune, nil, st)
	}
}

func (a *app) fillRow(y int, st tcell.Style) {
	w, _ := a.s.Size()
	for x := 0; x < w; x++ {
		a.s.SetContent(x, y, ' ', nil, st)
	}
}

const boxInner = 30

// widthSamples are drawn inside a box whose right edge is computed from
// runewidth. If the terminal agrees with our width math, every right edge
// lines up.
var widthSamples = []string{
	"한글 파일명.txt",
	"abc가나다def",
	"nginx-설정.conf 백업",
	"모호폭: …│※①○",
	"이모지: 🙂👍 (참고용)",
}

func (a *app) draw() {
	a.s.Clear()
	w, h := a.s.Size()

	a.fillRow(0, stHeader)
	a.put(0, 0, fmt.Sprintf(" tuidesk evlog │ %s │ TERM=%s │ %d색 │ %dx%d │ 마우스=%s",
		a.label, os.Getenv("TERM"), a.s.Colors(), w, h, modeNames[a.mode]), stHeader, w)
	a.put(0, 1, " F2 OSC52 복사  F5 OSC52 읽기  F3 로그 지우기  F4 마우스 모드 전환  F10 또는 Ctrl+C 두 번: 종료", stHelp, w)
	a.put(0, 2, fmt.Sprintf(" 포인터 %s │ 제스처 %s │ 붙여넣기 %s",
		orDash(a.mouseStatus), orDash(a.lastGesture), orDash(a.lastPaste)), stStatus, w)

	// Width test box.
	top := 3
	a.put(1, top, "┌"+strings.Repeat("─", boxInner)+"┐", stBox, w-1)
	for i, sample := range widthSamples {
		y := top + 1 + i
		a.put(1, y, "│", stBox, w-1)
		a.put(2, y, term.PadRight(sample, boxInner), stLog, w-2)
		a.put(2+boxInner, y, "│", stBox, w-2-boxInner)
	}
	a.put(1, top+1+len(widthSamples), "└"+strings.Repeat("─", boxInner)+"┘", stBox, w-1)
	a.put(boxInner+5, top+1, "← 오른쪽 세로선이 모두 한 줄로 맞으면 폭 계산 OK", stDim, w-boxInner-5)
	a.put(boxInner+5, top+2, "  한글 글자 위를 클릭해 보고하는 x좌표를 확인하세요", stDim, w-boxInner-5)
	a.put(boxInner+5, top+4, "  모호폭 줄이 어긋나면 터미널의 '모호한 문자 2칸' 설정 확인", stDim, w-boxInner-5)
	a.put(boxInner+5, top+5, "  이모지 줄은 터미널마다 달라 참고만 합니다", stDim, w-boxInner-5)

	// Log area.
	logTop := top + len(widthSamples) + 3
	a.fillRow(logTop-1, stDim)
	a.put(0, logTop-1, "─── 이벤트 로그 (최신이 아래) "+strings.Repeat("─", max(0, w)), stDim, w)
	rows := h - logTop
	if rows > 0 {
		start := max(0, len(a.lines)-rows)
		for i, line := range a.lines[start:] {
			a.put(0, logTop+i, line, stLog, w)
		}
	}

	// Pointer marker: invert the cell the terminal says the mouse is on.
	if a.mx >= 0 && a.my >= 0 && a.mx < w && a.my < h {
		r, comb, st, _ := a.s.GetContent(a.mx, a.my)
		a.s.SetContent(a.mx, a.my, r, comb, st.Reverse(true))
	}
	a.s.Show()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
