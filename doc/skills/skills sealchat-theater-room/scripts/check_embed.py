#!/usr/bin/env python3
"""检查组件 HTML 是否满足 SealChat 小剧场 iForm 嵌入约束。

用法：python3 check_embed.py <html 或目录> [...]
检查项：
  · 嵌入代码 ≤ 88192 字节（SealChat 服务端硬上限，按 UTF-8 字节计；中文 3 字节/字）
  · 禁用 alert / confirm / prompt（频道 iframe 处于 sandbox）
  · localStorage / sessionStorage 必须包在 try 中（srcdoc 无 allow-same-origin，访问即抛错）
  · 外部脚本只允许宿主注入的 SDK（运行时动态加载），不得 <script src> 其他地址
  · html/body 背景应为透明，并声明 color-scheme（否则浏览器可能给 iframe 垫底色）
  · 外部字体样式表非阻塞加载
  · <sealstage-runtime> 块与 templates/runtime.js 一致
退出码：有 ERROR 时为 1。
"""
import pathlib
import re
import sys

LIMIT = 88192
HERE = pathlib.Path(__file__).resolve().parent
RUNTIME = (HERE.parent / "templates" / "runtime.js").read_text(encoding="utf-8").strip()
BLOCK = re.compile(r"/\* <sealstage-runtime.*?/\* </sealstage-runtime> \*/", re.S)


def check(path):
    raw = path.read_bytes()
    text = raw.decode("utf-8")
    errors, warns, infos = [], [], []
    size = len(text.strip().encode("utf-8"))
    pct = size * 100 // LIMIT
    if size > LIMIT:
        errors.append(f"体积 {size} B 超过上限 {LIMIT} B（{pct}%）——拆分组件或改用外链 URL iForm")
    elif size > LIMIT * 0.9:
        warns.append(f"体积 {size} B，已达上限 {pct}%")
    else:
        infos.append(f"体积 {size} B（{pct}%）")

    code = BLOCK.sub("", text)
    if re.search(r"\b(alert|confirm|prompt)\s*\(", code):
        errors.append("使用了 alert/confirm/prompt，sandbox 中不可用，改用页面内 DOM 对话框")
    for m in re.finditer(r"\b(localStorage|sessionStorage)\b", code):
        before = code[max(0, m.start() - 160):m.start()]
        if "try" not in before:
            warns.append(f"{m.group(1)} 可能未被 try 包裹（srcdoc 中访问会抛 SecurityError）")
            break
    for m in re.finditer(r"<script[^>]*\bsrc=[\"']([^\"']+)", text):
        errors.append(f"外部脚本 {m.group(1)}：嵌入代码只应动态加载宿主提供的 sdkUrl")
    for m in re.finditer(r"<link[^>]+rel=[\"']stylesheet[\"'][^>]*>", text):
        if "media=" not in m.group(0):
            warns.append("外部样式表会阻塞渲染（字体源不可达时组件长时间空白），改用 media=\"print\" onload=\"this.media='all'\"")
            break
    if 'name="color-scheme"' not in text:
        warns.append('缺少 <meta name="color-scheme" content="light dark">：宿主与 iframe 配色方案不一致时浏览器会垫不透明底色')
    if not re.search(r"(html|body)[^{]*\{[^}]*background:\s*transparent", text):
        warns.append("未声明 html/body 背景透明；如果是叠加层会遮挡舞台")

    blocks = BLOCK.findall(text)
    if blocks:
        if any(b.strip() != RUNTIME for b in blocks):
            warns.append("sealstage-runtime 块与 templates/runtime.js 不一致，运行 sync_runtime.py")
        infos.append("使用 SealStage 运行时：iForm 需启用 Embed API，并授予 storage.read/write、events.subscribe/publish、permissions.read")
    return errors, warns, infos


def main(argv):
    if not argv:
        print(__doc__)
        return 2
    failed = False
    for arg in argv:
        p = pathlib.Path(arg)
        # 目录扫描时跳过 index.html（本地预览页，不是组件）
        files = sorted(f for f in p.rglob("*.html") if f.name != "index.html") if p.is_dir() else [p]
        for f in files:
            errors, warns, infos = check(f)
            failed |= bool(errors)
            print(f"\n{f}")
            for e in errors:
                print(f"  ERROR  {e}")
            for w in warns:
                print(f"  WARN   {w}")
            for i in infos:
                print(f"  info   {i}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
