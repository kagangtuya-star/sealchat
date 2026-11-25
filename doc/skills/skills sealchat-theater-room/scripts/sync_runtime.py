#!/usr/bin/env python3
"""把 templates/runtime.js 同步进各组件 HTML 中的 <sealstage-runtime> 标记块。

用法：python3 sync_runtime.py <html 或目录> [...]
标记块格式（占位写法即可，脚本会整体替换）：
    /* <sealstage-runtime> ... */
    /* </sealstage-runtime> */
"""
import pathlib
import re
import sys

HERE = pathlib.Path(__file__).resolve().parent
RUNTIME = (HERE.parent / "templates" / "runtime.js").read_text(encoding="utf-8").strip()
BLOCK = re.compile(r"/\* <sealstage-runtime.*?/\* </sealstage-runtime> \*/", re.S)


def targets(args):
    for arg in args:
        p = pathlib.Path(arg)
        if p.is_dir():
            yield from sorted(p.rglob("*.html"))
        else:
            yield p


def main(argv):
    if not argv:
        print(__doc__)
        return 2
    for path in targets(argv):
        text = path.read_text(encoding="utf-8")
        if not BLOCK.search(text):
            continue
        new = BLOCK.sub(lambda _: RUNTIME, text)
        if new != text:
            path.write_text(new, encoding="utf-8")
            print(f"synced  {path}")
        else:
            print(f"ok      {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
