#!/usr/bin/env python3
"""Drives a qemu VM through its QMP socket: types keys, and takes
screenshots. vero's Windows VM has no other way in, so this is how
test-repo.sh --vm windows runs its test there and reads the result.

    qmp.py SOCKET type "text"          types the text, then Enter
    qmp.py SOCKET keys ctrl-alt-delete  sends a key combination
    qmp.py SOCKET shot out.ppm         saves a screenshot
"""
import json
import socket
import sys
import time

KEYS = {" ": "spc", "-": "minus", "=": "equal", ".": "dot", ",": "comma", "/": "slash", "\\": "backslash",
        ";": "semicolon", "'": "apostrophe", "[": "bracket_left", "]": "bracket_right", "`": "grave_accent",
        ":": ("shift", "semicolon"), '"': ("shift", "apostrophe"), "_": ("shift", "minus"), "+": ("shift", "equal"),
        "!": ("shift", "1"), "@": ("shift", "2"), "#": ("shift", "3"), "$": ("shift", "4"), "%": ("shift", "5"),
        "^": ("shift", "6"), "&": ("shift", "7"), "*": ("shift", "8"), "(": ("shift", "9"), ")": ("shift", "0"),
        "<": ("shift", "comma"), ">": ("shift", "dot"), "?": ("shift", "slash"), "|": ("shift", "backslash"),
        "{": ("shift", "bracket_left"), "}": ("shift", "bracket_right"), "~": ("shift", "grave_accent")}


class QMP:
    def __init__(self, path: str) -> None:
        self.s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.s.connect(path)
        self.f = self.s.makefile("rw")
        self.f.readline()  # the greeting
        self.cmd("qmp_capabilities")

    def cmd(self, name: str, **args) -> dict:
        self.f.write(json.dumps({"execute": name, "arguments": args}) + "\n")
        self.f.flush()
        while True:
            reply = json.loads(self.f.readline())
            if "event" not in reply:
                return reply

    def keys(self, *names: str) -> None:
        self.cmd("send-key", keys=[{"type": "qcode", "data": n} for n in names])
        time.sleep(0.05)

    def type(self, text: str) -> None:
        for ch in text:
            if ch.isupper():
                self.keys("shift", ch.lower())
            elif ch in KEYS:
                k = KEYS[ch]
                self.keys(*k) if isinstance(k, tuple) else self.keys(k)
            else:
                self.keys(ch)


def main() -> None:
    q = QMP(sys.argv[1])
    what = sys.argv[2]
    if what == "type":
        q.type(sys.argv[3])
        q.keys("ret")
    elif what == "keys":
        q.keys(*sys.argv[3].split("-"))
    elif what == "shot":
        q.cmd("screendump", filename=sys.argv[3])


if __name__ == "__main__":
    main()
