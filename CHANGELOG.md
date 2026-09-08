# Changelog

WuKongIM release notes are maintained here. User-visible pull requests add
their entries under `Unreleased`; before a tag is created, release maintainers
move those entries into a version section named for that exact tag.

## [Unreleased]

<!--
Use only the non-empty categories that apply: `⚠️ Breaking Changes /
破坏性变更`, `🚀 New Features / 新功能`, `🐛 Bug Fixes / 问题修复`,
`🔧 Improvements / 改进`, `⬆️ Upgrade Notes / 升级说明`,
`🔒 Security / 安全`, `📚 Documentation / 文档`, and
`⚠️ Known Issues / 已知问题`. Prefix the selected category with `### `.

Every category must contain at least one "- " list entry. Release headings use
the exact form: ## [v3.0.0-beta.5] - 2026-09-01
-->

### 🚀 New Features / 新功能

- `wkmigrate` 支持按捕获摘要、用户／频道身份和保留消息尾序号限定补齐缺失会话，将已核验历史设为已读；原生历史可见范围保持完整，未批准或证据变化的情况仍拒绝。

- `wkmigrate` 可显式接受经完整消息前缀、正式副本多数和已应用配置日志证明的源副本落后；保留当前 Leader 原历史并在归档重建时重新核验，默认仍阻断空 Leader、同任期换主及内容冲突。另可按显式恢复决定选取空 Leader 频道的一份完整正式多数副本，绑定捕获、完整诊断和消息摘要；未知异常仍阻断。

- `wkmigrate` 支持插件程序分块归档、离线重建、原生目录安装及独立文件校验；显式兼容规则仅接受已审计的 AI 示例 Linux/amd64 原程序、Receive 注册和统一配置。未知程序仍阻断，精确重试拒绝程序被替换或权限漂移。

- `wkmigrate` 可显式将原用户创建／更新时间完整留档，以适配没有对应字段的 v3 原生用户表；报告绑定每个节点的原值，归档重建重新核对，用户身份、插件绑定及设备凭据仍严格比较。

- Add explicit offline stream-message exclusion with source archives, continuous per-channel sequences, mapped read/delete boundaries, and native append/restart validation. / 离线迁移支持显式排除流消息，完整归档源行、连续编号频道消息并映射读删位置，验证原生追加与重启接续。

- `wkmigrate dedupe-plan` 报告 v3 新增按节点的消息字段影响统计，区分排除 CMD、去重后的保留消息与排除消息，并提供有界样本；不修改协议字段、不放宽兼容检查，需使用新的规划工作空间。

- 迁移计划新增 `plugin_configs`，可让指定插件统一采用某源节点的有效配置，同时保留各节点启用状态、时间戳和全部原配置。导入配置纳入批次完整性检查，离线校验从原始归档独立重算并拒绝缺失、篡改或多余配置；业务兼容检查仍保留。

- 新增插件迁移的单节点/三节点隔离验收，串联原绑定导入、默认程序扫描、节点配置、真实消息落库，并在两次完整重启后、发送新消息前核对全部已有消息和回复；发现逐节点配置会影响自动回复内容时，仍阻止业务兼容放行，支持对已批准的指定插件显式选择统一配置来源。

- 迁移工具支持把原 `PluginUser` 绑定按 UID 重新分配到 v3 原生表，并从源归档独立核对绑定字段、副本数量和双向查询；原纳秒时间保留在归档，目标沿用原生毫秒精度。插件程序、配置及业务行为仍须通过兼容验收。

- 迁移计划可显式指定 `plugin_nodes`，为每个目标选择配置来源，支持同一来源扩展到多个目标及缩容时明确选择来源；全部原配置留档，保持所选启用状态和时间戳。遗漏或重复目标、未知来源时拒绝，归档重建独立重算；该步骤不解除插件业务兼容检查。

- `wkmigrate diagnose` 的插件兼容问题新增各节点的配置、方法及原记录指纹，可识别节点配置差异且不输出配置值；诊断结果不代表插件已兼容，也不放宽迁移检查。

