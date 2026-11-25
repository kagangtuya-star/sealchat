#!/usr/bin/env python3
"""把设计页（如对话框底图）用无头 Chrome 渲染成透明底 PNG，并校验透明度。

用法：
  python3 export_png.py <html> <out.png> [--size 1920x336] [--query "theme=school&frame=board"] [--wait 4000]

· 页面以 ?mode=export 打开（设计页据此隐藏参考线、示例文字与棋盘格），附加的 --query 原样拼接。
· 视口 = --size，设备像素比 1，背景色 00000000 → 页面未绘制的区域保持透明。
· --wait 为虚拟时间预算（毫秒），给 Web 字体与 SVG 滤镜留出加载时间。
· Chrome 路径可用环境变量 CHROME 指定；否则依次查找 google-chrome / chromium / msedge 等。
· 只用标准库：导出后解码 PNG，确认是 RGBA、四角透明，并报告透明像素占比。
"""
import argparse
import os
import pathlib
import shutil
import struct
import subprocess
import sys
import tempfile
import zlib

CANDIDATES = [
    "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge", "microsoft-edge",
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    r"C:\Program Files\Google\Chrome\Application\chrome.exe",
    r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe",
]


def find_chrome():
    env = os.environ.get("CHROME")
    if env:
        return env
    for c in CANDIDATES:
        p = shutil.which(c) or (c if os.path.isfile(c) else None)
        if p:
            return p
    sys.exit("找不到 Chrome / Chromium / Edge，请用环境变量 CHROME 指定可执行文件路径")


def decode_png(data):
    """返回 (宽, 高, 颜色类型, RGBA 行列表或 None)。只完整解码 8 位 RGBA。"""
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("不是 PNG 文件")
    pos, idat, w, h = 8, bytearray(), 0, 0
    ctype = depth = None
    while pos < len(data):
        length, kind = struct.unpack(">I4s", data[pos:pos + 8])
        chunk = data[pos + 8:pos + 8 + length]
        if kind == b"IHDR":
            w, h, depth, ctype = struct.unpack(">IIBB", chunk[:10])
        elif kind == b"IDAT":
            idat += chunk
        pos += 12 + length
    if ctype != 6 or depth != 8:
        return w, h, ctype, None
    raw, stride, rows, prev = zlib.decompress(bytes(idat)), w * 4, [], bytearray(w * 4)
    for y in range(h):
        f = raw[y * (stride + 1)]
        line = bytearray(raw[y * (stride + 1) + 1:(y + 1) * (stride + 1)])
        for i in range(stride):
            a = line[i - 4] if i >= 4 else 0
            b = prev[i]
            c = prev[i - 4] if i >= 4 else 0
            if f == 1:
                line[i] = (line[i] + a) & 255
            elif f == 2:
                line[i] = (line[i] + b) & 255
            elif f == 3:
                line[i] = (line[i] + ((a + b) >> 1)) & 255
            elif f == 4:
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                line[i] = (line[i] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        rows.append(line)
        prev = line
    return w, h, ctype, rows


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("html")
    ap.add_argument("out")
    ap.add_argument("--size", default="1920x336", help="输出像素尺寸 宽x高（默认 1920x336 = SealChat 默认对话框比例）")
    ap.add_argument("--query", default="", help="附加的 URL 参数，如 theme=school&frame=board")
    ap.add_argument("--wait", type=int, default=4000, help="虚拟时间预算（毫秒）")
    args = ap.parse_args()

    w, h = (int(v) for v in args.size.lower().split("x"))
    src = pathlib.Path(args.html).resolve()
    out = pathlib.Path(args.out).resolve()
    out.parent.mkdir(parents=True, exist_ok=True)
    query = "mode=export" + (f"&w={w}&h={h}") + (f"&{args.query}" if args.query else "")
    url = f"{src.as_uri()}?{query}"

    with tempfile.TemporaryDirectory() as profile:
        cmd = [find_chrome(), "--headless=new", "--disable-gpu", "--hide-scrollbars", "--no-first-run",
               "--no-default-browser-check", f"--user-data-dir={profile}", "--force-device-scale-factor=1",
               "--default-background-color=00000000", f"--window-size={w},{h}",
               f"--virtual-time-budget={args.wait}", f"--screenshot={out}", url]
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    if res.returncode != 0 or not out.exists():
        sys.exit(f"Chrome 渲染失败（退出码 {res.returncode}）：\n{res.stderr[-2000:]}")

    pw, ph, ctype, rows = decode_png(out.read_bytes())
    print(f"{out}  {pw}×{ph}  {out.stat().st_size // 1024} KB")
    ok = True
    if (pw, ph) != (w, h):
        print(f"  WARN   实际尺寸 {pw}×{ph} 与请求 {w}×{h} 不一致（检查 Chrome 版本的 --window-size 行为）")
        ok = False
    if rows is None:
        print(f"  ERROR  不是 8 位 RGBA（颜色类型 {ctype}），没有透明通道")
        sys.exit(1)
    alpha = [r[i] for r in rows for i in range(3, len(r), 4)]
    clear = sum(1 for a in alpha if a == 0)
    corners = [rows[0][3], rows[0][-1], rows[-1][3], rows[-1][-1]]
    print(f"  info   透明像素 {clear * 100 / len(alpha):.1f}% · 半透明 {sum(1 for a in alpha if 0 < a < 255) * 100 / len(alpha):.1f}% · 四角 alpha {corners}")
    if clear == 0:
        print("  ERROR  没有任何透明像素：页面 html/body 是否设置了不透明背景？")
        sys.exit(1)
    if any(corners):
        print("  WARN   有角落不透明：如果设计本身铺满画布可以忽略")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
