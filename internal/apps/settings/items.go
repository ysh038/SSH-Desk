// Package settings is the Settings window (SPEC 7.7): every simple value of
// config.toml, grouped the way desktop systems group them, with a search
// over all of them. This file is the model: what the settings are and how
// they are found and changed. It does not draw anything.
package settings

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"tuidesk/internal/config"
)

// Kind is how a setting is shown and changed.
type Kind int

const (
	Toggle Kind = iota // on / off
	Choice             // one of Options
	Number             // an integer between Min and Max
	Text               // a line of text (a path)
	Action             // a button
	Info               // read-only text
	Probe              // the double-click test line
)

// Option is one value of a Choice.
type Option struct{ Value, Label string }

// Item is one line of the Settings window.
type Item struct {
	ID      string
	Section string
	Label   string
	// Help is the sentence shown under the list for the selected line.
	Help string
	// Keywords are extra words the search finds the item by.
	Keywords string
	Kind     Kind

	Options   []Option
	Min, Max  int
	Step      int
	Unit      string
	Empty     string // Text: shown when the value is ""
	Button    string // Action: the button text
	Value     string // Info: the text
	Bool      func(*config.Config) *bool
	Str       func(*config.Config) *string
	Int       func(*config.Config) *int
	Check     func(string) (string, error) // Text: tidy and validate
	ActionKey string                       // Action: what the window runs
}

// Sections in the order they are listed. The names follow the settings
// apps people know (macOS System Settings, Windows Settings).
var Sections = []string{
	"Appearance", "Date & Time", "Mouse", "Keyboard", "Notifications",
	"Files", "Text Editor", "Terminal", "Storage & USB", "Privacy & Security", "About",
}

// About is what the About section shows; main fills it in.
type About struct {
	Version    string
	Build      string // "lite" or "full"
	ConfigPath string
	// Details are more lines: terminal type, colors, size...
	Details [][2]string
}

// Stat is os.Stat; tests replace it.
var Stat = os.Stat

func checkFolder(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", nil
	}
	if !strings.HasPrefix(strings.TrimSpace(p), "/") {
		return "", errors.New("Type the full path, starting with /")
	}
	c := config.CleanMountDir(p)
	if c == "" {
		return "", errors.New("The root folder / cannot be used")
	}
	fi, err := Stat(c)
	switch {
	case err != nil:
		return "", errors.New("That folder does not exist. Create it first.")
	case !fi.IsDir():
		return "", errors.New("That is a file, not a folder")
	}
	return c, nil
}

func checkProgram(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", errors.New("Type the full path, e.g. /bin/bash")
	}
	fi, err := Stat(p)
	switch {
	case err != nil:
		return "", errors.New("That program does not exist")
	case fi.IsDir() || fi.Mode()&0o111 == 0:
		return "", errors.New("That file is not a program")
	}
	return p, nil
}

