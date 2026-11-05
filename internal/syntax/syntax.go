// Package syntax colors one line at a time with small per-language regex
// rules (ADR 0003 §3). Multi-line constructs are not tracked; the priority
// languages (nginx, yaml, json, toml, ini, sh, systemd, Dockerfile, python,
// go, markdown) are line oriented enough for this to look right.
package syntax

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Kind is a token class.
type Kind uint8

const (
	Plain Kind = iota
	Comment
	String
	Keyword
	Number
	Var
	Key
	Header
)

// Span colors runes [Start, End) of a line.
type Span struct {
	Start, End int
	Kind       Kind
}

type rule struct {
	re    *regexp.Regexp
	kind  Kind
	group int // submatch to color (0 = whole match)
}

// Lang is a highlighter.
type Lang struct {
	Key         string // map key: "nginx", "yaml", … (snippet files use it)
	Name        string
	rules       []rule
	ColonIndent bool // new level after a trailing ':' (YAML, Python)
	Comment     string
}

func r(kind Kind, expr string) rule { return rule{re: regexp.MustCompile(expr), kind: kind} }
func rg(kind Kind, expr string, g int) rule {
	return rule{re: regexp.MustCompile(expr), kind: kind, group: g}
}

var (
	hashComment = r(Comment, `#.*$`)
	dq          = r(String, `"(?:[^"\\]|\\.)*"?`)
	sq          = r(String, `'[^']*'?`)
	num         = r(Number, `\b\d+(?:\.\d+)?[kKmMgGsh]?\b`)
	shVar       = r(Var, `\$(?:\{[^}]*\}|[A-Za-z_][A-Za-z0-9_]*|[0-9@#?$!*-])`)
)

func words(kind Kind, ws ...string) rule {
	return r(kind, `\b(?:`+strings.Join(ws, "|")+`)\b`)
}

var langs = map[string]*Lang{
	"nginx": {Name: "nginx", Comment: "#", rules: []rule{
		hashComment, dq, sq, shVar,
		rg(Keyword, `^\s*([a-z_][a-z0-9_]*)`, 1),
		num,
	}},
	"yaml": {Name: "YAML", Comment: "#", ColonIndent: true, rules: []rule{
		rg(Comment, `(?:^|\s)(#.*)$`, 1), dq, sq,
		rg(Key, `^\s*(?:-\s+)?([^\s:#][^:#]*?)\s*:(?:\s|$)`, 1),
		words(Keyword, "true", "false", "yes", "no", "on", "off", "null"),
		r(Header, `^---$|^\.\.\.$`),
		num,
	}},
	"json": {Name: "JSON", rules: []rule{
		rg(Key, `("(?:[^"\\]|\\.)*")\s*:`, 1), dq,
		words(Keyword, "true", "false", "null"),
		r(Number, `-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b`),
	}},
	"toml": {Name: "TOML", Comment: "#", rules: []rule{
		hashComment, r(Header, `^\s*\[\[?[^\]]*\]\]?`), dq, sq,
		rg(Key, `^\s*([A-Za-z0-9_.\-]+)\s*=`, 1),
		words(Keyword, "true", "false"), num,
	}},
	"ini": {Name: "INI", Comment: "#", rules: []rule{
		r(Comment, `^\s*[#;].*$`), r(Header, `^\s*\[[^\]]*\]`),
		rg(Key, `^\s*([^=\s#;][^=]*?)\s*=`, 1), dq, num,
	}},
	"sh": {Name: "Shell", Comment: "#", rules: []rule{
		rg(Comment, `(?:^|\s)(#.*)$`, 1), dq, sq, shVar,
		words(Keyword, "if", "then", "else", "elif", "fi", "for", "in", "do", "done", "while", "until",
			"case", "esac", "function", "return", "export", "local", "readonly", "set", "shift", "exit", "source"),
		num,
	}},
	"dockerfile": {Name: "Dockerfile", Comment: "#", rules: []rule{
		hashComment,
		r(Keyword, `(?i)^\s*(?:FROM|RUN|CMD|COPY|ADD|ENV|EXPOSE|WORKDIR|ENTRYPOINT|USER|ARG|LABEL|VOLUME|HEALTHCHECK|SHELL|STOPSIGNAL|ONBUILD)\b`),
		dq, sq, shVar, num,
	}},
	"python": {Name: "Python", Comment: "#", ColonIndent: true, rules: []rule{
		hashComment, r(String, `[rbfRBF]?"(?:[^"\\]|\\.)*"?`), r(String, `[rbfRBF]?'(?:[^'\\]|\\.)*'?`),
		r(Header, `^\s*@[\w.]+`),
		words(Keyword, "def", "class", "return", "if", "elif", "else", "for", "while", "in", "not", "and", "or",
			"import", "from", "as", "with", "try", "except", "finally", "raise", "pass", "break", "continue",
			"lambda", "yield", "None", "True", "False", "async", "await", "is", "global", "nonlocal"),
		num,
	}},
	"go": {Name: "Go", Comment: "//", rules: []rule{
		r(Comment, `//.*$`), r(Comment, `/\*.*?\*/`), dq, r(String, "`[^`]*`?"), r(String, `'(?:[^'\\]|\\.)'`),
		words(Keyword, "package", "import", "func", "return", "if", "else", "for", "range", "switch", "case",
			"default", "go", "defer", "chan", "select", "struct", "interface", "map", "type", "var", "const",
			"break", "continue", "fallthrough", "goto", "nil", "true", "false", "iota"),
		num,
	}},
	"markdown": {Name: "Markdown", rules: []rule{
		r(Header, `^#{1,6}\s.*$`), r(String, "`[^`]+`"), r(Keyword, `^\s*(?:[-*+]|\d+\.)\s`),
		r(Var, `\[[^\]]*\]\([^)]*\)`),
	}},
	"text": {Name: "Text"},
}

