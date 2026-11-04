#!/usr/bin/env python3
"""Drive the Linux-console test VM started by scripts/linux-console-vm.sh.

Talks to QEMU's monitor socket (keys, screenshots) and the guest serial
console (setup commands). Standard library only.

  vmctl.py serial "<shell command>" [--expect REGEX] [--timeout SEC]
  vmctl.py login                       # log in as root on the serial console
  vmctl.py keys <key> [<key> ...]      # QEMU key names: ctrl-c f2 shift-up a ret ...
  vmctl.py type "<text>"               # type ASCII text on the active VT
  vmctl.py shot <out.png>              # screenshot of the VGA console
  vmctl.py mon "<monitor command>"
"""
import os
import re
import socket
import sys
import time

VM_DIR = os.environ.get("VM_DIR", os.path.join(os.path.dirname(__file__), "..", ".cache", "vm"))
MON = os.path.join(VM_DIR, "mon.sock")
SERIAL = os.path.join(VM_DIR, "serial.sock")

PROMPT = re.compile(rb"[#$] $")


def _connect(path):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect(path)
    return s


def _read_until(s, pattern, timeout):
    s.settimeout(0.2)
    buf = b""
    end = time.time() + timeout
    while time.time() < end:
        try:
            chunk = s.recv(65536)
            if not chunk:
                break
            buf += chunk
            if pattern is not None and re.search(pattern, buf):
                return buf, True
        except socket.timeout:
            pass
    return buf, False


def mon(cmd):
    s = _connect(MON)
    _read_until(s, rb"\(qemu\) ", 3)
    s.sendall(cmd.encode() + b"\n")
    out, _ = _read_until(s, rb"\(qemu\) ", 10)
    s.close()
    text = out.decode(errors="replace")
    text = re.sub(r"\x1b\[[0-9;]*[A-Za-z]", "", text)
    # Drop the monitor's line-editor echo of the command and the trailing prompt.
    _, _, text = text.partition("\r\n")
    return text.replace("(qemu) ", "").strip()


def serial(cmd, expect=PROMPT, timeout=30):
    s = _connect(SERIAL)
    _read_until(s, None, 0.3)  # drain
    s.sendall(cmd.encode() + b"\n")
    out, ok = _read_until(s, expect if isinstance(expect, bytes) else expect, timeout)
    s.close()
    return out.decode(errors="replace"), ok


def login(timeout=180):
    s = _connect(SERIAL)
    end = time.time() + timeout
    buf = b""
    while time.time() < end:
        s.sendall(b"\n")
        out, _ = _read_until(s, rb"(login: |# $)", 3)
        buf += out
        if re.search(rb"# $", buf):
            s.close()
            return True
        if re.search(rb"login: $", buf):
            s.sendall(b"root\n")
            out, ok = _read_until(s, rb"# $", 15)
            s.close()
            return ok
    s.close()
    return False


# Characters that QEMU sendkey needs spelled out.
_KEYNAMES = {
    " ": "spc", "\n": "ret", "\t": "tab", "-": "minus", "=": "equal", "/": "slash",
    ".": "dot", ",": "comma", ";": "semicolon", "'": "apostrophe", "`": "grave_accent",
    "[": "bracket_left", "]": "bracket_right", "\\": "backslash",
}
_SHIFTED = {
    "_": "minus", "+": "equal", "?": "slash", ">": "dot", "<": "comma", ":": "semicolon",
    '"': "apostrophe", "~": "grave_accent", "{": "bracket_left", "}": "bracket_right",
    "|": "backslash", "!": "1", "@": "2", "#": "3", "$": "4", "%": "5", "^": "6",
    "&": "7", "*": "8", "(": "9", ")": "0",
}


def type_text(text):
    for ch in text:
        if ch in _KEYNAMES:
            key = _KEYNAMES[ch]
        elif ch in _SHIFTED:
            key = "shift-" + _SHIFTED[ch]
        elif ch.isupper():
            key = "shift-" + ch.lower()
        else:
            key = ch
        mon("sendkey " + key)
        time.sleep(0.03)


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 2
    cmd, args = argv[1], argv[2:]
    if cmd == "mon":
        print(mon(args[0]))
    elif cmd == "serial":
        expect = PROMPT
        timeout = 30
        if "--expect" in args:
            expect = args[args.index("--expect") + 1].encode()
        if "--timeout" in args:
            timeout = float(args[args.index("--timeout") + 1])
        out, ok = serial(args[0], expect, timeout)
        print(out)
        return 0 if ok else 1
    elif cmd == "login":
        ok = login()
        print("logged in" if ok else "login failed")
        return 0 if ok else 1
    elif cmd == "keys":
        for k in args:
            mon("sendkey " + k)
            time.sleep(0.15)
    elif cmd == "type":
        type_text(args[0])
    elif cmd == "shot":
        out = os.path.abspath(args[0])
        print(mon(f"screendump {out} -f png"))
    else:
        print(__doc__)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
