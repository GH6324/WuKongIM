# v2 → v3 离线迁移操作手册

适用于未经修改的 `v2.2.5-20260422`（提交
`a888f89533d0e7d1b2030e06504ca97f1ad891d4`），迁入全新的 v3 集群。
不要求升级现有 v2。迁移工具和目标 v3 服务必须使用包含本迁移实现的同一代码版本构建。

## 流程与边界

停止业务写入 → 原版 v2 收敛并排空通知 → 正常停止全部 v2 节点 →
保存完整停机备份 → `prepare` → `export` → `import` → 停机 `verify` →
隔离启动 v3 并做业务验收 → 切换流量。

单节点集群和多节点集群使用相同流程。可以改变节点数，但第一版不向已有业务数据的
v3 集群合并，不做在线增量追赶。`prepare` 是离线扫描，不是运行在 v2 内的准备接口。

源基线是原版 v2 权威节点停机后的可恢复持久化业务状态。旧版本没有提供的历史
ACK、历史多数派确认或外部 Webhook 送达证据，工具不会补造。源 Raft 状态用于
判断源是否可迁移；目标使用新的原生 Controller / Slot / Channel 初始化状态。

## 1. 准备二进制、配置与源备份

迁移工具当前支持 Linux 和 macOS 的文件锁。在本实现所在的仓库工作目录构建：

```sh
GOWORK=off go build -o /srv/tools/wkmigrate ./cmd/wkmigrate
GOWORK=off go build -o /srv/tools/wukongim-v3 ./cmd/wukongim
/srv/tools/wkmigrate --help
```

停机前记录原部署的版本凭据、所有节点 ID、数据目录、业务 DB 分片数和有效配置
（包括环境变量）。`source_commit` 是运维提供的版本证据，不是工具自动鉴定二进制。

阻断所有业务写入口，等待原版集群应用完日志、完成正在进行的拓扑变更并排空通知；
通过原有 API 留存代表性历史消息、权限、会话、CMD 和事件同步响应。然后正常停机，
关闭自动拉起，再复制每个节点的完整数据目录。不能复制仍在写入的 Pebble 目录。

收集所有节点目录到迁移机或可靠的本地挂载路径。文件锁用于拒绝仍被进程打开的
源库；工具还会在扫描前后校验文件清单。不要修改原数据来绕过检查。

**迁移归档不是完整文件系统备份。** 它保存原始数据库行、索引、管理记录、权威
选择与文件校验清单，不包含原始全部 WAL 文件字节。回滚必须保留完整停机备份。

目标运行配置需要另行核对。数据库转换不会自动验证或搬运 TLS、监听/公布地址、
网关鉴权开关、Webhook 地址/签名、插件程序、推送配置及业务方服务配置。

## 2. 固定迁移计划

示例 `/srv/wkmigrate/plan.json`（地址、路径、分片数按实际部署填写）：

```json
{
  "version": 1,
  "source_commit": "a888f89533d0e7d1b2030e06504ca97f1ad891d4",
  "sources": [
    {"node_id": 1, "data_dir": "/srv/v2-snapshots/node1", "shard_count": 8},
    {"node_id": 2, "data_dir": "/srv/v2-snapshots/node2", "shard_count": 8},
    {"node_id": 3, "data_dir": "/srv/v2-snapshots/node3", "shard_count": 8}
  ],
  "target": {
    "cluster_id": "production-v3-migrated",
    "created_at": "2026-09-06T08:00:00Z",
    "slot_count": 256,
    "hash_slot_count": 256,
    "replicas": 3,
    "channel_replicas": 3,
    "nodes": [
      {"node_id": 101, "addr": "10.20.0.11:7000", "data_dir": "/srv/wkmigrate/node101"},
      {"node_id": 102, "addr": "10.20.0.12:7000", "data_dir": "/srv/wkmigrate/node102"},
      {"node_id": 103, "addr": "10.20.0.13:7000", "data_dir": "/srv/wkmigrate/node103"}
    ]
  }
}
```

