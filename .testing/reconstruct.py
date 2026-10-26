"""Reconstruct the reviewed TEST2 source delta. Never writes outside src/tests."""
import base64
import hashlib
import json
import lzma
import subprocess
from pathlib import Path, PurePosixPath

EXPECTED = "0837d978727a4d69f9b6936a6e8a6177460a5a4a0804f6989fd25381ffe405e8"
root = Path(__file__).resolve().parents[1]
parts = sorted((root / ".testing/updater-source").glob("part*.b64"))
assert [p.name for p in parts] == [f"part{i:02}.b64" for i in range(1, 11)], "Missing source part"
encoded = "".join(p.read_text(encoding="utf-8").strip() for p in parts)
packed = base64.b64decode(encoded, validate=True)
raw = lzma.decompress(packed, memlimit=256 * 1024 * 1024)
assert len(raw) < 2_000_000 and hashlib.sha256(raw).hexdigest() == EXPECTED, "Source checksum mismatch"
entries = json.loads(raw)
assert len(entries) == 48
seen = set()
prepared = []
for item in entries:
    rel = PurePosixPath(item["path"])
    assert not rel.is_absolute() and ".." not in rel.parts and "\\" not in str(rel)
    assert rel.parts[0] in {"src", "tests"} and str(rel) not in seen
    seen.add(str(rel))
    path = root.joinpath(*rel.parts)
    assert not path.is_symlink() and root in path.resolve().parents
    # Git blobs are canonical bytes; Windows checkout may convert LF to CRLF.
    blob = subprocess.run(["git", "show", "HEAD:" + str(rel)], cwd=root, capture_output=True, check=False)
    old = blob.stdout if blob.returncode == 0 else b""
    assert hashlib.sha256(old).hexdigest() == item["before"], f"Wrong source base: {rel}"
    if "base64" in item:
        assert str(rel).startswith("src/browser_extension/icon") and rel.suffix == ".png"
        new = base64.b64decode(item["base64"], validate=True)
        assert new.startswith(b"\x89PNG\r\n\x1a\n")
    else:
        lines = old.decode("utf-8").splitlines(keepends=True)
        end = len(lines)
        for i, j, text in reversed(item["edits"]):
            assert 0 <= i <= j <= end and isinstance(text, str)
            lines[i:j] = [text]
            end = i
        new = "".join(lines).encode("utf-8")
    assert hashlib.sha256(new).hexdigest() == item["after"], f"Reconstruction mismatch: {rel}"
    prepared.append((path, new))
fixture = subprocess.check_output(["git", "show", "HEAD:.testing/native_updater_test.go.txt"], cwd=root)
assert hashlib.sha256(fixture).hexdigest() == "715846c42c4c27cee0df8a5f0e5e814a22ecf60ad0d936d576cc3190b79bc6c8"
for path, content in prepared:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(content)
(root / "src/updater_windows_e2e_test.go").write_bytes(fixture)
print(f"Verified and reconstructed {len(prepared)} TEST2 source files and native test fix. Stable branch is untouched.")
