package snippets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledSnippetsLoad(t *testing.T) {
	s := Load("")
	if len(s.Errors) > 0 {
		t.Fatalf("errors: %v", s.Errors)
	}
	for _, want := range []struct{ lang, trigger string }{
		{"nginx", "server"}, {"nginx", "proxy"}, {"ini", "service"}, {"sh", "bash"},
		{"yaml", "healthcheck"}, {"nginx", "cron"}, // "*" snippets are available everywhere
	} {
		if _, ok := s.Find(want.lang, want.trigger); !ok {
			t.Errorf("missing %s/%s", want.lang, want.trigger)
		}
	}
}

func TestUserSnippetsOverride(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "nginx.toml"), []byte("[[snippet]]\ntrigger = \"server\"\ntitle = \"mine\"\nbody = \"server {}\"\n"), 0o644)
	sn, ok := Load(dir).Find("nginx", "server")
	if !ok || sn.Title != "mine" {
		t.Fatalf("got %+v", sn)
	}
	os.WriteFile(filepath.Join(dir, "bad.toml"), []byte("[[snippet]\n"), 0o644)
	if len(Load(dir).Errors) != 1 {
		t.Fatal("broken user file should be reported")
	}
}

func TestExpand(t *testing.T) {
	text, stops := Expand("listen ${1:80};\nserver_name ${2:example.com};\nreturn 301 https://$host$request_uri;$0")
	want := "listen 80;\nserver_name example.com;\nreturn 301 https://$host$request_uri;"
	if text != want {
		t.Fatalf("text %q", text)
	}
	if len(stops) != 3 || stops[0].N != 1 || string([]rune(text)[stops[0].Start:stops[0].End]) != "80" ||
		string([]rune(text)[stops[1].Start:stops[1].End]) != "example.com" || stops[2].N != 0 || stops[2].Start != len([]rune(text)) {
		t.Fatalf("stops %+v", stops)
	}
}

func TestExpandEdgeCases(t *testing.T) {
	text, stops := Expand(`price \$5 and $x then ${2} and $1`)
	if text != "price $5 and $x then  and " {
		t.Fatalf("text %q", text)
	}
	if len(stops) != 3 || stops[0].N != 1 || stops[1].N != 2 || stops[2].N != 0 {
		t.Fatalf("stops %+v", stops)
	}
	if text, _ := Expand("${1:한글} x"); text != "한글 x" {
		t.Fatalf("hangul default %q", text)
	}
}