- 迁移工具可按原唯一索引与当前 Slot Leader 列表核对重复会话，保留已读、删除位置及列表版本，并检查去重前数量上限；空频道管理记录仅在无业务引用、正式副本及原配置日志一致时完整归档。两项均须显式启用，归档重建重新验证，默认仍严格拒绝。

- `wkmigrate` 新增显式 `metadata.device_lookup=v2_cold_start` 选项，按原版冷启动登录的 UID 索引保留最小设备 ID 的完整凭据；原始重复记录留档，归档重建重新选择并验证。默认拒绝重复凭据，不推断停机前缓存，也不放宽副本一致性或会话检查。

- `wkmigrate prepare` 从原始捕获的 Slot 配置命令和完整消息历史重建来源证明，支持有充分证据的补副本状态及历史 Leader 自指标记；归档携带命令并独立重验，缺失、改动或错位命令仍拒绝，不改变原库或 v3 放置与存储逻辑。

- `wkmigrate` 支持显式排除 CMD 消息、保留最新重复消息并压紧剩余序号；同步映射普通会话的已读/删除位置，省略旧 CMD 同步位置，生成可校验序号映射，支持 `export-map` 从归档重建。原始记录完整归档，跨频道冲突及不确定来源仍拒绝，不改变 v3 存储逻辑。

- 新增 `wkmigrate dedupe-plan`：按频道内原序号规划重复 MessageID 或发送者 ClientMsgNo 的最新记录保留，输出每条候选删除记录、保留依据及连续序号影响；源数据保持不变，跨频道身份冲突与相互淘汰的保留记录明确阻止规划通过。

- 新增 `wkmigrate authority` 专项核验：只读核对带迁移标记的源频道配置、已保留的 Slot 配置日志及逐序号副本消息，输出校验和绑定的分类证据；不清除迁移标记、不选择或修改业务数据，也不放宽迁移门槛。

- 新增 `wkmigrate diagnose` 全量源诊断：收集所有可读节点的兼容问题，以磁盘排序统计重复 ID/索引冲突，输出分节点数量、有限样例和带校验和的完整明细；扫描不完整时明确标记，诊断结果不能作为迁移通过凭据。

- `wkmigrate` 支持显式排除升级遗留的旧流分片及元数据，保留主消息及原序号；排除项完整归档并列出数量和校验摘要，默认仍拒绝，插件兼容检查不受影响。
- 新增 `wkmigrate prepare/export/import/verify` 离线迁移命令，读取未经升级的固定 v2 源版本，导入全新 v3 集群，并校验业务字段、消息索引、摘要链、初始化快照与副本记录数量。相同计划支持中断恢复；拒绝不兼容业务插件、关键源索引损坏和超出原生恢复预算的单条消息，不覆盖已有业务目标。大规模性能验收尚未完成。

### 🐛 Bug Fixes / 问题修复

- 修复原生恢复过程中的错误隔离：独立元数据请求不再继承同批请求的校验错误；副本尚未提升时 Leader 故障可撤销旧补建任务后重新选主；追加转发连接不可用返回可重试错误。

- 修复原生频道替换副本无法追平的问题：通过有界后台任务复制已提交历史，新副本不计入写入 quorum；修复恢复检查读取已加载副本的过期进度。

- `wkmigrate prepare` 遇到活动插件时继续执行独立的源业务副本检查，失败时输出已完成的检查证据并保持退出码 1；插件未兼容时仍不生成可转换、可导出的准备结果。

- 迁移候选同步 main 已有的空会话兼容修复：授权成员读取尚无消息的频道时返回空数组，保留成员限制和真实故障；补充全流消息排除后的 HTTP 同步、未读操作及重启追加验收。

- Recover cold quorum Channel leaders before history and conversation reads, including restarted empty replicas; return errors while recovery is unavailable instead of incomplete successful pages. Keep uncommitted messages invisible when reverse-reading a zero committed watermark. / 修复多节点重启后历史末条消息或会话遗漏：读取前完成原生 quorum 恢复，并阻止倒序读取暴露未提交消息。