// Items returns every setting, in display order.
func Items(about About) []Item {
	zone, _ := time.Now().Zone()
	items := []Item{
		{ID: "desktop_icons", Section: "Appearance", Label: "Show desktop icons", Kind: Toggle,
			Help:     "The shortcuts on the desktop (Home, /etc, Terminal...). Hiding them does not delete them.",
			Keywords: "shortcuts background wallpaper", Bool: func(c *config.Config) *bool { return &c.DesktopIcons }},
		{ID: "show_host", Section: "Appearance", Label: "Show the server name in the taskbar", Kind: Toggle,
			Help:     "The name of the server you are connected to, next to the clock.",
			Keywords: "hostname computer name title", Bool: func(c *config.Config) *bool { return &c.ShowHost }},
		{ID: "shadows", Section: "Appearance", Label: "Window shadows", Kind: Toggle,
			Help:     "A dark edge to the right of and below windows, menus and dialogs, so they look raised.",
			Keywords: "shadow depth 3d look theme", Bool: func(c *config.Config) *bool { return &c.Shadows }},
		{ID: "hint_bar", Section: "Appearance", Label: "Show shortcut hints in windows", Kind: Toggle,
			Help:     "The line at the bottom of each window that lists its most useful keys.",
			Keywords: "help keys status bar bottom", Bool: func(c *config.Config) *bool { return &c.HintBar }},
		{ID: "names", Section: "Appearance", Label: "Names with non-English letters", Kind: Choice,
			Help:     "The Linux console has no Korean font. Automatic shows such names as \\uXXXX codes there and normally everywhere else.",
			Keywords: "korean hangul unicode font language region file names",
			Options:  []Option{{"auto", "Automatic"}, {"show", "Show as they are"}, {"escape", "Show as \\uXXXX codes"}},
			Str:      func(c *config.Config) *string { return &c.Names }},

		{ID: "clock", Section: "Date & Time", Label: "Clock format", Kind: Choice,
			Help: "How the clock in the top bar shows the time.", Keywords: "time 24 12 hour am pm",
			Options: []Option{{"24h", "24-hour (14:05)"}, {"12h", "12-hour (2:05 PM)"}},
			Str:     func(c *config.Config) *string { return &c.Clock }},
		{ID: "clock_seconds", Section: "Date & Time", Label: "Show seconds", Kind: Toggle,
			Help: "The clock then updates every second instead of every minute.", Keywords: "time clock",
			Bool: func(c *config.Config) *bool { return &c.ClockSeconds }},
		{ID: "clock_date", Section: "Date & Time", Label: "Show the date", Kind: Toggle,
			Help: "Shows the date before the time, e.g. 2026-11-18.", Keywords: "calendar day clock",
			Bool: func(c *config.Config) *bool { return &c.ClockDate }},
		{ID: "timezone", Section: "Date & Time", Label: "Time zone", Kind: Info, Value: zone,
			Help: "The time zone belongs to the server. An administrator changes it with: sudo timedatectl set-timezone", Keywords: "region utc kst"},

		{ID: "double_click", Section: "Mouse", Label: "Double-click speed", Kind: Number,
			Min: config.MinDoubleClickMs, Max: config.MaxDoubleClickMs, Step: 50, Unit: "ms",
			Help:     "The longest pause between two clicks that still counts as a double-click. Raise it if double-clicks are not recognized.",
			Keywords: "click interval slow fast", Int: func(c *config.Config) *int { return &c.DoubleClickMs }},
		{ID: "double_click_test", Section: "Mouse", Label: "Test: double-click this line", Kind: Probe,
			Help: "Double-click the line with the mouse to try the speed above.", Keywords: "try check"},
		{ID: "hover", Section: "Mouse", Label: "Highlight what the pointer is over", Kind: Toggle,
			Help:     "Buttons, tabs and rows light up under the mouse. Turn it off on a slow connection: the terminal then stops reporting every pointer movement.",
			Keywords: "hover highlight pointer movement motion slow connection", Bool: func(c *config.Config) *bool { return &c.Hover }},
		{ID: "wheel", Section: "Mouse", Label: "Scroll speed", Kind: Number,
			Min: config.MinWheelLines, Max: config.MaxWheelLines, Step: 1, Unit: "lines",
			Help: "How many lines one step of the mouse wheel scrolls.", Keywords: "wheel scrolling trackpad",
			Int: func(c *config.Config) *int { return &c.WheelLines }},

		{ID: "keyboard", Section: "Keyboard", Label: "Keyboard type", Kind: Choice,
			Help:     "Shortcuts are shown the way they are printed on your keyboard: Control/Option symbols for a Mac, Ctrl/Alt for a PC.",
			Keywords: "mac pc windows layout modifier ctrl alt option control labels",
			Options:  []Option{{"mac", "Mac"}, {"pc", "PC"}},
			Str:      func(c *config.Config) *string { return &c.Keyboard }},
		{ID: "shortcuts", Section: "Keyboard", Label: "Keyboard shortcuts", Kind: Action, Button: "Show",
			Help: "Opens the Help window with every shortcut.", Keywords: "keys hotkeys help list", ActionKey: "help"},

		{ID: "notify_seconds", Section: "Notifications", Label: "Show notifications for", Kind: Number,
			Min: config.MinNotifySeconds, Max: config.MaxNotifySeconds, Step: 1, Unit: "seconds",
			Help:     "How long a notification stays in the bottom-right corner.",
			Keywords: "toast message popup duration time", Int: func(c *config.Config) *int { return &c.NotifySeconds }},
		{ID: "notify_drives", Section: "Notifications", Label: "USB drive connected or removed", Kind: Toggle,
			Help:     "Shows a notification when a USB drive is plugged in or pulled out.",
			Keywords: "hotplug device removable alert", Bool: func(c *config.Config) *bool { return &c.NotifyDrives }},

		{ID: "show_hidden", Section: "Files", Label: "Show hidden files", Kind: Toggle,
			Help:     "Files whose names start with a dot. Applies to file windows opened from now on.",
			Keywords: "dotfiles dot folders", Bool: func(c *config.Config) *bool { return &c.ShowHidden }},
		{ID: "files_view", Section: "Files", Label: "Open folders in", Kind: Choice,
			Help: "How a new file window shows a folder.", Keywords: "list icons view layout default",
			Options: []Option{{"list", "List view"}, {"icons", "Icon view"}},
			Str:     func(c *config.Config) *string { return &c.FilesView }},
		{ID: "files_sort", Section: "Files", Label: "Sort files by", Kind: Choice,
			Help: "The order of files in a new file window. Folders always come first.", Keywords: "order arrange name size date",
			Options: []Option{{"name", "Name"}, {"size", "Size"}, {"date", "Date modified"}},
			Str:     func(c *config.Config) *string { return &c.FilesSort }},
		{ID: "confirm_trash", Section: "Files", Label: "Ask before moving files to the Trash", Kind: Toggle,
			Help:     "Off: Delete moves files to the Trash right away (Undo brings them back).",
			Keywords: "delete confirm remove recycle bin", Bool: func(c *config.Config) *bool { return &c.ConfirmTrash }},

		{ID: "tab_size", Section: "Text Editor", Label: "Tab width", Kind: Number,
			Min: config.MinTabSize, Max: config.MaxTabSize, Step: 1, Unit: "spaces",
			Help:     "How wide a tab is. A project's .editorconfig file overrides this.",
			Keywords: "indent indentation", Int: func(c *config.Config) *int { return &c.TabSize }},
		{ID: "expand_tabs", Section: "Text Editor", Label: "Insert spaces when Tab is pressed", Kind: Toggle,
			Help: "Off: the Tab key inserts a real tab character.", Keywords: "indent indentation soft tabs",
			Bool: func(c *config.Config) *bool { return &c.ExpandTabs }},
		{ID: "backup", Section: "Text Editor", Label: "Keep a .bak copy when saving", Kind: Toggle,
			Help:     "Before a file is overwritten, its old contents are kept next to it as name.bak.",
			Keywords: "backup save safety restore", Bool: func(c *config.Config) *bool { return &c.Backup }},

		{ID: "shell", Section: "Terminal", Label: "Shell program", Kind: Text, Empty: "Default (your login shell)",
			Help:     "The program a new terminal window starts, e.g. /bin/bash. Empty = your login shell.",
			Keywords: "bash zsh sh command line", Str: func(c *config.Config) *string { return &c.Shell }, Check: checkProgram},
		{ID: "scrollback", Section: "Terminal", Label: "Lines kept for scrolling back", Kind: Number,
			Min: config.MinScrollback, Max: config.MaxScrollback, Step: 1000, Unit: "lines",
			Help:     "How much earlier output a terminal window remembers. Applies to terminals opened from now on.",
			Keywords: "history buffer scroll", Int: func(c *config.Config) *int { return &c.Scrollback }},

		{ID: "mount_dir", Section: "Storage & USB", Label: "Mount USB drives at", Kind: Text,
			Empty:    "Default (/media/<user>)",
			Help:     "A folder that already exists, e.g. /mnt/offline. One drive at a time uses it; a second drive goes to the default place. Empty = default.",
			Keywords: "mount point location folder path removable drive disk stick",
			Str:      func(c *config.Config) *string { return &c.MountDir }, Check: checkFolder},
		{ID: "mount_ro", Section: "Storage & USB", Label: "Mount drives read-only", Kind: Toggle,
			Help:     "Files on a drive can then be read and copied from it but not changed. My PC can still mount one drive for writing.",
			Keywords: "write protect removable usb", Bool: func(c *config.Config) *bool { return &c.MountReadOnly }},
		{ID: "open_mypc", Section: "Storage & USB", Label: "Drives and USB devices", Kind: Action, Button: "Open My PC",
			Help: "Opens My PC: mount, open and eject drives.", Keywords: "disk devices eject unmount", ActionKey: "mypc"},

		{ID: "clipboard", Section: "Privacy & Security", Label: "Copy to this computer's clipboard", Kind: Toggle,
			Help:     "Copied text is also sent through the terminal to the computer you are sitting at (OSC 52). Off: it stays inside tuidesk.",
			Keywords: "osc52 paste copy remote local", Bool: func(c *config.Config) *bool { return &c.Clipboard }},
		{ID: "warn_root", Section: "Privacy & Security", Label: "Warn when started as root", Kind: Toggle,
			Help:     "tuidesk is meant to run as a normal user and asks for sudo only when needed.",
			Keywords: "sudo administrator superuser", Bool: func(c *config.Config) *bool { return &c.WarnRoot }},

		{ID: "version", Section: "About", Label: "Version", Kind: Info, Value: about.Version + " (" + about.Build + " build)",
			Help: "The lite build contains no network code.", Keywords: "release build"},
		{ID: "config_path", Section: "About", Label: "Settings file", Kind: Info, Value: about.ConfigPath,
			Help: "Everything in this window is stored in this file.", Keywords: "config toml"},
	}
	for _, d := range about.Details {
		items = append(items, Item{ID: "about:" + d[0], Section: "About", Label: d[0], Kind: Info, Value: d[1],
			Help: "What tuidesk detected about your terminal.", Keywords: "diagnostics doctor terminal"})
	}
	return append(items,
		Item{ID: "open_config", Section: "About", Label: "Edit the settings file", Kind: Action, Button: "Open",
			Help:     "Opens the settings file in the text editor. It also holds the desktop icons and the checks run after saving.",
			Keywords: "config toml advanced validators", ActionKey: "open_config"},
		Item{ID: "reset", Section: "About", Label: "Reset all settings", Kind: Action, Button: "Reset",
			Help:     "Puts every setting in this window back to its default. Desktop icons are kept.",
			Keywords: "defaults restore factory", ActionKey: "reset"},
	)
}

