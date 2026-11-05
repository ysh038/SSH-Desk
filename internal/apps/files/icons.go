package files

import (
	"path/filepath"
	"strings"

	"tuidesk/internal/fsops"
)

// Icon view cell size: an icon line and a name line, cellW columns wide
// (SPEC 7.2: text icons by default so the Linux console can draw them).
const (
	cellW = 14
	cellH = 2
)

var iconExt = map[string]string{
	".txt": "TXT", ".md": "TXT", ".csv": "TXT", ".rst": "TXT",
	".conf": "CFG", ".cfg": "CFG", ".ini": "CFG", ".toml": "CFG", ".yaml": "CFG", ".yml": "CFG",
	".json": "CFG", ".xml": "CFG", ".env": "CFG", ".service": "CFG", ".timer": "CFG", ".properties": "CFG",
	".sh": "SH", ".bash": "SH", ".py": "PY", ".go": "GO", ".js": "JS", ".ts": "JS", ".java": "SRC",
	".c": "SRC", ".h": "SRC", ".cpp": "SRC", ".rs": "SRC", ".rb": "SRC", ".php": "SRC", ".sql": "SQL",
	".log": "LOG", ".out": "LOG", ".err": "LOG",
	".tar": "ZIP", ".gz": "ZIP", ".tgz": "ZIP", ".zip": "ZIP", ".bz2": "ZIP", ".xz": "ZIP", ".zst": "ZIP", ".7z": "ZIP",
	".deb": "PKG", ".rpm": "PKG", ".jar": "PKG", ".whl": "PKG",
	".pem": "KEY", ".crt": "KEY", ".key": "KEY", ".pub": "KEY", ".cer": "KEY",
	".png": "IMG", ".jpg": "IMG", ".jpeg": "IMG", ".gif": "IMG", ".svg": "IMG", ".ico": "IMG",
	".pdf": "DOC", ".doc": "DOC", ".docx": "DOC", ".xls": "DOC", ".xlsx": "DOC",
}

// IconText is the text icon for a file: "[DIR]", "[TXT]", "[EXE]", …
// It is always 5 columns wide.
func IconText(name string, isDir, isLink bool, mode uint32) string {
	kind := "---"
	switch {
	case isLink:
		kind = "LNK"
	case isDir:
		kind = "DIR"
	default:
		ext := strings.ToLower(filepath.Ext(name))
		if k, ok := iconExt[ext]; ok {
			kind = k
		} else if strings.HasSuffix(strings.ToLower(name), ".log.1") || strings.Contains(name, ".log.") {
			kind = "LOG"
		} else if mode&0o111 != 0 {
			kind = "EXE"
		} else if ext == "" || strings.HasPrefix(name, ".") {
			kind = "TXT" // README, Makefile, .bashrc: usually text
		}
	}
	for len(kind) < 3 {
		kind += " "
	}
	return "[" + kind + "]"
}

func entryIcon(e fsops.Entry) string {
	return IconText(e.Name, e.IsDir, e.IsLink, uint32(e.Mode.Perm()))
}