- 源 `shard_count` 是原业务 DB 的实际分片数；工具单独盘点 Slot 日志分片。
- `created_at` 是这次新集群的固定创建时间，重试时保持不变。
- 目标节点 ID 为 1–1023；端口为 1–65535；所有目标节点同时是 Controller voter 和 data node。
- `hash_slot_count` 固定 256；物理 `slot_count` 为 1–256；两类副本数均不能超过节点数。
- 单节点集群只列一个目标节点，两类副本数设为 1；仍创建正常集群运行状态。
- 所有路径必须绝对且互不包含；目标节点目录首次导入时必须不存在，父目录先创建。
- 第一版在迁移机上安装全部目标目录。复制到对应目标主机时必须保留计划中的
  **同一绝对数据路径**，包括完整 `controller`、`slotraft-snapshots` 和迁移标记。
  Controller 原生快照清单包含路径，不能只移动消息库或随意改目录。

计划内容、源目录内容和工作空间绑定。要改变计划，使用新的工作空间、归档和
全新的目标目录重新执行；不要删除完成标记来复用已有目标。

## 3. prepare：离线检查并生成转换清单

```sh
mkdir -p /srv/wkmigrate/reports
/srv/tools/wkmigrate prepare \
  --plan /srv/wkmigrate/plan.json \
  --workspace /srv/wkmigrate/scratch \
  > /srv/wkmigrate/reports/prepare.json
```

命令执行以下检查：

1. 检查节点目录、文件锁、原始表格式、业务读取所需索引、分片清单和文件校验和。
2. 对照持久化集群配置、Slot 日志及应用位置，确定源权威副本。
3. 解析所有业务身份，比较当前副本组的业务记录；冲突或无法确定权威时停止。
4. 合并原版可恢复的会话尾部意图，只补原本不存在的记录。
5. 校验消息 ID、序号、保留前缀、事件游标等映射，并在磁盘工作空间生成原生目标记录。

成功时退出码为 0，JSON `status` 为 `prepared`，包含源清单、选中/归档记录数量和
转换摘要。`cutover_ready` 仍为 `false`。此阶段不创建目标集群，不操作原 v2 服务，
也不会自动替你证明停写或外部通知送达。

当前明确阻止完成的业务边界包括：插件业务绑定、全局方法或非空有效配置缺少兼容映射、独立非零 unread
计数未证明等价、CMD 删除边界、已有历史但缺少原会话的成员可见性歧义、无状态投影
的事件游标、不能被原生接口等价表达的记录。错误必须解决后重做，不能把业务记录
改为管理归档来“通过”。普通空群、普通会话、CMD 已读位置和删除位置已有样本覆盖。

预检和后续归档重建都会核对设备、成员、会话、消息的业务读取索引及原分片，
拒绝缺失、错误指向、跨分片消息 ID 遮蔽及未知格式。原版截断留下且不改变查询结果
的消息索引可以完整归档；时间、计数等管理索引只核对格式，不宣称其历史值准确。

纯插件名称、版本、配置模板等描述可以归档；注册方法或有效配置不能归档代替迁移。
原版持久化的插件禁用标记不足以证明全局钩子不影响业务，也不能绕过预检。

通知 DB 非空或磁盘通知队列未排空会阻止迁移。即使队列深度为零，也不能据此断言
外部系统已经接收全部通知；迁移不会重放旧通知。

## 4. export：封存源归档

```sh
/srv/tools/wkmigrate export \
  --plan /srv/wkmigrate/plan.json \
  --workspace /srv/wkmigrate/scratch \
  --archive /srv/wkmigrate/archive \
  > /srv/wkmigrate/reports/export.json
```

必须沿用成功 `prepare` 的工作空间。再次检查源未改变后，发布有校验和的分块归档、
清单和完成标记。缺块、损坏、冲突或缺完成标记的归档不能导入。

归档含设备凭据、消息正文等业务数据，应按数据库备份管理访问权限。导出后可以
卸载源目录；`import` / `verify` 只从归档重建源事实，不要求源目录在线。

## 5. import：生成全新 v3 数据目录

```sh
/srv/tools/wkmigrate import \
  --plan /srv/wkmigrate/plan.json \
  --workspace /srv/wkmigrate/scratch \
  --archive /srv/wkmigrate/archive \
  > /srv/wkmigrate/reports/import.json
```

迁移机变化时，可以使用新的工作空间，从完整归档重建输入；计划保持不变。
导入按目标原生分片和副本布局写入业务数据、Controller / Slot 启动快照以及原生
消息日志。每个节点完成持久化后才发布完成标记；不完整标记的节点拒绝启动。

中断后，在目标服务始终未启动、计划和归档未变的条件下，重跑同一命令。
已完成节点会校验文件指纹后跳过，部分节点按相同记录重试。已经启动或被修改的
完成目录拒绝被覆盖。若目录创建后、身份标记落盘前发生断电，工具会保守拒绝无身份
目录；保留现场，另选全新输出目录和计划重做。