// Display is the value of the item as shown in the list.
func (it Item) Display(c config.Config) string {
	switch it.Kind {
	case Toggle:
		if *it.Bool(&c) {
			return "[x] On"
		}
		return "[ ] Off"
	case Choice:
		v := *it.Str(&c)
		for _, o := range it.Options {
			if o.Value == v {
				return o.Label
			}
		}
		if it.ID == "keyboard" {
			return "PC" // not chosen yet
		}
		return v
	case Number:
		return strconv.Itoa(*it.Int(&c)) + " " + it.Unit
	case Text:
		if v := *it.Str(&c); v != "" {
			return v
		}
		return it.Empty
	case Action:
		return "[ " + it.Button + " ]"
	case Info:
		return it.Value
	}
	return ""
}

// Flip changes a Toggle.
func (it Item) Flip(c *config.Config) {
	if it.Kind == Toggle {
		p := it.Bool(c)
		*p = !*p
	}
}

// SetNumber stores n, kept inside the item's range.
func (it Item) SetNumber(c *config.Config, n int) {
	if it.Kind == Number {
		*it.Int(c) = min(max(n, it.Min), it.Max)
	}
}

// ParseNumber reads what the user typed for a Number.
func (it Item) ParseNumber(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, errors.New("Type a whole number")
	}
	if n < it.Min || n > it.Max {
		return 0, fmt.Errorf("Type a number from %d to %d", it.Min, it.Max)
	}
	return n, nil
}

// Search returns the indexes of the items that contain every word of query
// in their section, label, help, keywords or choices (case-insensitive).
// Items whose own name matches come first: "clock" lists Clock format
// before a setting that only mentions the clock in its explanation. An
// empty query finds nothing.
func Search(items []Item, query string) []int {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	all := func(hay string) bool {
		for _, w := range words {
			if !strings.Contains(hay, w) {
				return false
			}
		}
		return true
	}
	var byName, other []int
	for i, it := range items {
		name := strings.ToLower(it.Section + " " + it.Label)
		hay := name + " " + strings.ToLower(it.Help+" "+it.Keywords+" "+it.Button)
		for _, o := range it.Options {
			hay += " " + strings.ToLower(o.Label)
		}
		switch {
		case all(name):
			byName = append(byName, i)
		case all(hay):
			other = append(other, i)
		}
	}
	return append(byName, other...)
}

// InSection returns the indexes of the items of one section.
func InSection(items []Item, section string) []int {
	var out []int
	for i, it := range items {
		if it.Section == section {
			out = append(out, i)
		}
	}
	return out
}