func init() {
	for k, l := range langs {
		l.Key = k
	}
}

// Detect picks a language from the path and the first line.
func Detect(path, firstLine string) *Lang {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(base))
	dir := filepath.ToSlash(filepath.Dir(path))
	switch {
	case strings.Contains(dir, "/nginx") || ext == ".nginx" || base == "nginx.conf":
		return langs["nginx"]
	case ext == ".yaml" || ext == ".yml":
		return langs["yaml"]
	case ext == ".json":
		return langs["json"]
	case ext == ".toml":
		return langs["toml"]
	case ext == ".service" || ext == ".timer" || ext == ".socket" || ext == ".mount" || ext == ".target" ||
		ext == ".ini" || ext == ".cfg" || ext == ".desktop":
		return langs["ini"]
	case ext == ".sh" || ext == ".bash" || base == ".bashrc" || base == ".profile" || base == ".bash_profile" ||
		strings.HasPrefix(firstLine, "#!") && (strings.Contains(firstLine, "sh")):
		return langs["sh"]
	case base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || ext == ".dockerfile":
		return langs["dockerfile"]
	case ext == ".py" || strings.HasPrefix(firstLine, "#!") && strings.Contains(firstLine, "python"):
		return langs["python"]
	case ext == ".go":
		return langs["go"]
	case ext == ".md" || ext == ".markdown":
		return langs["markdown"]
	case ext == ".conf":
		return langs["ini"]
	}
	return langs["text"]
}

// ByName returns a language by its key (nginx, yaml, …).
func ByName(name string) *Lang { return langs[name] }

// Line highlights one line. At each position the earliest match wins; ties
// go to the rule listed first.
func (l *Lang) Line(line string) []Span {
	if l == nil || len(l.rules) == 0 || line == "" {
		return nil
	}
	var out []Span
	pos := 0 // byte offset
	for pos < len(line) {
		bestStart, bestEnd, bestKind := -1, -1, Plain
		for _, ru := range l.rules {
			loc := ru.re.FindStringSubmatchIndex(line[pos:])
			if loc == nil {
				continue
			}
			// Anchored rules (^) only match at the true line start.
			if pos > 0 && strings.HasPrefix(ru.re.String(), "^") || pos > 0 && strings.HasPrefix(ru.re.String(), "(?i)^") {
				continue
			}
			s, e := loc[0], loc[1]
			if ru.group > 0 && len(loc) > 2*ru.group+1 && loc[2*ru.group] >= 0 {
				s, e = loc[2*ru.group], loc[2*ru.group+1]
			}
			if e <= s {
				continue
			}
			if bestStart < 0 || pos+s < bestStart {
				bestStart, bestEnd, bestKind = pos+s, pos+e, ru.kind
			}
		}
		if bestStart < 0 {
			break
		}
		out = append(out, Span{
			Start: utf8.RuneCountInString(line[:bestStart]),
			End:   utf8.RuneCountInString(line[:bestEnd]),
			Kind:  bestKind,
		})
		pos = bestEnd
	}
	return out
}
