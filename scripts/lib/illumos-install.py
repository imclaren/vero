"""Install OpenIndiana into a fresh disk, over the serial console.

Called by scripts/run-illumos.sh --install.  Everything here is what the text
installer needs answering, in order, plus the two repairs it leaves behind.

Three things are worth knowing if this ever needs changing:

  * Arrow keys have to be sent as ESC O A / ESC O B.  The usual ESC [ B
    arrives as a literal "B" and is typed into whatever field has focus -
    the first time, it ended up in the hostname.
  * Dialogs are Tab to move to the button, then Enter.
  * The installer's own screen is only redrawn when something changes, so
    ctrl-L is sent before reading it.
"""
import re
import socket
import sys
import time

sock, pubkey = sys.argv[1], sys.argv[2]

s = socket.socket(socket.AF_UNIX)
for _ in range(120):
    try:
        s.connect(sock)
        break
    except OSError:
        time.sleep(1)
else:
    raise SystemExit("qemu never opened its console socket")
s.settimeout(0.5)

F2 = b"\x1b2"


def drain(seconds):
    end, out = time.time() + seconds, b""
    while time.time() < end:
        try:
            out += s.recv(4096)
        except socket.timeout:
            pass
    return out.decode("utf-8", "replace")


def squash(text):
    return " ".join(re.sub(r"\x1b\[[0-9;?]*[A-Za-z]", " ", text).split())


def screen(wait=8):
    s.sendall(b"\x0c")
    time.sleep(2)
    return squash(drain(wait))


def wait_for(text, limit, what):
    end, seen = time.time() + limit, ""
    while time.time() < end:
        seen += drain(5)
        if text in squash(seen):
            return
    raise SystemExit("the installer never showed %s" % what)


def typ(text):
    for ch in text:
        s.sendall(ch.encode())
        time.sleep(0.05)


def run(cmd, wait=60):
    s.sendall(cmd.encode() + b"\n")
    return squash(drain(wait))


# The loader.  The kernel writes to the framebuffer unless the console is
# moved, and then nothing arrives here at all.
seen, end = "", time.time() + 300
while time.time() < end:
    seen += drain(2)
    if "Autoboot in" in seen:
        break
s.sendall(b" ")
time.sleep(1.5)
drain(2)
s.sendall(b"5")                                  # Configure Boot Options
time.sleep(3)
drain(3)
for _ in range(6):
    s.sendall(b"2")                              # cycle Os Console
    time.sleep(2)
    if re.search(r"Os C ?onsole[\. ]*ttya", squash(drain(3))):
        break
else:
    raise SystemExit("could not put the console on ttya")
s.sendall(b"\x08")                               # back
time.sleep(2)
drain(2)
s.sendall(b"1")                                  # boot

print("   booting the installer", flush=True)
wait_for("keyboard layout", 900, "the keyboard prompt")
s.sendall(b"47\n")                               # US-English
wait_for("enter a number", 300, "the language prompt")
s.sendall(b"1\n")                                # English
wait_for("installation menu", 600, "the menu")
s.sendall(b"4\n")                                # terminal type
time.sleep(3)
drain(3)
s.sendall(b"vt100\n")
time.sleep(4)
drain(5)
s.sendall(b"1\n")                                # Install OpenIndiana

print("   answering the installer", flush=True)
wait_for("Welcome to OpenIndiana", 420, "the welcome screen")
time.sleep(3)
s.sendall(F2)

wait_for("Where should OpenIndiana be installed", 300, "the disk screen")
time.sleep(3)
s.sendall(F2)
time.sleep(6)
if "Cancel" in screen(5):                        # "this will destroy the disk"
    s.sendall(b"\t")
    time.sleep(1.5)
    s.sendall(b"\r")
    time.sleep(6)

# MBR, not the default EFI: the whole-disk EFI layout this installer writes is
# not bootable by SeaBIOS or by OVMF.
s.sendall(b"\x1bOB")
time.sleep(2)
s.sendall(F2)
time.sleep(10)
screen(6)                                        # the partition table
s.sendall(F2)
time.sleep(10)

wait_for("Computer Name", 300, "the network screen")
s.sendall(b"\t")                                 # into the radio list
time.sleep(2)
s.sendall(F2)                                    # Automatically
time.sleep(10)

wait_for("Time Zone", 300, "the time zone screen")
time.sleep(2)
s.sendall(F2)                                    # UTC/GMT
time.sleep(10)
s.sendall(F2)                                    # date and time
time.sleep(10)

wait_for("Root password", 300, "the users screen")
time.sleep(3)
typ("veroVero1")
time.sleep(1.5)
s.sendall(b"\t")
time.sleep(1.5)
typ("veroVero1")
time.sleep(1.5)
s.sendall(F2)
time.sleep(10)

wait_for("Esc-2_Install", 300, "the summary")
s.sendall(F2)
print("   installing", flush=True)

last, end = "", time.time() + 7200
while time.time() < end:
    out = screen(10)
    found = re.findall(r"\(\s*(\d+)\s*%\)", out)
    if found and found[-1] != last:
        last = found[-1]
        print("     %s%%" % last, flush=True)
    if "complete" in out.lower() or "reboot" in out.lower():
        break
    time.sleep(45)

# It reboots itself, back into the install media, and what it leaves on the
# disk cannot boot: no bootloader in the MBR and no boot archive.  Both are
# written from here.
print("   repairing the installation", flush=True)
wait_for("Autoboot in", 600, "the loader again")
s.sendall(b" ")
time.sleep(1.5)
drain(2)
s.sendall(b"5")
time.sleep(3)
drain(3)
for _ in range(6):
    s.sendall(b"2")
    time.sleep(2)
    if re.search(r"Os C ?onsole[\. ]*ttya", squash(drain(3))):
        break
s.sendall(b"\x08")
time.sleep(2)
drain(2)
s.sendall(b"1")
wait_for("keyboard layout", 900, "the keyboard prompt")
s.sendall(b"47\n")
wait_for("enter a number", 300, "the language prompt")
s.sendall(b"1\n")
wait_for("installation menu", 600, "the menu")
s.sendall(b"3\n")                                # Shell
time.sleep(12)
drain(5)

run("zpool import -f -N rpool", 120)
run("beadm mount openindiana /zz", 120)
run("bootadm install-bootloader -M -f -P rpool", 180)
run("bootadm update-archive -f -R /zz", 600)
for _ in range(30):                              # the archive takes a while
    if "No such" not in run("ls /zz/platform/i86pc/amd64/boot_archive", 40):
        break
    time.sleep(30)
else:
    raise SystemExit("the boot archive was never written")

# ssh in rather than logging in: the password set during the install does not
# take, and illumos asks for it even in single user mode.
run("mkdir -p /zz/root/.ssh", 30)
run("echo '%s' > /zz/root/.ssh/authorized_keys" % pubkey, 30)
run("chmod 700 /zz/root/.ssh; chmod 600 /zz/root/.ssh/authorized_keys", 30)
run("sed 's/^PermitRootLogin no/PermitRootLogin without-password/' "
    "/zz/etc/ssh/sshd_config > /tmp/sc && cp /tmp/sc /zz/etc/ssh/sshd_config", 60)
run("beadm umount openindiana", 120)
run("zpool export rpool", 180)
print("   done", flush=True)