导入期间不要启动任何目标节点，也不要让自动部署系统拉起服务。

## 6. verify：独立核对全部业务数据与启动产物

**在首次启动 v3 之前**执行：

```sh
/srv/tools/wkmigrate verify \
  --plan /srv/wkmigrate/plan.json \
  --workspace /srv/wkmigrate/scratch \
  --archive /srv/wkmigrate/archive \
  > /srv/wkmigrate/reports/verify.json
```

校验器重新解码归档中的权威源记录，不使用转换器的目标行或成功计数作为预期值。
逐字段核对所有目标副本：凭据、成员/权限、会话/CMD 状态、事件投影和消息完整内容；
检查 message ID 与幂等索引、消息尾部和提交边界、完整 proposal 摘要链和成对索引、
频道运行元数据及多余/缺失记录。
同时核对 Controller WAL/快照、Slot 快照与已核对的业务元数据、整套发布文件指纹。

成功为退出码 0、`status: "offline_verified"`，报告给出源摘要、校验摘要、节点数、
消息副本数和各表副本数。逻辑消息数与所有副本之和不同，例如三副本会计三次。
任何差异都不能视为成功。启动过的库不再是这份初始生成状态，不能拿它重新执行
离线初始化校验；保留首次报告与验收前的目标副本。

`cutover_ready: false` 是有意保留的边界：离线数据相等不等于服务已通过业务验收。

## 7. 隔离启动并验收后切流

为每个节点准备 `wukongim.toml`，确保以下值与计划一致：`node.id`、`node.data_dir`、
`cluster.id`、`cluster.nodes`、`cluster.initial_slot_count`、`cluster.hash_slot_count`、
`cluster.slot_replica_n`、`cluster.channel_replica_n`。这些启动参数受迁移标记校验。
保留初始 bootstrap 配置；后续拓扑调整遵循 v3 原生集群流程。

在不接生产流量的环境执行，至少保存这些证据：

| 验收项 | 通过标准 |
| --- | --- |
| 集群健康 | 所有节点就绪，Controller 与每个 Slot 有实际 Leader，副本/任务收敛 |
| 原凭据 | 用原 token 和相同 device_flag 登录成功，错误 token 拒绝 |
| 历史数据 | API 分页读取首尾和跨页消息，ID/seq/正文/协议字段与源基线一致 |
| 权限和会话 | 成员/黑白名单/禁言行为正确；空群、删除会话、已读和 CMD ACK 符合基线 |
| 事件 | `/message/eventsync` 的投影、顺序、游标、私有过滤符合源基线 |
| 新写入 | 新消息 ID 不与旧记录冲突，seq 超过原尾部；同幂等键重试不重复 |
| 重启与故障 | 重启后结果不变；多节点集群丢失一个 Leader 后恢复读写且历史不丢 |

自动黑盒矩阵使用未经修改原版 v2 的停机 fixture，实际运行四个 CLI 命令、v3 进程、
HTTP 和仓库 Go WKProto 客户端，覆盖 1→1、1→3、3→1、3→3、3→5。它不能代替你的 SDK
版本、插件、Webhook 和生产配置验收。测试写入应使用隔离演练副本或明确的测试账号。

切流由运维在所有报告与业务验收通过后执行。切流前源备份完整、v3 无新业务写入时
可恢复原 v2；v3 已接受业务写入后，不能直接切回旧 v2，否则会丢失新写入。
第一版不提供反向增量迁移。

## 资源与未验证项

转换使用磁盘工作空间和有界批处理；仍需要同时容纳源备份、归档、工作空间及目标
全部副本。原生 Slot Raft 快照桥接当前上限为每个物理 Slot **256 MiB 元数据**，超出
会停止而非无限扩张内存。事件同步每页保留投影上限为 8 MiB。
迁入消息按原生恢复预算分批，每个不可拆分 proposal 的原生编码与传输计费均不超过
1 MiB；单条消息若超过该预算，会在预检拒绝，不能仅凭 payload 大小判断是否可迁。

100 GiB 源业务数据、停写至完成校验 ≤4 小时是约定的性能验收目标，**尚未执行**。
在测试环境提供前，本机小样本测试只证明对应功能，不构成该吞吐或停机时长承诺。
