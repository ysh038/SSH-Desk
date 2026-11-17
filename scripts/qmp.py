#!/usr/bin/env python3
"""Plug an image file into a QEMU VM as a USB drive, or pull it out (QMP).

  qmp.py SOCKET plug IMAGE     attach IMAGE as a removable USB storage device
  qmp.py SOCKET unplug         detach it (like pulling the stick out)
  qmp.py SOCKET status         "plugged" or "unplugged"

Standard library only. Used by scripts/usb-vm.sh.
"""
import json
import socket
import sys
import time

DEV = "tuidesk-usb"
NODE = "tuidesk-usb-img"


class QMP:
    def __init__(self, path):
        self.s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.s.settimeout(10)
        self.s.connect(path)
        self.f = self.s.makefile("rw")
        self._read()  # greeting
        self.cmd("qmp_capabilities")

    def _read(self):
        line = self.f.readline()
        if not line:
            raise SystemExit("error: QEMU closed the connection")
        return json.loads(line)

    def cmd(self, name, **args):
        """Run a command; returns (result, error message or None)."""
        msg = {"execute": name}
        if args:
            msg["arguments"] = args
        self.f.write(json.dumps(msg) + "\n")
        self.f.flush()
        while True:
            r = self._read()
            if "return" in r:
                return r["return"], None
            if "error" in r:
                return None, r["error"].get("desc", "error")
            # anything else is an asynchronous event

    def plugged(self):
        _, err = self.cmd("qom-get", path="/machine/peripheral/" + DEV, property="type")
        return err is None


def main(argv):
    if len(argv) < 3 or argv[2] not in ("plug", "unplug", "status"):
        sys.exit(__doc__)
    q = QMP(argv[1])
    if argv[2] == "status":
        print("plugged" if q.plugged() else "unplugged")
    elif argv[2] == "plug":
        if len(argv) != 4:
            sys.exit(__doc__)
        if q.plugged():
            sys.exit("error: a USB drive is already plugged in (unplug it first)")
        q.cmd("blockdev-del", **{"node-name": NODE})  # left over from a crash
        _, err = q.cmd("blockdev-add", **{"driver": "raw", "node-name": NODE,
                                         "file": {"driver": "file", "filename": argv[3]}})
        if err:
            sys.exit("error: cannot open the image: " + err)
        _, err = q.cmd("device_add", driver="usb-storage", id=DEV, drive=NODE, removable=True)
        if err:
            q.cmd("blockdev-del", **{"node-name": NODE})
            sys.exit("error: cannot plug in the USB drive: " + err)
        print("plugged")
    else:
        if not q.plugged():
            print("unplugged")
            return
        _, err = q.cmd("device_del", id=DEV)
        if err:
            sys.exit("error: cannot unplug: " + err)
        for _ in range(50):
            if not q.plugged():
                break
            time.sleep(0.1)
        else:
            sys.exit("error: the guest did not release the USB drive")
        q.cmd("blockdev-del", **{"node-name": NODE})
        print("unplugged")


if __name__ == "__main__":
    main(sys.argv)
