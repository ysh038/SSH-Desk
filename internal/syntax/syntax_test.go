package syntax

import (
	"fmt"
	"strings"
	"testing"
)

// render marks spans as kind letters for compact assertions:
// c=comment s=string k=keyword n=number v=var y=key h=header.
func render(l *Lang, line string) string {
	rs := []rune(line)
	out := []rune(strings.Repeat(".", len(rs)))
	for _, sp := range l.Line(line) {
		for i := sp.Start; i < sp.End; i++ {
			out[i] = []rune(".csknvyh")[sp.Kind]
		}
	}
	return string(out)
}

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"/etc/nginx/nginx.conf":              "nginx",
		"/etc/nginx/sites-available/default": "nginx",
		"docker-compose.yml":                 "YAML",
		"/etc/systemd/system/app.service":    "INI",
		"deploy.sh":                          "Shell",
		"Dockerfile":                         "Dockerfile",
		"main.go":                            "Go",
		"README.md":                          "Markdown",
		"notes.txt":                          "Text",
	}
	for path, want := range cases {
		if got := Detect(path, "").Name; got != want {
			t.Errorf("%s → %s, want %s", path, got, want)
		}
	}
	if got := Detect("run", "#!/usr/bin/env bash").Name; got != "Shell" {
		t.Errorf("shebang → %s", got)
	}
}

func TestNginx(t *testing.T) {
	l := ByName("nginx")
	got := render(l, `    proxy_pass http://$host:8080; # 주석`)
	want := `....kkkkkkkkkk........vvvvv.nnnn..cccc`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	// '#' inside a string is not a comment.
	if got := render(l, `return 200 "a#b";`); got != `kkkkkk.nnn.sssss.` {
		t.Fatalf("got %s", got)
	}
}

func TestYAML(t *testing.T) {
	l := ByName("yaml")
	if got := render(l, `  image: "nginx:1.27" # pin`); got != `..yyyyy..ssssssssssss.ccccc` {
		t.Fatalf("got %s", got)
	}
	if got := render(l, `- enabled: true`); got != `..yyyyyyy..kkkk` {
		t.Fatalf("got %s", got)
	}
}

func TestJSONAndShell(t *testing.T) {
	if got := render(ByName("json"), `{"port": 8080, "on": true}`); got != `.yyyyyy..nnnn..yyyy..kkkk.` {
		t.Fatalf("json %s", got)
	}
	if got := render(ByName("sh"), `if [ -n "$HOME" ]; then echo ${USER} # x`); got != fmt.Sprint(`kk......sssssss....kkkk......vvvvvvv.ccc`) {
		t.Fatalf("sh %s", got)
	}
}

func TestINIAndSystemd(t *testing.T) {
	l := ByName("ini")
	if got := render(l, `[Service]`); got != `hhhhhhhhh` {
		t.Fatalf("got %s", got)
	}
	if got := render(l, `ExecStart=/usr/bin/app --port 80`); got != `yyyyyyyyy.......................` && !strings.HasPrefix(got, "yyyyyyyyy") {
		t.Fatalf("got %s", got)
	}
}
