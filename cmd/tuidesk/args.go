package main

import (
	"fmt"
	"io"
)

// version is set at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

const usage = `Usage: tuidesk [--version | --help]

TUI desktop for Linux servers over ssh. Run it with no arguments.
Settings: ~/.config/tuidesk/config.toml (created on first run).
`

// handleArgs deals with command-line arguments. run is false when the
// program should exit with code instead of starting the desktop.
func handleArgs(args []string, stdout, stderr io.Writer) (run bool, code int) {
	if len(args) == 0 {
		return true, 0
	}
	switch args[0] {
	case "--version", "-version", "-v", "version":
		fmt.Fprintf(stdout, "tuidesk %s (%s build)\n", version, buildFlavor)
		return false, 0
	case "--help", "-help", "-h", "help":
		fmt.Fprint(stdout, usage)
		return false, 0
	}
	fmt.Fprintf(stderr, "tuidesk: unknown argument %q\n\n%s", args[0], usage)
	return false, 2
}