- Fix read-only message catalog pagination across different-length channel keys so diagnostics and migration verification do not omit channels at page boundaries.
- Recover native cold Channel replicas for leader failover and allow quorum verification while a transfer write fence remains active; business writes remain fenced until cutover validation finishes.
- Preserve native `SyncOnce` in Channel RPC v8 so remote history reads keep recovery barriers and CMD records filtered; reject lossy legacy-codec fallbacks without changing storage.
- Preserve original message `red_dot` in the existing v3 stored flag and through ordinary send, history reads, replication and recovery; do not archive or clear it during v2 migration. Target nodes require Channel RPC v8 / Exchange v4 and a coordinated all-node upgrade.

- `wkmigrate authority` 识别旧 v2 原样保存配置版本的规则；仅在所有所属 Slot 副本的前后命令、完整配置字段及消息历史均匹配时，识别历史 Leader 指向自己的无成员变更标记。缺失证据仍阻止候选判定，不改写原库，也不放行未证实的状态。

- 迁移输出遵守现有 v3 消息存储与复制约束；不再引入迁移专用摘要格式或历史前缀恢复分支。无法完整保留的协议字段、非 1 起始历史及重复 ID 明确阻止迁移，不改写标识或额外丢弃数据。

- 迁移预检核对原插件绑定的主键与用户、插件双向业务索引，避免把原账号旧插件字段为空误判为无绑定；插件运行及配置兼容门槛仍保持。

- 迁移预检按原版语义保留零配置版本和非空 ID 的类型 0 源频道配置；仍按所属 Slot 核对副本一致性，不改写版本，也不放宽消息、会话和频道策略的身份检查。
- 迁移工具无损保留原 v2 会话及频道配置中的非 UTF-8 标识，按原字节关联、路由和校验，避免 JSON 替换字符合并不同身份；更新工具后须使用新的工作空间和归档。
- 迁移预检识别原 v2 独立写入的频道成员计数，并从原用户、设备解析个人权限频道；完整保留黑名单及原始计数，避免误报缺失频道身份或凭计数创建频道属性。无法解析的实际权限和空身份记录仍拒绝。
- 迁移配置比较保留原始历史离线统计作为证据，但不再将各节点的离线次数、离线时间差异误判为权威配置冲突；Leader、任期、副本及复制日志检查保持严格。
- 迁移预检按原版 v2 恢复语义处理空用户 ID 的会话缓存：完整归档并记录数量，不再误报为无效已读位置，也不创建空用户会话。

- 恢复原版 `/message/eventsync` 的持久事件投影读取，保留事件序号、分页及可见性过滤行为。

- `/message/send` 现在会在 `from_uid` 与兼容别名 `sender_uid` 均为空时使用系统账号，并支持通过 `message.system_uid` / `WK_MESSAGE_SYSTEM_UID` 配置该账号（默认 `____system`），恢复 Demo 命令消息的旧版兼容行为。

### 🔧 Improvements / 改进

- 内嵌聊天 Demo 新增中英文界面，根据浏览器语言偏好自动选择，未匹配支持语言时默认使用英文。
- 频道消息同步、旧会话同步与插件读取共用分页规则，继续保持各入口原有的可见范围、返回顺序和错误语义。

- Manager 消息诊断页现在会在集群节点未启用 diagnostics 时显示 TOML 与环境变量配置指引、标出受影响节点并禁用无效的追踪操作，不再直接暴露内部错误文本。
- Manager 节点列表现在显示每个节点当前运行的 WuKongIM 程序版本，便于识别滚动升级期间的版本差异。

### 📚 Documentation / 文档

- 补充原版 v2 数据迁入指定三节点 v3 测试目录的部署验收报告，记录旧环境备份、真实 SDK 缓存重置、插件回复、完整重启和监控检查。

- Document the legacy stream-parent audit and the explicit decision to omit stream messages while preserving continuous sequences. / 记录旧流主消息语义核对及本次排除流消息、保持连续序号的明确范围。

- 中英文文档补齐 Web 双用户接入闭环，统一教程文本消息与 Manager 访问方式，并提供可下载的监控告警、压测配置及结果解读。

