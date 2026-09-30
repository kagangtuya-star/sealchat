# TTS Provider / Model / Voice 能力目录

`providerKind` 是云服务商品牌（`aliyun` / `tencent`），`providerId` 是管理员配置的具体实例。
JSON/YAML 中旧实例缺少 `providerKind` 时按阿里云处理；读取时补齐，不要求迁移配置文件。

## 目录与 API

- `pkg/ttsprovider/providers.go`：`ProviderCatalog()` / `LookupProvider(kind)` 返回已知品牌及显示名称（阿里、腾讯），不表示模型、音色或运行时已经实现。
- `pkg/ttsprovider/models.go`：`LookupModel(providerKind, modelID)`、模型能力和模型默认音色。
- `pkg/ttsprovider/voices.go`：`TTSVoiceSpec`、`VoicesForModel`、`VoiceSupported` 和 `DefaultVoice`。
- `tts_catalog_basic_20260723.json`、`tts_catalog_31_20260930.json`：阿里云版本化音色目录数据；`voices_aliyun.go`、`voices_aliyun_31.go` 只负责嵌入与归一化装载。JSON 的 `targetModel` 在加载时转换为 `providerKind + models[]`，官方 ID 大小写保持不变。

`GET /api/v1/tts/voices` 的 `system` 包含 `id`、`name`、`providerKind`、`models`、`languages`、`kind`，以及存在时的 `tags`。
响应的 `providers` 为 `[{ kind, name }]`，与 `items`、`total`、`system`、`catalogVersion` 并列。VoicePicker 从它生成「全部 / 阿里预设 / 腾讯预设 / 我的音色 / 公开音色」来源页签；腾讯目前没有模型或音色，显示「当前没有可用的腾讯预设音色。」，不调用腾讯 API。
单模型音色的 `targetModel` 为兼容旧客户端派生，新业务仅使用 `providerKind + models`。
个人音色另带只读 `supported`，由后端检查其创建实例当前的账号和模型配置，账号命名空间不对普通客户端公开。

`GET /api/v1/tts/admin/models` 为每个模型返回 `providerKind`、`capabilities` 和 `defaultVoice`。
三个概念相互独立：

- **ProviderMeta**：品牌展示目录，来自 `ProviderCatalog`；不是管理员实例或能力列表。
- **VoiceContext / VoiceContexts**：系统预设音色与合成路由，结构为 `{ providerKind, providerId, modelId }`。`GET /api/v1/tts/me` 保留表示默认实例的 `voiceContext`，并返回按配置原始顺序排列的 `voiceContexts`，只含同时满足 `Enabled`、APIKey 非空、`LookupModel` 成功、模型支持 `HTTPStreaming` 或 `WebSocketStreaming`、`SynthesisPriceConfirmed()` 为 true 的实例。未确认合成价格的非默认 Provider 可以继续存在于管理员配置，但不进入 `voiceContexts`，因此不会作为用户可选合成来源；显式 0 价格属于已确认价格，仍可进入。腾讯品牌注册不会使其进入此列表。旧后端缺少多实例字段时，前端使用单个 `voiceContext` 兼容。
- **VoiceCreationTarget**：`GET /api/v1/tts/voice-targets` 提供的 `providerId + modelId + ModelCapabilities`，仅供 design/clone；不会合并 `voiceContexts` 或系统音色绑定。

系统音色在 UI 中优先选择兼容默认 context，否则选 `voiceContexts` 中第一个兼容项；点击后冻结完整实例路由。个人音色目录不再按默认 context 过滤，availability 使用 `saved`、`providerStatus == OK` 和后端 `supported !== false`，合成仍绑定其创建实例。
来源与语言/类型/标签属性筛选独立。切换来源清属性筛选并保留搜索；清除筛选按钮只在展开筛选区内且有激活属性时出现，不修改来源、搜索或音色选择。context 更新会重置分页、筛选和个人目录/失效查询缓存，不改写选择。
管理员的默认音色下拉选项来自系统目录，切换默认实例或模型时保留兼容选择，否则使用模型目录的默认值。

`GET /api/v1/tts/voice-targets` 向已登录用户返回启用且有 API Key 的创建实例：`providerId`、`providerKind`、仅含实例实际配置模型的 `models`（`id`、`providerKind`、`name`、`capabilities`，以及 clone 的 `cloneLanguages` / `supportsClonePreprocess`）、实例的 `designPrice` / `clonePrice`；价格仅来自实例级字段，`null` 表示尚未确认，不返回模型价格、默认音色、凭证、账号命名空间或 endpoint。
创建面板按 `voiceDesign` / `voiceClone` 过滤目标，默认使用 `voiceContext.providerId + modelId`，切换方式时保留支持的目标，否则选择第一个可用项。
`POST /api/v1/tts/jobs/design` 和 `/clone` 可携带 `providerId`、`modelId` 指定创建目标，必须成对提供且与实例配置精确一致；省略时保留历史解析行为。新面板总是显式提供目标，独立于系统/个人音色绑定。后端检查模型能力并冻结实际 provider，Worker 继续以其 `Model` 作为 `voice-enrollment/create_voice` 的 `target_model` 和个人资产的 `TargetModel`。这些字段不参与 audition 或消息合成路由。

