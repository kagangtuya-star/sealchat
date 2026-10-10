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
3.1 的 68 个官方系统音色按 `providerKind + models[]` 严格隔离，`targetModel` 仅为单模型音色的 API 兼容派生字段；不修改用户自定义音色结构。详见 [能力目录](tts-catalog.md)。

## 世界访问与额度

`speech.worldAccessMode` 接受 `all` / `whitelist`，历史配置和空值默认为
`all`。`whitelist` 模式只允许 `TTSWorldPolicy.Allowlisted=true` 的世界。
`speech.worldActivationCode` 留空时关闭自助激活，仅管理员配置接口可读取此值。

`GET /api/v1/tts/me` 保持全局个人语音语义；可选 `channelId` 由服务端解析真实
世界，返回的 `enabled` 为当前世界 capability，另附 `worldAccess` 的
`worldId`、`allowed`、`reason`、`canActivate`、`policy` 和 `usage`。
额度耗尽在预留时拒绝，capability 不因余额耗尽隐藏。前端统一使用 speech store，
频道切换刷新 capability，未加入白名单时隐藏语音入口。

世界主或世界管理员可调用 `POST /api/v1/tts/worlds/:worldId/activate`，请求体为
`{"code":"激活码"}`。成功仅更新白名单及激活人/时间，保留已设置额度。
平台管理员使用以下接口管理现有世界：

- `GET /api/v1/tts/admin/worlds?page=1&pageSize=20&search=世界名或ID`
- `GET /api/v1/tts/admin/worlds/:worldId`
- `PATCH /api/v1/tts/admin/worlds/:worldId`，仅接受 `allowlisted`、
  `quotaOverrideEnabled`、`dailyLimit`、`monthlyLimit`、`lifetimeLimit`。
  额度 `null` 表示不限，省略字段保持原值。
- `GET /api/v1/admin/ai/usage-logs?quotaKind=speech&worldId=...`，可叠加
  `providerId`、`model`、`featureKey` 明细过滤。

独立世界额度是用户额度之外的第二层约束；关闭 `quotaOverrideEnabled` 表示
无额外世界限额。金额沿用 `TotalCost`，日/月/累计消费读取 speech ledger，
加上同世界全部 active reservation（包括等待核对的预留）。预留和结算固定按
用户、世界顺序取得数据库写锁，世界额度检查与新预留在同一事务中。

Job、reservation、log、ledger 记录服务端从 Channel 推导的 `WorldID`；
消息合成必须归属有效世界，世界内的角色试听与音色创建也按频道归属。
全局个人音色请求及平台 `system_preview` 的 `WorldID` 为空；系统试听继续
沿用平台计费和缓存行为。迁移仅添加列和世界 policy 表，历史 `WorldID` 保持
空字符串。缓存命中仍检查世界白名单，缓存回放继续沿用原有不重复收费语义。

## 官方依据

- [3.1 Flash 模型及价格](https://help.aliyun.com/zh/model-studio/qwen-audio-3-1-tts-flash)
- [3.0 Flash 模型及价格](https://help.aliyun.com/zh/model-studio/qwen-audio-3-0-tts-flash)
- [3.0 Plus 模型及价格](https://help.aliyun.com/zh/model-studio/qwen-audio-3-0-tts-plus)
- [模型与 HTTP/WebSocket 支持](https://help.aliyun.com/zh/model-studio/tts-model)
- [HTTP/SSE 接口](https://help.aliyun.com/zh/model-studio/cosyvoice-tts-http-api)
- [系统音色（区分大小写）](https://help.aliyun.com/zh/model-studio/qwen-audio-tts-voice-list)

HTTP 参考页目前仍以字符用量示例为主。客户端兼容 SDK 通用用量结构中的
`input_tokens` / `output_tokens`，但不推断未返回的计费字段。