- 文档站“资源”菜单的聊天演示与 Manager 演示入口已切换到新的 HTTPS 域名。
- Product HTTP API 参考现在统一完整合同与窄 Profile 的信任等级和示例，补齐响应字段说明、可执行条件 Schema、节点本地与分阶段写入语义，并明确 HTTP 200 后仍需检查的业务结果。
- Docker 部署文档补充配置文件权限要求，明确官方镜像的 `10001:10001` 非 root 身份、推荐的 `0640` 权限，以及仅 root 可读导致容器启动失败的诊断信息。
- 文档站的当前发布版本现在从根 Changelog 的最新版本标题构建期注入，并在不可变二进制发布成功后自动部署；后续发布无需再手工同步中英文 Linux、Docker 示例及其测试。

## [v3.0.0-beta.8] - 2026-09-04

### ⚠️ Breaking Changes / 破坏性变更

- Gateway 现在默认启用 CONNECT Token 鉴权，并按 UID 与设备类别精确校验 `/user/token` 持久化的设备凭据；升级前必须先为客户端准备 Token，或仅在受控兼容迁移期间设置 `gateway.token_auth_on=false`。

### 🐛 Bug Fixes / 问题修复

- 频道消息同步现在会在应用成员可见性下限时保留“最新一页”的零边界语义，消息超过单页时不再误返回最早一页。

### 📚 Documentation / 文档

- 项目根目录恢复 Apache License 2.0 正文，并将中英文 README 的许可证声明链接到仓库内文件。
- Product HTTP API 文档现已逐一对齐 41 个运行时路由的请求 DTO 与校验语义，为所有查询和 JSON 请求参数补充中英文解释，并修正空白字符串与 `null` 的兼容约束。
- 服务端部署与运维文档改为面向初学者的精简实操指南，快速开始统一使用 Linux 软件包与 systemd，并修复已移除页面的链接。
- 中英文 Linux 部署与配置参考现已将 `v3.0.0-beta.7` 作为签名 Preview 软件源版本，并以 `sudo wukongim init` 作为默认初始化入口，同时保留显式路径兼容命令；Docker 部署示例也同步固定到同一版本。

## [v3.0.0-beta.7] - 2026-09-03

### 🚀 New Features / 新功能

- 原生 Linux 安装新增 `sudo wukongim init` 快捷入口，默认安全生成 `/etc/wukongim/wukongim.toml`，同时保留原有显式路径命令用于兼容和自定义位置。
- 签名 Linux Preview 软件源新增 `wukongim-archive-keyring` 与 `wukongim-release` 引导包，首次配置后可直接通过 APT 或 DNF/YUM 安装和升级 WuKongIM，并由包管理器自动接收后续公开签名证书更新。

### 🐛 Bug Fixes / 问题修复

