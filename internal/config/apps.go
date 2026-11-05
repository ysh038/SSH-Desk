package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// AppManifest registers an external program (SPEC 8.3 step 1):
// ~/.config/tuidesk/apps/<name>.toml. It runs in a terminal window and is
// listed in the launcher; a desktop icon can use it with app = "<name>".
type AppManifest struct {
	Name    string `toml:"name"`
	Icon    string `toml:"icon"`    // short text shown on the desktop icon, e.g. "G"
	Command string `toml:"command"` // run with /bin/sh -c
	// Cwd is the starting folder: a path, "~", "{selected_dir}" (the folder
	// of the focused file manager) or empty (home).
	Cwd string `toml:"cwd"`
	// Hold keeps the window open after the program exits.
	Hold bool `toml:"hold"`
}

// AppsDir is the manifest folder next to config.toml.
func AppsDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "apps")
}

// LoadApps reads every *.toml in dir. Broken files are reported in errs and
// skipped. A missing folder is not an error.
func LoadApps(dir string) (apps []AppManifest, errs []error) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.toml"))
	sort.Strings(matches)
	for _, m := range matches {
		var a AppManifest
		data, err := os.ReadFile(m)
		if err == nil {
			_, err = toml.Decode(string(data), &a)
		}
		if err == nil && strings.TrimSpace(a.Command) == "" {
			err = fmt.Errorf("no command")
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(m), err))
			continue
		}
		if a.Name == "" {
			a.Name = strings.TrimSuffix(filepath.Base(m), ".toml")
		}
		apps = append(apps, a)
	}
	return apps, errs
}
