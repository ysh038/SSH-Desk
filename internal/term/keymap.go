package term

// dubeolsik maps Hangul compatibility jamo produced by the standard Korean
// 2-set (두벌식) layout to the QWERTY key in the same position.
//
// When the Korean input method is active, terminals that speak the kitty /
// modifyOtherKeys protocols (which tcell enables) report Ctrl+C as Ctrl+ㅊ.
// Shortcuts must be matched by physical key, so these are normalized back.
var dubeolsik = map[rune]rune{
	'ㅂ': 'q', 'ㅈ': 'w', 'ㄷ': 'e', 'ㄱ': 'r', 'ㅅ': 't',
	'ㅛ': 'y', 'ㅕ': 'u', 'ㅑ': 'i', 'ㅐ': 'o', 'ㅔ': 'p',
	'ㅁ': 'a', 'ㄴ': 's', 'ㅇ': 'd', 'ㄹ': 'f', 'ㅎ': 'g',
	'ㅗ': 'h', 'ㅓ': 'j', 'ㅏ': 'k', 'ㅣ': 'l',
	'ㅋ': 'z', 'ㅌ': 'x', 'ㅊ': 'c', 'ㅍ': 'v', 'ㅠ': 'b',
	'ㅜ': 'n', 'ㅡ': 'm',
	// Shifted jamo: shortcuts ignore case, so map to the same letter.
	'ㅃ': 'q', 'ㅉ': 'w', 'ㄸ': 'e', 'ㄲ': 'r', 'ㅆ': 't',
	'ㅒ': 'o', 'ㅖ': 'p',
}

// QwertyFromJamo returns the lowercase QWERTY letter on the same physical key
// as the given 2-set Hangul jamo.
func QwertyFromJamo(r rune) (rune, bool) {
	q, ok := dubeolsik[r]
	return q, ok
}

// ShortcutRune normalizes the rune of a key event for shortcut matching:
// when Ctrl or Alt is held and the rune is a 2-set jamo, the QWERTY letter is
// returned instead. Plain typing (no Ctrl/Alt) is never changed, so Hangul
// text input keeps working.
func ShortcutRune(r rune, mod Mod) rune {
	if mod&(ModCtrl|ModAlt) == 0 {
		return r
	}
	if q, ok := QwertyFromJamo(r); ok {
		return q
	}
	return r
}