- 文档 CDN 现在仅对静态导出的页面 RSC `index.txt` 从缓存键删除 `_rsc`，并在构建期验证每个已发布路由都有独立静态载荷；章节跳转不再因一次性 RSC 查询值反复触发高延迟回源，同时保留图片及其他查询参数的缓存隔离。
- 文档 Pages 自定义域名迁移与回滚现在会先暂存已验证产物，在域名绑定变化后立即重新部署，并以绕过 CDN 的根路径、双语首页、深层页面及搜索真实 GET 作为内容就绪门禁；证书批准或 API `204` 不再被误当作站点可用。
- Manager 精确查询频道运行时元数据时，现在会按频道 Leader 路由最大消息序号读取；非 Slot 副本节点不再因本地缺少元数据而误报 404。
- 多副本频道 Leader 重启后，最近会话重试会在持久化提交 checkpoint 落后于本地日志时按当前元数据激活冷 runtime 并恢复 quorum 高水位，不再把仍存在的会话返回为空。
- 多副本频道的消息拉取现在会在持久化 checkpoint 落后时使用活动 Leader 已确认的提交高水位，发送成功后的首条消息可立即出现在频道与最近会话同步结果中。
- 文档 CDN 证书检查现在根据显式公网路由模式和多个公共解析器的直接 CNAME 答案决定是否验证阿里云边缘证书，不再将供应商 `DomainCnameStatus` 误当作公网切流证明。
- 文档 CDN 证书检查现在正确接受“已安装证书且暂无需续期”以及强制初始化时“尚未安装证书”的布尔结果，同时仍会拒绝缺失或类型错误的状态字段。
- 文档 CDN 证书轮换现在兼容阿里云已启用的手动上传证书省略 `Status` 字段的响应，同时仍会拒绝免费或未知类型的空状态以及尚未生效的证书状态。
- 文档 CDN 的 ACME 账户初始化现在使用固定生产端点和已审阅条款的账户专用流程，不再因合法邮箱或 Let's Encrypt 省略可选联系人字段而失败，也不会在初始化账户时误发起证书申请。
- WKProto 编解码器现在会安全处理空输入并拒绝未知帧类型编码，避免畸形输入触发越界崩溃或静默产出空报文。
- 插件热重载监视器现在在启动后立即停止时保持完成信号的稳定引用，避免并发清理将其置空后引发 `close of nil channel` 崩溃。
- Issue Agent 验证器现在会在判定工作目录越界前规范化 checkout 根路径，避免 macOS 上 `/var` 与 `/private/var` 别名导致合法子目录被误拒。
- JSON-RPC 解码器现在会将未知通知方法统一归类为 `ErrUnknownMethod`，与未知请求的错误分类保持一致，便于调用方稳定识别协议错误。
- JSON-RPC subscribe/unsubscribe 请求现在会在协议适配层转换为带正确 action 的 `SUB` 帧，并将可用的 `SUBACK` 关联到原请求；当前 Product Gateway 仍未发布 `SUB` 入站能力。
- 权限元数据批量读取现在会在进入 Slot 代理前拒绝未知读取类型，并保持其余合法结果的原始对齐，避免无效类型被下游结果覆盖或污染整批授权证据。
- Controller Slot 副本迁移与 Leader 转移在运行时未启动时现在统一 fail closed 为 `ErrNotStarted`，不再根据空状态返回误导性的业务校验错误。
- Slot FSM 现在会为确定性的过期元数据提案持久化已应用水位，节点重启后不再重复回放已经判定为无操作的 Raft 日志。
- 元数据存储关闭后清理终态频道迁移任务现在返回关闭错误，不再因访问已释放的底层数据库而触发空指针崩溃。
- 消息恢复后缀替换现在会在写入前校验保留边界的 Proposal 与 Entry 身份一致性，检测到损坏时拒绝替换并保留原有后缀。
- 元数据备份与恢复现在拒绝夹带运行时或迁移状态、跨注册 span 乱序及重复键的快照，并在完整性预检失败时保留目标端原有数据。
- 完整备份发布现在会在写入任何仓库对象前校验全部 256 个 Hash Slot 的完成进度，避免不完整任务提前绑定空仓库或留下发布副作用。
- Controller Raft 启动时若物化状态文件丢失且无可用快照，现在会从保留 WAL 重建；若日志已压缩且快照数据缺失则拒绝启动，避免以空状态继续运行。
- 集群节点停止时现在会撤销路由、Slot 与频道就绪状态，并阻止在途控制快照在停止后重新发布就绪，避免停机窗口暴露错误健康状态。
- 应用启动失败回滚现在会先关闭已开放的 Prometheus、Manager 与 API 入口，再停止备份调度运行时，避免回滚窗口继续接受新的管理请求。
- 阿里云 Lease 盘点、主机创建与身份移除现在会拒绝子资源角色冲突、跨实例磁盘响应及无法由 SDK 错误码证明已删除的身份资源。
- 阿里云仿真账户 Bootstrap 现在会安全处理官方 SDK 错误响应，并仅依据结构化错误码判断资源不存在，避免错误路径崩溃或把普通服务与传输故障误判为已删除。
- 阿里云只读权限探针现在兼容官方 SDK 的两种结构化 403 错误类型，合法 RAM 拒绝不再被误判为探针失败。
- 消息备份流回放现在会拒绝 `log_start_offset` 超过提交高水位的非法 checkpoint，避免校验和合法但语义损坏的快照进入恢复流程。
- 云部署离线文件适配器现在会拒绝负数读取与清单上限，并正确处理最大整数读取边界，避免非法参数触发 panic 或把已有文件静默读成空内容。
- Slot 代理现在会沿包装错误链识别“Slot 不存在”，避免上游附加上下文后被误分类为“暂无 Leader”并返回错误的 RPC 路由状态。
- WKDB bundle 导出现在会拒绝无法表示为 `int64` 的无符号 inspect 字段，并保留非法频道目录状态的 `ErrValidation` 分类，避免损坏或类型异常的数据被静默回绕或失去可识别的验证错误。
- Cloud View 现在会按原始文件大小严格拒绝超过 256 KiB 的配置和超过 64 KiB 的运行状态，即使超量部分仅为尾随空白，也无法再绕过文件上限。
- Cloud Simulation 现在会在创建任何付费资源前校验完整的 Run Locator 参数，并按原始输入大小拒绝超过 64/128 KiB 的请求与阿里云配置，避免无效命令留下资源或通过尾随空白绕过上限。
- Cloud Host 在线与离线安装现在都会在执行任何主机副作用前拒绝无效的远程根目录前缀，避免参数错误导致部分安装状态残留。
- Cloud Bundle 现在会按完整输入大小拒绝超过 128 KiB 的部署 spec，合法 JSON 后追加尾随空白也无法再绕过上限。
- Gateway 会话的 `LoadOrStoreValue` 现在保留已存储的 `nil`，并在并发初始化保留热键时只允许一个调用方取得写入权，避免会话状态被后续竞争者覆盖。
- Cloud Analysis Bridge 现在会拒绝固定 PEM 证书前的非空白数据，避免额外内容被 PEM 解析器静默忽略。
- Review Agent GitHub 适配器现在保留取消与超时错误身份，并严格限制写操作响应体大小，避免上层误判重试语义或读取过量响应数据。
- Cloud Analysis 诊断结果现在会拒绝 NaN 和无穷大置信度，确保分类一直满足 `[0,1]` 约束。
- `wkcli bench` 现在会拒绝无法安全表示的超大 payload 尺寸，并将帮助与错误文本写入命令注入的对应输出流。
- Raft 日志现在会将 `leader lost` 归类为 `leader_change` 事件，避免 Leader 丢失信号被误计入普通日志。

