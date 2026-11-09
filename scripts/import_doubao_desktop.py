#!/usr/bin/env python3
"""Import the Doubao desktop client session into CLIProxyAPI.

Windows only. Reads the desktop client's Chromium cookie store, decrypts the
v10 values with the DPAPI-protected key from Local State, and writes a
`doubao` auth file that the Doujie OAuth plugin can use.

Requires: Python 3.10+ and `cryptography` (`uv pip install --system
--break-system-packages cryptography`).

Usage:
    python scripts/import_doubao_desktop.py \
        --auth-dir "D:/path/to/cpa/oauth" \
        --user-data "C:/Users/<you>/AppData/Local/Doubao/User Data"
"""

from __future__ import annotations

import argparse
import base64
import ctypes
import ctypes.wintypes
import hashlib
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import time
from pathlib import Path

try:
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
except ImportError:
    sys.exit("missing dependency: uv pip install --system --break-system-packages cryptography")


class DATA_BLOB(ctypes.Structure):
    _fields_ = [("cbData", ctypes.wintypes.DWORD), ("pbData", ctypes.POINTER(ctypes.c_char))]


def unprotect(data: bytes) -> bytes:
    in_blob = DATA_BLOB(
        len(data),
        ctypes.cast(ctypes.create_string_buffer(data, len(data)), ctypes.POINTER(ctypes.c_char)),
    )
    out_blob = DATA_BLOB()
    if not ctypes.windll.crypt32.CryptUnprotectData(
        ctypes.byref(in_blob), None, None, None, None, 0, ctypes.byref(out_blob)
    ):
        raise OSError("CryptUnprotectData failed")
    return ctypes.string_at(out_blob.pbData, out_blob.cbData)


def cookie_plaintext(value: bytes) -> str:
    """v10 payloads carry a short binary prefix before the ASCII cookie value."""
    for start in range(len(value)):
        chunk = value[start:]
        try:
            text = chunk.decode("ascii")
        except UnicodeDecodeError:
            continue
        if all(32 <= ord(char) < 127 for char in text):
            return text
    return ""


def load_cookies(user_data: Path) -> dict[str, str]:
    state = json.loads((user_data / "Local State").read_text(encoding="utf-8"))
    key = unprotect(base64.b64decode(state["os_crypt"]["encrypted_key"])[5:])
    aes = AESGCM(key)

    source = user_data / "Default" / "Network" / "Cookies"
    with tempfile.TemporaryDirectory() as tmp:
        copy = Path(tmp) / "Cookies"
        shutil.copy2(source, copy)
        connection = sqlite3.connect(copy)
        rows = list(
            connection.execute(
                "select name, encrypted_value from cookies "
                "where host_key like '%doubao%' or host_key like '%bytedance%'"
            )
        )

    cookies: dict[str, str] = {}
    for name, encrypted in rows:
        encrypted = bytes(encrypted)
        if encrypted[:3] != b"v10":
            continue
        try:
            plain = aes.decrypt(encrypted[3:15], encrypted[15:], None)
        except Exception:
            continue
        value = cookie_plaintext(plain)
        if value:
            cookies[name] = value
    return cookies


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--auth-dir", required=True, help="CLIProxyAPI oauth/auth directory")
    parser.add_argument(
        "--user-data",
        default=str(Path(os.environ.get("LOCALAPPDATA", "")) / "Doubao" / "User Data"),
        help="Doubao desktop 'User Data' directory",
    )
    parser.add_argument("--label", default="豆姐账号（桌面端导入）")
    args = parser.parse_args()

    cookies = load_cookies(Path(args.user_data))
    if not cookies.get("sessionid"):
        sys.exit("no sessionid cookie found; sign in to the Doubao desktop client first")

    auth_dir = Path(args.auth_dir)
    auth_dir.mkdir(parents=True, exist_ok=True)
    backup_dir = auth_dir.parent / (auth_dir.name + "-backup")
    backup_dir.mkdir(parents=True, exist_ok=True)

    previous = sorted(auth_dir.glob("doubao-*.json"))
    base: dict = {}
    for item in previous:
        try:
            base = json.loads(item.read_text(encoding="utf-8"))
        except Exception:
            base = {}
        shutil.move(str(item), str(backup_dir / item.name))
        print(f"backed up {item.name} -> {backup_dir}")

    digest = hashlib.sha256(cookies["sessionid"].encode()).hexdigest()[:24]
    target = auth_dir / f"doubao-{digest}.json"
    credential = {
        "type": "doubao",
        "email": args.label,
        "cookies": cookies,
        "device_id": base.get("device_id", "7000000000000000"),
        "web_id": base.get("web_id", "7600000000000000000"),
        "fp": base.get("fp", "verify_desktop_import"),
        "login_at": int(time.time()),
    }
    target.write_text(json.dumps(credential, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"wrote {target} with {len(cookies)} cookies")
    print("restart the CPA core to load the imported session")


if __name__ == "__main__":
    main()
