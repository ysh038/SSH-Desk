package term

import "testing"

func layoutString(cells []Cell) (string, int) {
	s := ""
	w := 0
	for _, c := range cells {
		s += string(c.Rune)
		w = c.Col + c.Width
	}
	return s, w
}

func TestWidthHangul(t *testing.T) {
	cases := map[string]int{
		"abc":           3,
		"한글":            4,
		"한글 파일명.txt":    15,
		"nginx-설정.conf": 15,
	}
	for s, want := range cases {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestLayoutNeverSplitsWideRune(t *testing.T) {
	// "가나다" is 6 columns; 5 columns must hold only two syllables.
	s, w := layoutString(Layout("가나다", 5, false))
	if s != "가나" || w != 4 {
		t.Fatalf("got %q width %d", s, w)
	}
}

func TestLayoutEllipsis(t *testing.T) {
	cases := []struct {
		in    string
		max   int
		want  string
		width int
	}{
		{"한글 파일명.txt", 8, "한글 파…", 8},
		{"한글", 4, "한글", 4},  // fits exactly: no ellipsis
		{"가나다", 4, "가…", 3}, // wide rune would overflow the reserved column
		{"abcdef", 4, "abc…", 4},
	}
	for _, c := range cases {
		s, w := layoutString(Layout(c.in, c.max, true))
		if s != c.want || w != c.width {
			t.Errorf("Layout(%q,%d) = %q (w=%d), want %q (w=%d)", c.in, c.max, s, w, c.want, c.width)
		}
	}
}

func TestPadRight(t *testing.T) {
	if got := PadRight("한글", 6); got != "한글  " {
		t.Fatalf("got %q", got)
	}
	if got := PadRight("가나다", 5); got != "가나 " {
		t.Fatalf("got %q", got)
	}
	if Width(PadRight("한글 파일명", 20)) != 20 {
		t.Fatal("padded width mismatch")
	}
}

func TestAmbiguousRunesAreNarrowByDefault(t *testing.T) {
	// Even under a Korean locale, '…' and '│' must be 1 column like tcell.
	t.Setenv("LANG", "ko_KR.UTF-8")
	t.Setenv("RUNEWIDTH_EASTASIAN", "")
	widthCond = newWidthCond()
	defer func() { widthCond = newWidthCond() }()
	for _, r := range []rune{'…', '│', '※'} {
		if w := RuneWidth(r); w != 1 {
			t.Errorf("RuneWidth(%q) = %d, want 1", r, w)
		}
	}
	if w := RuneWidth('한'); w != 2 {
		t.Errorf("RuneWidth('한') = %d, want 2", w)
	}
}

func TestAmbiguousRunesWideWhenRequested(t *testing.T) {
	t.Setenv("RUNEWIDTH_EASTASIAN", "1")
	widthCond = newWidthCond()
	defer func() {
		widthCond = newWidthCond()
	}()
	if w := RuneWidth('…'); w != 2 {
		t.Errorf("RuneWidth('…') = %d, want 2", w)
	}
}

func TestEscapeNonASCII(t *testing.T) {
	bs := string(rune(92)) // backslash
	if got, want := EscapeNonASCII("한글 a.txt"), bs+"uD55C"+bs+"uAE00 a.txt"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := EscapeNonASCII("x\ty🙂"), "x"+bs+"x09y"+bs+"U0001F642"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
