package term

import "testing"

func TestQwertyFromJamoCoversCommonShortcuts(t *testing.T) {
	// Editor shortcuts from SPEC 7.3 typed with the Korean IME active.
	cases := map[rune]rune{
		'ㄴ': 's', // save
		'ㅂ': 'q', // quit
		'ㅋ': 'z', // undo
		'ㅛ': 'y', // redo
		'ㅊ': 'c', // copy
		'ㅌ': 'x', // cut
		'ㅍ': 'v', // paste
		'ㄹ': 'f', // find
		'ㄱ': 'r', // replace
		'ㅎ': 'g', // go to line
		'ㅁ': 'a', // select all
		'ㅗ': 'h', // hidden files
		'ㅃ': 'q', // shifted jamo
		'ㅖ': 'p',
	}
	for in, want := range cases {
		got, ok := QwertyFromJamo(in)
		if !ok || got != want {
			t.Errorf("QwertyFromJamo(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
}

func TestQwertyFromJamoCoversAllLetters(t *testing.T) {
	seen := map[rune]bool{}
	for _, q := range dubeolsik {
		seen[q] = true
	}
	for c := 'a'; c <= 'z'; c++ {
		if !seen[c] {
			t.Errorf("letter %q has no jamo mapping", c)
		}
	}
}

func TestQwertyFromJamoRejectsOthers(t *testing.T) {
	for _, r := range []rune{'a', '한', '가', '1', ' '} {
		if _, ok := QwertyFromJamo(r); ok {
			t.Errorf("QwertyFromJamo(%q) should not map", r)
		}
	}
}

func TestShortcutRune(t *testing.T) {
	cases := []struct {
		r    rune
		mod  Mod
		want rune
	}{
		{'ㅊ', ModCtrl, 'c'},
		{'ㄴ', ModCtrl, 's'},
		{'ㅁ', ModAlt, 'a'},
		{'ㅊ', ModCtrl | ModShift, 'c'},
		{'ㅊ', 0, 'ㅊ'},        // plain typing is untouched
		{'ㅊ', ModShift, 'ㅊ'}, // shift alone is still typing
		{'한', ModCtrl, '한'},  // composed syllables are not keys
		{'s', ModCtrl, 's'},
	}
	for _, c := range cases {
		if got := ShortcutRune(c.r, c.mod); got != c.want {
			t.Errorf("ShortcutRune(%q,%v) = %q want %q", c.r, c.mod, got, c.want)
		}
	}
}
