package files

import "testing"

func TestIconText(t *testing.T) {
	for _, c := range []struct {
		name      string
		dir, link bool
		mode      uint32
		want      string
	}{
		{"etc", true, false, 0o755, "[DIR]"},
		{"current", true, true, 0o777, "[LNK]"},
		{"nginx.conf", false, false, 0o644, "[CFG]"},
		{"deploy.sh", false, false, 0o755, "[SH ]"},
		{"tuidesk", false, false, 0o755, "[EXE]"},
		{"README", false, false, 0o644, "[TXT]"},
		{"syslog.2.gz", false, false, 0o640, "[ZIP]"},
		{"access.log.1", false, false, 0o640, "[LOG]"},
		{"설정.yaml", false, false, 0o644, "[CFG]"},
		{"blob.bin", false, false, 0o644, "[---]"},
	} {
		if got := IconText(c.name, c.dir, c.link, c.mode); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