### 🔧 Improvements / 改进

- 文档静态构建现在生成并校验固定域名、排序唯一且有数量上限的 RSC `index.txt` URL 清单，为后续精确刷新设计提供可审计输入；当前发布流程仍只刷新原有四个 URL，不新增预热、权限或 TTL 变更。
- 原生 Linux 包 CI 现在会在 Ubuntu 24.04、Debian 12、Rocky Linux 9 与 AlmaLinux 9 的真实 systemd 环境中验证配置初始化、健康检查、显式启停/重启、活动卸载、状态保留及重装不自动激活。
- 文档发布新增默认关闭的阿里云 CDN 定点刷新与 Let's Encrypt DNS-01 边缘证书轮换支持；两条路径使用独立的 GitHub OIDC 角色，不在仓库保存长期阿里云凭据，且在完成外部配置和切流前不会改变现有 GitHub Pages 服务。
- Chat Lifecycle 正式演练启动器、正式收尾器和通用 Cloud Lease 回收扫描现在仅在存在 transition、handoff、付费资源生产者或云端库存期间启用；取得完整空闲与零库存证明后会自动停用，并在下一次 transition、停止请求、精确清理或付费 Acquire 前安全恢复；完整 Artifact 盘点会重试短暂的 GitHub API 分页错误。

### 🔒 Security / 安全

- 二进制发布的手动恢复现在必须从目标版本的精确 tag ref 启动，并同时绑定事件提交与 Workflow 提交；从 `main` 为其他 tag 生成无法被软件源信任的 provenance 会在构建前失败。

### 📚 Documentation / 文档

