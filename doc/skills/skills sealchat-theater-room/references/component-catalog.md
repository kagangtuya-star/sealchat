# 组件目录（templates/）

所有模板都是单文件 HTML，主题 `ink | taisho | cosmic | washi | classic | school`；本地预览支持 URL 参数覆盖。

> 模板的主题、边框、母题、粒子预设是**快速草案用的参数**，不是风格范围。正式房间取模板的工程骨架（缩放、运行时、同步、角色切换、导出），视觉按 `design-essence.md` 第 3 节立项的方向重写或扩写。

## runtime.js · SealStage 运行时

不是独立组件，由 `scripts/sync_runtime.py` 注入到需要同步的组件。

| API | 说明 |
| --- | --- |
| `Stage.ready` | 连接完成或回落后 resolve |
| `Stage.mode` | `sealchat`（已同步）· `local`（本地预览：BroadcastChannel + localStorage）· `unsynced`（在 SealChat 里但没启用 Embed API） |
| `Stage.gm` | World Owner/Admin 为 true；本地预览为 true，`?gm=0` 模拟玩家 |
| `Stage.get(k)` / `Stage.set(k, v)` | 持久状态（Embed Storage），乐观更新 |
| `Stage.onState(fn)` | `fn(key, value)`；订阅时立即回放当前全部状态 |
| `Stage.emit(topic, p)` / `Stage.on(topic, fn)` | 瞬时事件（Embed Events），本地立即触发、远端回声去重 |
| `Stage.onGM(fn)` | 权限变化 |
| `STAGE_NS` | 组件内声明的命名空间（本地预览隔离用；SealChat 中按 iForm 自动隔离） |

## frame.html · 舞台框（主背景）

- `CONFIG.frame`：`scroll` 卷轴 · `ticket` 车票（齿孔、剪线、编号）· `screen` 屏幕（双线、角括号、吊星）· `lattice` 花窗（回纹角）
- `CONFIG.motifs`：`waves` 青海波 · `clouds` 霞云 · `orbit` 星盘 · `stars` 星点 · `moonring` 月相环 · `checker` 市松 · `ribbon` 弧形缎带
- `title / seal / subtitle / meta / latin / ribbon`
- 预览：`?theme=&frame=&motifs=a,b&title=&seal=&sub=&ribbon=`

## opening.html · 开场揭幕

时间轴：墨晕 0.1s → 笔刷线 0.4s → 纹章 1.2s → 逐字书写 2.1s 起每字 0.38s → 落印 + 震屏 → 副标题 / 信息 / 诗句。
- `title / vertical / seal / subtitle / ringText / poem[] / meta[[k,v]] / rule / end('hold'|'fade') / fadeAfter`
- 点击重播；放进独立小剧场场景实现"切场即播"。

## ambient.html · 氛围层

- `preset`：`sakura plum snow fireflies stars embers leaves ginkgo dust dragonflies crows rain bubbles moondust`，可用 `+` 组合
- `density speed wind reverse colors avoidCenter`
- 透明 canvas，按舞台面积自适应数量，`reverse: true` 做"时间倒流"。

## dossier.html · 资料夹

`DATA.tabs[]`，每页：
- `type: 'table'`：`dice: '1D6'|'2D6'`、`rows: [[n, html]]`、`note`。掷骰结果经 Events 同步，所有人高亮同一行。
- `type: 'cards'`：`cards: [{ tag, big, title, body, back, reveal, sealed, sealedText }]`。`back` 点击翻面；`reveal` 由 GM 点「公开」后才显示（Storage）；`sealed: true` 公开前整张显示为 ？？？。
- `type: 'text'`：`html`（可用 `.kv` 定义表）。
- `color`：吊签单独配色。

## stage-core.html · 演出核心

同一个 iForm 两种角色（宽 < 800 控制台 / 铺满特效层）。
- `acts[{ name, sub }]` → 幕名缎带横幅
- `counter{ label, max }` → 计数印章（巡 / 轮 / 日）
- `cards[{ id, label, title, text }]` → 全屏字卡
- `flashes[{ id, label, color }]` → 闪光 + 震屏（color 用 6 位十六进制）
- Storage key `core`，Events topic `core.fx`。

## dialogue-box.html · 对话框底图（设计源，导出 PNG）

不是 iForm 组件：在浏览器里调好，用 `scripts/export_png.py` 导出透明 PNG，上传到「小剧场外观 → 对话框 → 对话框图片」。规格见 `dialogue-box.md`。
- `CONFIG.frame`：`scroll` 卷轴（左右卷轴杆）· `ticket` 车票（齿孔、半圆缺口、票号）· `screen` 屏幕（角括号、吊星）· `lattice` 花窗（回纹角）· `board` 黑板（木框、粉笔槽）
- `CONFIG.theme`：同 `frame.html`；`seal` 上沿印章字；`serial` 票号；`width / height` 画布（默认 1920×336）
- 模式：默认设计台（参考线、示例文字、设置建议）· `?mode=export` 只画底图 · `?mode=stage` 铺满视口并按 SealChat 排版叠示例文字
- 预览：`?theme=&frame=&seal=&w=&h=`

## 进阶实例（rooms/paradise/）

| 文件 | 可借鉴的技巧 |
| --- | --- |
| `01-opening.html` | 多段分镜（烛光卷轴 → 剪影消散 → 钟声涟漪 → 黄昏 → 标题）；`#stage *` 先暂停、挂载后统一开跑 |
| `02-stage-day.html` | 程序化校舍 / 彩旗悬链线 / 短册摇摆 / 会"回跳"的钟 |
| `03-stage-night.html` | 同构图夜版：破碎主窗、环绕轨道上的日历页、断线 |
| `04-timeloop.html` | 模组专属叙事 FX：倒转大钟 + 倒数、红线断裂 + 彼岸花、谜团翻牌、玻璃碎裂、八岐、结局常驻卡；「正」字计数；逐巡递增的不安感 |
| `07-dialogue-box.html` | 专属对话框底图：主窗黑板缩成横向小黑板，名札压上框、上沿挂彩旗、红线从两侧出血；附舞台合成预览 |
