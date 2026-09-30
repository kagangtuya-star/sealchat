# TTS Provider / Model / Voice 能力目录

`providerKind` 是云服务商类型（目前为 `aliyun`），`providerId` 是管理员配置的具体实例。
JSON/YAML 中旧实例缺少 `providerKind` 时按阿里云处理；读取时补齐，不要求迁移配置文件。

## 目录与 API

- `pkg/ttsprovider/models.go`：`LookupModel(providerKind, modelID)`、模型能力和模型默认音色。
- `pkg/ttsprovider/voices.go`：`TTSVoiceSpec`、`VoicesForModel`、`VoiceSupported` 和 `DefaultVoice`。
- `voices_aliyun.go`、`voices_aliyun_31.go`：阿里云音色数据。旧版 JSON 在加载时转换为 `providerKind + models[]`，官方 ID 大小写保持不变。

`GET /api/v1/tts/voices` 的 `system` 包含 `id`、`name`、`providerKind`、`models`、`languages`、`kind`，以及存在时的 `tags`。
单模型音色的 `targetModel` 为兼容旧客户端派生，新业务仅使用 `providerKind + models`。
个人音色另带只读 `supported`，由后端检查其创建实例当前的账号和模型配置，账号命名空间不对普通客户端公开。

`GET /api/v1/tts/admin/models` 为每个模型返回 `providerKind`、`capabilities` 和 `defaultVoice`。
`GET /api/v1/tts/me` 返回 `voiceContext: { providerKind, providerId, modelId }`，供选音器过滤当前可用系统和个人音色。
管理员的默认音色下拉选项来自系统目录，切换默认实例或模型时保留兼容选择，否则使用模型目录的默认值。

`GET /api/v1/tts/voice-targets` 向已登录用户返回启用且有 API Key 的创建实例：`providerId`、`providerKind`、仅含实例实际配置模型的 `models`（每项仅有 `id`、`providerKind`、`name`、`capabilities`）、实例的 `designPrice` / `clonePrice`；价格仅来自实例级字段，`null` 表示尚未确认，不返回模型价格、默认音色、凭证、账号命名空间或 endpoint。
创建面板按 `voiceDesign` / `voiceClone` 过滤目标，默认使用 `voiceContext.providerId + modelId`，切换方式时保留支持的目标，否则选择第一个可用项。
`POST /api/v1/tts/jobs/design` 和 `/clone` 可携带 `providerId`、`modelId` 指定创建目标，必须成对提供且与实例配置精确一致；省略时保留历史解析行为。新面板总是显式提供目标，独立于系统/个人音色绑定。后端检查模型能力并冻结实际 provider，Worker 继续以其 `Model` 作为 `voice-enrollment/create_voice` 的 `target_model` 和个人资产的 `TargetModel`。这些字段不参与 audition 或消息合成路由。

历史配置读取可以修复不兼容的默认音色；配置写入使用 `NormalizeSpeechConfigForWrite` 保留显式非空选择，随后严格校验，非法组合返回 400。
未指定音色时使用平台配置的兼容默认值；缺少默认值时由能力目录提供。阿里 3.0 Flash、3.1 Flash 和 3.0 Plus 默认分别为 `longanhuan_v3.6`、`longanhuan_v3.1`、`longanlingxin`。

## 角色绑定与个人资产

新系统绑定保存 `systemVoice`、`systemVoiceProvider`（ProviderKind）和 `systemVoiceModel`。
后两个字段都为空的历史绑定在当前 ProviderKind/Model 下按 ID 解析；部分命名空间或不匹配的绑定视为失效。
角色编辑显示失效状态，读取和自动朗读不会重写数据库。自动朗读遇到失效系统绑定回退平台默认音色，显式试听和保存失效绑定返回 validation error。
试听请求也可以携带 `systemVoiceProvider` 和 `systemVoiceModel`，避免模型切换后重复 ID 被重新解释。

个人设计/复刻音色的资产结构不变，`PersonalVoiceSupported` 校验创建时的 ProviderID、CredentialScope、Region、Workspace 和 TargetModel，并核对模型所属 ProviderKind。
请求继续通过创建实例合成；不会把个人资产路由至其他实例，也不为失效个人音色静默换声。

## 增加服务商

增加服务商时新增 provider adapter、该厂商的模型条目与音色数据，并接入厂商的配置校验、凭证解析和 adapter 调用入口。
目录采用静态数据与纯查询函数，无插件注册或预建空接口。
配置默认音色解析、系统音色最终兼容性校验、角色命名空间与失效处理、VoicePicker 和管理员默认音色下拉无需增加厂商白名单。

本次不改 Worker、结算、Token usage、长文本切片、播放队列、并发或 WebSocket 合成协议，也未实现其他厂商客户端。
