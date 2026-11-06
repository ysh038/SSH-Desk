tuidesk @VERSION@ (linux-@ARCH@)
================================

A desktop (windows, file manager, editor, terminal) inside a text terminal,
for Linux servers you reach over ssh. One static binary: no libraries, no
network access, no root needed.

Contents
  tuidesk                 the program
  install.sh              installer
  SHA256SUMS              checksums of the files in this folder
  examples/apps/          example launcher entries (htop, lazygit, ...)
  LICENSE                 MIT license
  THIRD_PARTY_NOTICES.md  licenses of the bundled Go modules

Install
  tar xzf tuidesk-@VERSION@-linux-@ARCH@.tar.gz
  cd tuidesk-@VERSION@-linux-@ARCH@
  ./install.sh                  # just you: ~/.local/bin/tuidesk
  sudo ./install.sh --system    # everyone: /usr/local/bin/tuidesk

  install.sh checks the machine type and SHA256SUMS first. To check the
  archive itself, compare it with the SHA256SUMS file published next to it:
    sha256sum -c SHA256SUMS --ignore-missing

Run
  tuidesk
  Quit with Ctrl+Q. F9 opens the menu, F1 shows help.

  Works best from macOS Terminal.app over ssh (mouse supported) and on the
  Linux console (keyboard only). The terminal must use UTF-8.

Settings
  ~/.config/tuidesk/config.toml   written on first run (keyboard question)
  ~/.config/tuidesk/apps/         launcher entries (*.toml), see examples/apps
  ~/.config/tuidesk/snippets/     your own snippets (*.toml)
  Updating or uninstalling never changes these files.

Update
  Unpack the new version and run its install.sh the same way.

Uninstall
  ./install.sh --uninstall            (or: sudo ./install.sh --uninstall --system)
