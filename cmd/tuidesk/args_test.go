package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleArgs(t *testing.T) {
	for _, tc := range []struct {
		args        []string
		run         bool
		code        int
		out, errOut string
	}{
		{args: nil, run: true},
		{args: []string{"--version"}, out: "tuidesk dev (" + buildFlavor + " build)\n"},
		{args: []string{"-v"}, out: "tuidesk dev"},
		{args: []string{"--help"}, out: "Usage: tuidesk"},
		{args: []string{"한글"}, code: 2, errOut: `unknown argument "한글"`},
	} {
		var out, errOut bytes.Buffer
		run, code := handleArgs(tc.args, &out, &errOut)
		if run != tc.run || code != tc.code {
			t.Errorf("%q: run=%v code=%d", tc.args, run, code)
		}
		if !strings.Contains(out.String(), tc.out) || !strings.Contains(errOut.String(), tc.errOut) {
			t.Errorf("%q: stdout %q stderr %q", tc.args, out.String(), errOut.String())
		}
		if tc.out == "" && out.Len() != 0 {
			t.Errorf("%q: unexpected stdout %q", tc.args, out.String())
		}
	}
}
