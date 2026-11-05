// Package snippets loads snippet files (SPEC 7.4) and expands tab stops:
// ${1:default}, ${2}, $3 and $0 (final cursor). Bundled files are embedded;
// users add more in ~/.config/tuidesk/snippets/*.toml.
package snippets

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"tuidesk/assets"
)

// Snippet is one template.
type Snippet struct {
	Trigger string `toml:"trigger"`
	Title   string `toml:"title"`
	Body    string `toml:"body"`
	Lang    string `toml:"-"` // syntax key ("nginx", "yaml", …) or "*"
}

type file struct {
	Lang    string    `toml:"lang"`
	Snippet []Snippet `toml:"snippet"`
}

// Set is all loaded snippets.
type Set struct {
	All    []Snippet
	Errors []error // files that could not be read (shown to the user)
}

// Load reads the bundled snippets and every *.toml in userDir (if any).
// User snippets come first so they can override a bundled trigger.
func Load(userDir string) *Set {
	s := &Set{}
	if userDir != "" {
		matches, _ := filepath.Glob(filepath.Join(userDir, "*.toml"))
		sort.Strings(matches)
		for _, m := range matches {
			data, err := os.ReadFile(m)
			if err == nil {
				err = s.add(data, strings.TrimSuffix(filepath.Base(m), ".toml"))
			}
			if err != nil {
				s.Errors = append(s.Errors, err)
			}
		}
	}
	_ = fs.WalkDir(assets.Snippets, "snippets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := assets.Snippets.ReadFile(p)
		if err := s.add(data, strings.TrimSuffix(filepath.Base(p), ".toml")); err != nil {
			s.Errors = append(s.Errors, err)
		}
		return nil
	})
	return s
}

func (s *Set) add(data []byte, name string) error {
	var f file
	if _, err := toml.Decode(string(data), &f); err != nil {
		return err
	}
	if f.Lang == "" {
		f.Lang = name
	}
	for _, sn := range f.Snippet {
		sn.Lang = f.Lang
		s.All = append(s.All, sn)
	}
	return nil
}

// For returns snippets usable in language lang (its own and "*").
func (s *Set) For(lang string) []Snippet {
	var out []Snippet
	for _, sn := range s.All {
		if sn.Lang == lang || sn.Lang == "*" {
			out = append(out, sn)
		}
	}
	return out
}

// Find returns the snippet with trigger for lang (user files win).
func (s *Set) Find(lang, trigger string) (Snippet, bool) {
	for _, sn := range s.For(lang) {
		if sn.Trigger == trigger {
			return sn, true
		}
	}
	return Snippet{}, false
}

// Stop is a tab stop as rune offsets into the expanded text.
type Stop struct {
	N          int
	Start, End int
}

// Expand turns a body into plain text and its stops in visiting order
// (1, 2, …, then 0). Without $0 the final stop is the end of the text.
// "\$" is a literal dollar; "$name" (not a digit) is kept as is so nginx
// and shell variables survive.
func Expand(body string) (string, []Stop) {
	rs := []rune(body)
	var out []rune
	var stops []Stop
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if c == '\\' && i+1 < len(rs) && rs[i+1] == '$' {
			out = append(out, '$')
			i++
			continue
		}
		if c != '$' || i+1 >= len(rs) {
			out = append(out, c)
			continue
		}
		j := i + 1
		braced := rs[j] == '{'
		if braced {
			j++
		}
		k := j
		for k < len(rs) && rs[k] >= '0' && rs[k] <= '9' {
			k++
		}
		if k == j { // not a tab stop
			out = append(out, c)
			continue
		}
		n := atoi(rs[j:k])
		def := []rune{}
		if braced {
			if k < len(rs) && rs[k] == ':' {
				end := k + 1
				for end < len(rs) && rs[end] != '}' {
					end++
				}
				def = rs[k+1 : end]
				k = end
			}
			if k >= len(rs) || rs[k] != '}' { // malformed: keep literally
				out = append(out, c)
				continue
			}
			k++
		}
		stops = append(stops, Stop{N: n, Start: len(out), End: len(out) + len(def)})
		out = append(out, def...)
		i = k - 1
	}
	sort.SliceStable(stops, func(a, b int) bool {
		if (stops[a].N == 0) != (stops[b].N == 0) {
			return stops[b].N == 0
		}
		return stops[a].N < stops[b].N
	})
	if len(stops) == 0 || stops[len(stops)-1].N != 0 {
		stops = append(stops, Stop{N: 0, Start: len(out), End: len(out)})
	}
	return string(out), stops
}

func atoi(rs []rune) int {
	n := 0
	for _, r := range rs {
		n = n*10 + int(r-'0')
	}
	return n
}

// ExpandPlain is Expand without the stops.
func ExpandPlain(body string) string {
	s, _ := Expand(body)
	return s
}
