package sudo

import (
	"reflect"
	"testing"
)

func TestArgs(t *testing.T) {
	if got, want := Args(false, "umount", "/media/kim/USB"), []string{"-n", "-k", "umount", "/media/kim/USB"}; !reflect.DeepEqual(got, want) {
		t.Errorf("no password: %q", got)
	}
	if got, want := Args(true, "sh", "-c", ":"), []string{"-S", "-k", "-p", "", "sh", "-c", ":"}; !reflect.DeepEqual(got, want) {
		t.Errorf("password: %q", got)
	}
}

func TestInput(t *testing.T) {
	if got := string(Input(nil, []byte("data"))); got != "data" {
		t.Errorf("no password: %q", got)
	}
	// A Hangul password must reach sudo as UTF-8, followed by one newline.
	pw := []rune("비밀pw")
	if got := string(Input(pw, []byte("data"))); got != "비밀pw\ndata" {
		t.Errorf("password: %q", got)
	}
	in := Input(pw, nil)
	Wipe(in)
	WipeRunes(pw)
	for _, b := range in {
		if b != 0 {
			t.Fatal("Wipe left bytes behind")
		}
	}
	for _, r := range pw {
		if r != 0 {
			t.Fatal("WipeRunes left runes behind")
		}
	}
}

func TestExplain(t *testing.T) {
	cases := map[string]string{
		"sudo: 1 incorrect password attempt":               "The sudo password was not accepted.",
		"Sorry, try again.":                                "The sudo password was not accepted.",
		"kim is not in the sudoers file.":                  "kim is not allowed to use sudo on this server.",
		"mount: /media/kim/USB: wrong fs type, bad option": "",
		"umount: /media/kim/USB: target is busy.":          "",
	}
	for out, want := range cases {
		if got := Explain([]byte(out), "kim"); got != want {
			t.Errorf("Explain(%q) = %q, want %q", out, got, want)
		}
	}
}
