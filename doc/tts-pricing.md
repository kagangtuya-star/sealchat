# TTS 模型目录与定价

模型能力和内置价格定义在 `pkg/ttsprovider/models.go`。管理员可通过
`GET /api/v1/tts/admin/models` 获取支持的模型，通过原有
`POST /api/v1/tts/admin/provider/resolve` 校验百炼业务空间和密钥、导入在线目录。
只接纳能力目录中支持当前 HTTP/SSE 合成协议的型号。

## 官方价格快照（北京，2026-09-30 核对）

| 模型 | 计费模式 | 原价 |
| --- | --- | --- |
| qwen-audio-3.0-tts-flash | character | 1 元 / 万字符 |
| qwen-audio-3.0-tts-plus | character | 1.4 元 / 万字符 |
| qwen-audio-3.1-tts-flash | token | 输入 1.5 元 / 百万 Token，输出 12 元 / 百万 Token |

价格以单字符、单输入 Token、单输出 Token 或单次请求归一化。百炼 `/api/v1/models` 可确认的在线价格优先；对于 Token 计费模型，在线目录缺失的输入/输出价格可继续复用 LLM 已有的 `models.dev` 价格目录和 12 小时缓存进行补全，仍缺失时再回退到内置官方价格快照。字符计费模型不使用 `models.dev` 补价。

实际优先级为“百炼在线价格 → `models.dev` 可确认价格 → 内置价格快照 → `unknown`”。`pricingSource` 为 `online`、`modelsdev`、`builtin`、`mixed` 或 `unknown`；其中 `mixed` 表示价格字段来自多个来源补全。`displayPrice` 仅用于显示；无法确认的价格保持 `null`，显式 `0` 是合法价格。

百炼模型目录的网络或鉴权错误不会被后续价格来源掩盖，也不会据此宣称密钥已验证；`models.dev` 查询失败或无法精确匹配 provider/model 时仅跳过该来源，继续使用内置价格快照。

## 配置与最终结算

新增 `pricingMode`、`inputTokenPrice`、`outputTokenPrice`，均兼容 JSON/YAML。
旧配置不需要迁移，缺少 `pricingMode` 时仍按 `character` 处理，
`characterPrice` 继续表示单字符成本。选择 3.1 时需采用 `token` 模式。

提交时仍使用字符数做额度预留估算；Token 模式的预留单价为输入与输出单价之和。
这不是实际 Token 用量。最终字符账单沿用供应商 `usage.characters`；
Token 账单只使用成功终止事件确认的 `usage.input_tokens` 和 `usage.output_tokens`，
按输入费用与输出费用之和结算（保留原有六位小数精度）。
供应商没有可确认的 Token 用量时，任务进入 `usage_unknown`，保留预留，
不以字符换算 Token、不重新提交收费请求。管理员核对时需分别提供真实输入和输出 Token 数，
原有字符/创建次数核对接口仍兼容。

Token 用量和实际费用以新增可空字段保存，现有任务通过既有 AutoMigrate 添加列；
旧任务的 `actualUnits` 与字符账单不变。Token 日志复用现有输入/输出 Token 及费用字段，
不把预留估算单价冒充最终单价。重复结算继续幂等。

3.1 与 3.0 复用同一个 `ttsprovider.Client`、同一个 `SpeechSynthesizer` endpoint
和 HTTP/SSE 请求结构。现有超长文本分段、WebSocket 合成分支、播放队列和并发架构不变；
该分支没有可确认的 Token 用量时同样暂停结算。
3.1 的 68 个官方系统音色按 `targetModel` 严格隔离，不修改用户自定义音色结构。

## 官方依据

- [3.1 Flash 模型及价格](https://help.aliyun.com/zh/model-studio/qwen-audio-3-1-tts-flash)
- [3.0 Flash 模型及价格](https://help.aliyun.com/zh/model-studio/qwen-audio-3-0-tts-flash)
- [3.0 Plus 模型及价格](https://help.aliyun.com/zh/model-studio/qwen-audio-3-0-tts-plus)
- [模型与 HTTP/WebSocket 支持](https://help.aliyun.com/zh/model-studio/tts-model)
- [HTTP/SSE 接口](https://help.aliyun.com/zh/model-studio/cosyvoice-tts-http-api)
- [系统音色（区分大小写）](https://help.aliyun.com/zh/model-studio/qwen-audio-tts-voice-list)

HTTP 参考页目前仍以字符用量示例为主。客户端兼容 SDK 通用用量结构中的
`input_tokens` / `output_tokens`，但不推断未返回的计费字段。
