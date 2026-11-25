#!/usr/bin/env python3
"""Pack a SealChat StylePackage JSON file into an ImportLogStyle-compatible ZIP archive.

Usage:
    python .agents/skills/log-layout-style-designer/scripts/pack_style_zip.py <package.json> [output.zip]

The generated ZIP matches service.ExportLogStyle / service.ImportLogStyle:
- Root files: manifest.json, tokens.json, settings.schema.json, styles/main.css, templates/<id>.html
- Optional assets declared in package["assets"] resolved relative to <package.json>
- POSIX forward-slash paths and UNIX 0644 regular non-executable file permissions
"""

from __future__ import annotations

import argparse
import json
import sys
import zipfile
from pathlib import Path


def pack_style_zip(src_json: Path, dst_zip: Path | None = None) -> Path:
    src_json = src_json.resolve()
    pkg = json.loads(src_json.read_text(encoding="utf-8"))

    for required in ("schemaVersion", "id", "version", "name", "license", "minRendererVersion", "tokens", "templates", "css", "rules", "settingsSchema", "assets"):
        if required not in pkg:
            raise ValueError(f"missing required field in StylePackage JSON: {required}")

    if dst_zip is None:
        dst_zip = src_json.parent / f'{pkg["id"]}-{pkg["version"]}.zip'
    else:
        dst_zip = dst_zip.resolve()

    manifest = {
        "schemaVersion": pkg["schemaVersion"],
        "id": pkg["id"],
        "version": pkg["version"],
        "name": pkg["name"],
        "license": pkg["license"],
        "minRendererVersion": pkg["minRendererVersion"],
        "tokens": None,
        "templates": None,
        "css": "",
        "rules": pkg["rules"],
        "settingsSchema": None,
        "assets": pkg["assets"],
    }

    entries: dict[str, bytes] = {
        "manifest.json": json.dumps(manifest, ensure_ascii=False, separators=(",", ":")).encode("utf-8"),
        "tokens.json": json.dumps(pkg["tokens"], ensure_ascii=False, separators=(",", ":")).encode("utf-8"),
        "settings.schema.json": json.dumps(pkg["settingsSchema"], ensure_ascii=False, separators=(",", ":")).encode("utf-8"),
        "styles/main.css": str(pkg["css"]).encode("utf-8"),
    }

    for tpl_id, html_src in pkg["templates"].items():
        entries[f"templates/{tpl_id}.html"] = str(html_src).encode("utf-8")

    for asset in pkg["assets"]:
        rel_path = str(asset["path"])
        if not rel_path.startswith("assets/") or "\\" in rel_path or ".." in rel_path.split("/"):
            raise ValueError(f"invalid asset path: {rel_path}")
        asset_file = src_json.parent / rel_path
        if not asset_file.is_file():
            raise FileNotFoundError(f"declared asset not found: {asset_file}")
        entries[rel_path] = asset_file.read_bytes()

    dst_zip.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(dst_zip, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name in sorted(entries):
            info = zipfile.ZipInfo(filename=name, date_time=(2026, 1, 1, 0, 0, 0))
            info.create_system = 3  # UNIX
            info.external_attr = 0o100644 << 16  # regular file, non-executable
            info.compress_type = zipfile.ZIP_DEFLATED
            zf.writestr(info, entries[name])

    return dst_zip


def main() -> int:
    parser = argparse.ArgumentParser(description="Pack a SealChat StylePackage JSON into a valid ZIP.")
    parser.add_argument("source", help="Path to StylePackage JSON file")
    parser.add_argument("output", nargs="?", help="Optional output ZIP path (default: <id>-<version>.zip)")
    args = parser.parse_args()

    try:
        out = pack_style_zip(Path(args.source), Path(args.output) if args.output else None)
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(f"Created: {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