历史配置读取可以修复不兼容的默认音色；配置写入使用 `NormalizeSpeechConfigForWrite` 保留显式非空选择，随后严格校验，非法组合返回 400。
未指定音色时使用平台配置的兼容默认值；缺少默认值时由能力目录提供。阿里 3.0 Flash、3.1 Flash 和 3.0 Plus 默认分别为 `longanhuan_v3.6`、`longanhuan_v3.1`、`longanlingxin`。

## 角色绑定与个人资产

新系统绑定保存 `systemVoice`、`systemVoiceProvider`（ProviderKind）、`systemVoiceProviderId`（具体管理员实例）和 `systemVoiceModel`。数据库由现有 AutoMigrate 增列，不批量迁移历史数据。角色保存验证成功后回写实际完整 route；清空系统音色时同时清空三个 namespace 字段。自动消息朗读读取全部字段。
`TTSRequest.providerId/modelId` 专门用于 design/clone 创建目标；系统合成使用独立的 `systemVoiceProviderId`，不改变 `systemVoiceProvider` 的品牌含义。

后端系统路由规则：

1. 携带 `systemVoiceProviderId` 时精确匹配该实例，要求启用、有 API Key、给出的品牌/模型一致且音色支持该模型；任何不符返回 validation error，禁止改用其他实例。
2. 没有实例 ID 的历史绑定优先使用兼容默认实例，否则查全部启用且有 API Key 的兼容候选；只有一个候选时使用它，多个或没有候选时显式试听/保存返回 validation error，自动朗读将旧绑定视为 stale 并使用 DefaultProvider + DefaultVoice。旧品牌/模型字段要么同时为空，要么构成完整匹配 namespace；部分旧 namespace 仍视为失效。
3. 无音色绑定继续继承 DefaultProvider + DefaultVoice。

角色编辑显示失效状态，读取和自动朗读不会重写数据库。UI 兼容两代历史系统绑定：品牌、实例、模型全部为空的最老绑定，以及已有 providerKind + modelId 但缺少 providerId 的上一代绑定；两者均优先使用兼容默认实例，否则仅在候选唯一时解析，存在歧义时标记为不可用。部分 namespace 仍视为失效。已选实例仍可用时，默认实例变化也不会覆盖它。

个人设计/复刻音色的资产结构不变，`PersonalVoiceSupported` 校验创建时的 ProviderID、CredentialScope、Region、Workspace 和 TargetModel，并核对模型所属 ProviderKind。Region/Workspace 是现有 provider-specific identity 的组成部分；未来适配器按真实账户资源语义扩展，不预造腾讯 Workspace。
请求继续通过创建实例合成；不会把个人资产路由至其他实例，也不为失效个人音色静默换声。

## 增加服务商

`service/tts_provider_runtime.go` 是极薄的 provider runtime dispatch seam，收敛 Worker / Maintenance 的合成、创建、查询、删除及完整音频下载分发。只在 `ProviderAliyun` 分支复用现有 `ttsprovider.Client`；其他品牌返回 unsupported provider，不自动重试。
Aliyun clone 在该分支内调用 `TTSCloneReadURL(job)`，保持 HTTPS Domain、短期 expires、token、reservation/job 绑定与附件授权。此签名公网 URL 是 Aliyun 的样本协议，不是所有 provider clone 的通用前置条件。

ProviderCatalog 注册 Tencent 不等于腾讯已实现：`LookupProvider("tencent")` 成功，`LookupModel("tencent", "fake")` 失败；没有腾讯 Client、Endpoint、ModelSpec、VoiceSpec、价格或 capability。

真正接入腾讯时还需增加 Tencent ModelSpec、Tencent VoiceSpec、Tencent 配置解析、Tencent runtime adapter（包括其真实样本协议），以及 Tencent 专属价格/usage 适配。无需重新修改 VoicePicker 来源结构、voiceContexts 协议、系统音色角色命名空间或创建目标协议。
目录采用静态数据与纯查询函数，无 plugin framework / interface registry。

运行时分发保持现有 Aliyun payload、收费/usage、流 framing、spool/archive、长文本切片、播放队列与实时 PCM 行为。
