# Third-Party Notices

tuidesk includes the following third-party Go modules. Full license texts are
distributed with each module's source (see `go mod vendor` output in source
tarballs).

| Module | Version | License | Why |
|---|---|---|---|
| github.com/gdamore/tcell/v2 | v2.13.10 | Apache-2.0 | Terminal I/O: mouse (SGR 1006, drag 1002), bracketed paste, OSC52, wide characters |
| github.com/gdamore/encoding | v1.0.1 | Apache-2.0 | tcell dependency (character set encodings) |
| github.com/mattn/go-runewidth | v0.0.31 | MIT | Display width of Hangul / East Asian characters |
| github.com/rivo/uniseg | v0.4.7 | MIT | tcell/runewidth dependency (grapheme clusters, widths) |
| github.com/clipperhouse/uax29/v2 | v2.2.0 | MIT | runewidth dependency (grapheme segmentation) |
| github.com/lucasb-eyer/go-colorful | v1.3.0 | MIT | tcell dependency (color conversion) |
| golang.org/x/sys | v0.38.0 | BSD-3-Clause | tcell dependency (terminal syscalls) |
| golang.org/x/term | v0.37.0 | BSD-3-Clause | tcell dependency (raw mode) |
| golang.org/x/text | v0.31.0 | BSD-3-Clause | tcell dependency (encodings) |