- Linux 服务端部署文档将软件源安装收敛为统一的 `curl -fsSL https://packages.githubim.com/repo | sudo sh` 添加源入口，再显式更新索引并按包名安装；首次添加后不再手工下载特定版本的 WuKongIM deb/rpm。
- Linux 软件源引导脚本明确只添加签名软件源，不会更新索引、安装 WuKongIM 或启动服务；文档继续保留首次 HTTPS 信任边界、底层 APT/RPM 引导包和固定主密钥指纹说明。
- Linux 服务端部署文档现提供 `v3.0.0-beta.6` 签名 Preview 软件源的 APT 与 DNF/YUM 安装流程，并在写入专用 keyring 前固定核对仓库主密钥指纹。
- 中英文 v3 文档站现由仓库内 GitHub Pages 工作流执行完整静态验收后发布到 `docs.githubim.com`，发布产物与通过验收的 `docs-site/out` 保持一致。
- WuKongEasySDK 中英文文档现固定 Web `2.0.4`、Android `1.0.5`、iOS `1.1.1` 与 Flutter `1.1.0` 正式包，并补充四端 example 与正式包的独立验收回执及可复现流程。
- Docker 服务端部署现在提供精简的 `docker run` 与 Docker Compose 两种流程，共用最小 `wukongim.toml`、持久数据卷和完整配置参考；中英文教程删除远程一键安装脚本及其自动版本解析说明，保持两步完成启动和就绪验证。
- 服务端配置文档新增独立的中英文常用配置页，以表格解释最常用的 10 个配置项；配置参考改写为可搜索的逐字段手册，为全部公开 TOML 与 `WK_*` 配置补充用途，并标明关键默认值、`0` 值、互斥、敏感和迁移说明。
- 文档站资源菜单移除官网链接，并使用聊天演示与 Manager 演示当前可用的 HTTP 地址。
- 中英文公共文档新增可访问的 Mermaid 架构与流程图，精简产品、指南和部署导航，并将已撤下的 Kubernetes 页面重定向到受支持的部署入口。

## [v3.0.0-beta.6] - 2026-09-01

### 🐛 Bug Fixes / 问题修复

- 服务端二进制现在内置 IANA 时区数据库，官方最小 Docker 运行时可在 Manager 中保存 `Asia/Shanghai` 等非 UTC 备份计划，不再因运行镜像缺少 `zoneinfo` 返回无效请求。
- 二进制发布恢复流程不再使用 Actions `GITHUB_TOKEN` 无权访问的仓库管理接口，并可从精确工作流提交取得旧标签缺失的 Release Notes 解析器；不可变 Release 设置改由管理员在发布前外部核验，流程仍在发布后强制验证 Release 已封存为不可变。

### 🔧 Improvements / 改进

- GitHub Release 正文现由人工维护的 Changelog 生成，并在二进制文件与三个 Docker 镜像仓库的版本身份、摘要和平台验证完成后再公开发布。

### 📚 Documentation / 文档

- Docker 服务端部署文档已切换到三仓库同摘要的 `v3.0.0-beta.5` 非 root 镜像，并同步更新预发布风险说明。

## [v3.0.0-beta.5] - 2026-09-01

### 🚀 New Features / 新功能

- GitHub Release 新增未签名的 Linux amd64 DEB/RPM 安装包，并与四个平台的压缩包共用校验和与构建来源证明；软件源发布仍保持关闭。

### 🔧 Improvements / 改进

- Chat Lifecycle 演练收尾定时器现在仅在付费演练或待清理 handoff 存在期间启用，取得全局空闲与零库存证明后自动停用，避免仓库空闲时持续产生 GitHub Actions 运行。
- Chat Lifecycle handoff 发现现在可安全穷尽最多 20,000 个保留 Artifact，仓库中超过 5,000 个无关 Artifact 时不再阻塞空闲定时器停用。
- 官方 Docker 镜像新增 `/readyz` 健康检查和 `SIGTERM` 优雅停止契约，并显著缩小 Docker 构建上下文。

### ⬆️ Upgrade Notes / 升级说明

- 官方 Docker 镜像默认改为 UID/GID `10001:10001` 非 root 用户；命名卷可直接使用，自定义宿主机绑定目录需在升级前授予该 UID/GID 写权限。

### 🔒 Security / 安全

- Docker 运行时升级并固定到受支持的 Alpine 3.24.1 摘要，构建基础镜像同步固定摘要，镜像内 Go 安全相关依赖完成升级。
- Docker 发布流程现在会分别扫描 amd64 和 arm64 候选镜像；发现 Critical 或 High 漏洞时阻止发布，恢复发布也会重新扫描现有规范摘要。

### 📚 Documentation / 文档

- 中英文 Docker 服务端部署文档改为 Compose 优先的可验证单节点集群流程，补充固定镜像、随机凭据、端口保护、持久化、健康检查、日常运维和 `docker run` 备用路径。
