# 中心-自治节点视频接入与管理平台设计基线

- 文档状态：已确认的架构基线（合并版）
- 适用范围：GB28181/RTSP 视频接入、节点自治、中心统一管理、多租户和节点横向扩展
- 核心部署：Docker / Docker Compose
- 控制面：Go
- 媒体面：ZLMediaKit
- 数据库：PostgreSQL
- 运行状态：Redis
- SIP 接入面：Kamailio 6.1.3（固定版本源码构建 + 自定义 gb28181 模块）

## 0. 文档来源与冲突裁决记录

本文档是仓库内全部设计材料的合并基线。原始讨论、分阶段设计、目录架构设计均归档为参考；以本文档为唯一现行设计，冲突处按最近一次用户决策为准。

| 时间 | 来源 | 内容 | 状态 |
|---|---|---|---|
| 2026-08-14 | `references/original/chatgpt-project-architecture-discussion-2026-08-14.md` | 早期方案构想：100 万设备、Cell/Shard、Kafka、Kubernetes、ClickHouse、Go+C++ | 历史参考，已否决其中与联邦式节点架构冲突的部分 |
| 2026-08-15 | 阶段一自治 Node 设计（会话归档） | 联邦式中心+自治节点架构基线、Go 模块化单体、Docker Compose、设备/通道/流/会话分层 | 已并入本文档主体 |
| 2026-08-16 | 后端目录架构设计（会话归档） | 按部署进程划分的模块化单体目录、依赖规则、契约边界 | 已并入 §12.1 |
| 2026-08-17 | 单自治节点接入闭环设计（会话归档） | Kamailio 阶段一标准化、Go node-access 协议适配器、gRPC `api/access/v1` | 部分被 2026-08-18 取代（node-access 与契约形态） |
| 2026-08-18（最新） | Kamailio node-access 第一阶段设计（会话归档） | **Kamailio 6.1.3 取代 Go node-access**；JSON-RPC over HTTP 取代 gRPC；Redis ACL 隔离 + AOF；PostgreSQL outbox；事件 poll/ack/snapshot | **现行决策，冲突时以此为准** |

冲突裁决原则：任何与 2026-08-18 设计冲突的内容，一律以 2026-08-18 为准。

关键冲突裁决摘要：

1. **node-access 运行进程**：Go node-access（08-15/08-17）→ **Kamailio 6.1.3 + 自定义 gb28181 C 模块**（08-18）。Go `cmd/node-access` 和 `internal/nodeaccess` 只有 HTTP 健康检查骨架，没有 SIP/GB28181/Redis 能力，无目标角色，不保留 Go Access 适配器。
2. **Node App ↔ Node Access 契约**：gRPC/Protobuf `api/access/v1`（08-15/08-16/08-17）→ **JSON-RPC 2.0 over HTTP**（08-18），因为 Kamailio 的 RPC 模型天然承载 JSON-RPC，不需要在 C 中嵌入 gRPC。
3. **Kamailio 定位**：仅在多接入实例时启用的 SIP 前置代理（08-15）→ **阶段一即作为 node-access 的唯一运行进程**（08-17/08-18）。
4. **Redis**：共享密码、易失（08-15）→ **ACL 隔离用户 + 私有 key 命名空间 + AOF 持久化**（08-18）。
5. **设备凭据**：明文密码不入库（08-15）→ **只存 MD5 HA1，profile 经 outbox 最终一致同步**（08-18）。
6. **阶段一范围**：含 PTZ（08-15）→ **第一阶段首个切片仅 REGISTER/Digest/KeepAlive/事件；PTZ 移出阶段一**（08-17 D2 / 08-18）。
7. **集成契约**：Access 事件经 gRPC 流订阅（08-17）→ **Redis Streams 非阻塞 poll + 单调 ACK + 全量 snapshot 校准**（08-18）。

## 1. 设计目标

平台采用“中心管理平台 + 自治节点平台”的联邦式架构。

每个自治节点本身是一套完整的小型视频平台，能够独立完成：

- 本地用户和权限管理；
- 租户管理；
- 设备与通道管理；
- GB28181/RTSP 接入；
- 实时视频、PTZ、录像和回放；
- 告警、审计和本地运维；
- 本地数据库和媒体服务。

中心平台负责统一纳管多个自治节点，提供：

- 节点注册、认证和授权；
- 全局租户目录；
- 可选的区域、地市和项目分组；
- 设备和通道轻量索引；
- 统计、告警和节点健康汇总；
- 跨节点检索；
- 授权后的远程业务操作；
- 全局运维和中心审计。

中心不是所有节点业务数据的唯一数据库。节点是本地业务数据的权威源，中心保存全局目录和管理投影。

## 2. 核心原则

1. **节点自治**：中心断开时，节点本地接入、播放、录像、告警和本地管理继续运行。
2. **中心分管**：中心通过私有管理协议统一纳管节点，但不绕过节点业务服务直接写数据库。
3. **双向授权**：中心管理员拥有权限且子节点向中心授予相应能力时，才允许远程操作。
4. **本地权威**：节点负责本地业务数据的最终写入和校验。
5. **中心投影**：中心同步目录、索引、摘要和事件，不复制所有本地细节。
6. **逻辑分级、通信扁平**：中心和节点直接连接；地市、区域、行业只是管理属性，不写死为固定行政层级。
7. **小节点可独立运行，大节点按需扩展**：一个节点可以部署在一台服务器，也可以扩展到多台服务器。
8. **业务单体、基础设施分工**：业务控制面保持模块化单体；接入面和媒体面按独立资源边界运行。
9. **最小必要设计**：不提前引入 Kubernetes、Kafka、服务网格、通用工作流引擎或复杂策略语言。
10. **Docker 标准部署**：所有组件使用 Docker，单机使用 Docker Compose，多机仍使用标准 Compose 部署，不依赖 Kubernetes。
11. **协议面单一权威**：SIP 协议状态由 Kamailio（含自定义模块）独占持有，业务面通过版本化 Access API 交互，不共享内部 Redis key。
12. **提交先于同步**：设备业务事务先在 node-app PostgreSQL 提交，再经 outbox 异步同步至 Access；同步失败不回滚业务提交。

## 3. 通用拓扑

```text
                         Center
                ┌───────────────────────────┐
                │ Platform IAM              │
                │ Node Registry             │
                │ Global Tenant Directory   │
                │ Region / Group            │
                │ Resource Projection       │
                │ Statistics / Alarm        │
                │ Remote Operation          │
                │ Central Audit             │
                └─────────────┬─────────────┘
                              │
                       mTLS Management API
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
       Node A              Node B              Node C
     多租户共享            租户专属              多租户共享
          │                   │                   │
     完整节点平台        完整节点平台        完整节点平台
```

节点名称可以使用“福州市”“厦门市”“某租户专属节点”等显示名称，但核心身份使用不可变的 `node_id`。节点不等于地市，也不要求一个地市只有一个节点。

节点属性可以包括：

```text
node_id
name
region_id              # 可选
node_type              # shared / dedicated
owner_tenant_id        # 专属节点可填写
status
version
last_seen_at
```

## 4. 节点与租户关系

支持以下关系：

```text
一个节点 -> 一个租户
一个节点 -> 多个租户
一个租户 -> 一个节点
一个大型租户 -> 多个节点分区
```

不在租户表中直接固定单个 `node_id`，使用绑定关系表达：

```text
NodeTenantBinding
├── center_tenant_id
├── node_id
├── local_tenant_id
├── partition_key
├── binding_type
└── status
```

典型部署：

```text
共享节点
├── Tenant A
├── Tenant B
└── Tenant C

专属节点
└── Tenant D
```

大型租户可以按区域、项目、设备组或容量分区：

```text
Tenant E
├── 分区 01 -> Node 01
├── 分区 02 -> Node 02
└── 分区 03 -> Node 03
```

首期优先采用明确的区域或业务分区，不实现任意设备的自动跨节点迁移。需要更高容量时，增加节点或节点内部实例，并由中心聚合成一个逻辑租户。

## 5. 自治节点内部结构

### 5.1 最小节点

```text
Node
├── node-app
├── node-access（Kamailio 6.1.3 + gb28181 模块）
├── zlmediakit
├── postgres
├── redis
├── node-web
└── reverse-proxy
```

一个小节点可以把这些容器部署在一台服务器上，目标容量约为 1 万台设备。最终容量必须通过目标设备、KeepAlive 周期、活跃流、录像和码率压测确认。

### 5.2 扩展节点

```text
Logical Node
├── node-app
├── node-access-01
├── node-access-02
├── node-access-03
├── zlmediakit-01
├── zlmediakit-02
├── postgres
├── redis
└── sip-edge（需要多接入实例时启用）
```

扩容方向可以独立：

- SIP/设备接入压力高：增加 `node-access`；
- 媒体带宽或流数高：增加 ZLMediaKit；
- 管理接口压力高：增加 `node-app` 实例；
- 数据库和 Redis 按可用性需求做主备或高可用。

节点内部不拆成 Device Service、PTZ Service、Alarm Service、Record Service 等大量微服务。

## 6. 服务职责

### 6.1 `node-app`：节点业务管理面

`node-app` 是 Go 模块化单体，负责：

- 本地用户与角色；
- 租户；
- 组织和资源；
- 设备与通道业务模型；
- 本地权限；
- 流生命周期和媒体编排；
- 录像索引；
- 告警处理；
- 审计；
- 节点运维；
- Center Connector；
- 远程命令的本地授权和执行入口；
- 设备接入 profile 的 PostgreSQL 权威与 outbox 同步；
- Access 事件轮询、运行时投影与全量校准。

### 6.2 `node-access`：SIP/GB28181 协议接入面

`node-access` 是固定的 Kamailio 6.1.3 源码构建 + 独立 `gb28181` C 模块，不是 Go 服务。Compose 服务名保持 `node-access`，但运行镜像为 Kamailio。

构建方式：以官方 `kamailio/kamailio` 6.1.3 标签源码构建，只启用所需官方模块（`auth`、`pv`、`registrar`、`sl`、`tm`、`usrloc`、`xhttp`）和本项目自定义 `gb28181` 模块；不修改 Kamailio Core。

它负责：

- SIP UDP/TCP 接收（阶段一首期仅 UDP）；
- REGISTER 和注册有效期；
- Digest 认证（MD5，qop=auth 与 no-qop，nonce 5 分钟过期）；
- KeepAlive XML 校验与本地状态维护；
- SIP Transaction 和 Dialog；
- Call-ID、CSeq、Tag 等协议状态；
- GB28181 profile 的 Redis 缓存与版本管理；
- 运行事件生产（Redis Streams）与 JSON-RPC poll/ack/snapshot；
- 设备注册会话的运行时状态。

`node-access` 不负责：

- 用户是否有播放权限；
- 租户和区域授权；
- 告警业务规则；
- 录像生命周期；
- Center 的全局权限；
- 媒体数据处理；
- 设备业务数据权威（PostgreSQL 只属于 node-app）。

边界原则：

> `node-app` 通过版本化 Access API 表达业务意图（profile 同步、快照查询、事件消费），Kamailio 自定义模块负责 SIP/GB28181 协议语义转换。

当前首个切片已实现的协议能力：

```text
node-app: access.v1.applyDeviceProfile / removeDeviceProfile / replaceDeviceProfiles
node-access: REGISTER + Digest MD5 + KeepAlive XML + 运行事件
```

尚未实现（后续切片）：Catalog、DeviceInfo、DeviceStatus、Alarm、RecordInfo、PTZ、INVITE/ACK/BYE 和媒体行为。

### 6.3 Kamailio 定位与多实例

Kamailio 同时是 node-access 运行进程和 SIP 前置入口，不再存在"Go node-access 之后的第二个 Kamailio 代理"。

- 单实例阶段：摄像头直接连接 `node-access`（Kamailio）的稳定 SIP 地址；
- 多实例阶段（阶段三）：多个 Kamailio 实例需要统一 SIP 入口时，通过 L4/SIP VIP 或负载均衡分发，Kamailio 仍只负责 SIP 语义路由和入口保护；
- 多实例时每个实例是独立 `node-access`，运行数据在各自 Redis 命名空间，通过事件模型预留给 `access_instance_id`/`session_epoch` 表达。

### 6.4 ZLMediaKit：媒体数据面

ZLMediaKit独立运行，负责：

- RTP/PS；
- RTSP；
- WebRTC；
- HLS等协议输出；
- Remux和协议转换；
- 录像；
- 媒体分发。

`node-app`负责媒体编排，Kamailio负责SIP信令，ZLMediaKit负责媒体数据。

媒体流不默认经过 Center，也不经过 Kamailio。

## 7. SIP和媒体调用流程

### 7.1 设备注册（当前切片）

```text
Camera
  -> node-access（Kamailio：REGISTER/Digest 校验/registrar 绑定）
  -> Redis：运行注册状态（nv:access:v1:*）
  -> node-app：事件轮询 -> 运行时投影（nv:nodeapp:v1:*）
```

### 7.2 设备注册（多实例目标形态）

```text
Camera
  -> Kamailio SIP 入口（多实例时的统一入口）
  -> node-access（gb28181 模块）
  -> Redis：运行时注册状态
  -> node-app：设备状态投影和事件
```

### 7.3 播放（目标流程，后续切片）

```text
用户
  -> node-app：权限检查
  -> node-app：选择媒体节点并创建流任务
  -> node-access：请求设备发送流（INVITE + SDP）
  -> Kamailio：代理 INVITE（多实例时）
  -> Camera：RTP
  -> ZLMediaKit：接收RTP并输出WebRTC
  -> Browser
```

### 7.4 PTZ（后续阶段）

```text
用户
  -> node-app：检查 ptz:control
  -> node-access：生成PTZ XML和SIP MESSAGE
  -> Kamailio：代理（多实例时）
  -> Camera
```

## 8. 数据权威与同步

### 8.1 Node权威数据

节点 PostgreSQL 保存：

- 本地租户；
- 本地用户和角色；
- 设备和通道；
- 协议配置（接入 profile、HA1、启用状态、profile 版本）；
- 录像索引；
- 告警；
- 本地审计；
- 节点配置；
- Access profile outbox（同步意图）。

节点是本地业务数据的权威源。

### 8.2 Center数据

中心保存：

- 节点目录；
- 节点版本和状态；
- 全局租户目录；
- Node 与租户映射；
- 平台用户和区域管理范围；
- 设备和通道轻量索引；
- 统计摘要；
- 告警摘要；
- 中心操作审计；
- 同步游标和远程命令记录。

中心不保存节点的全部业务细节。设备详情、完整录像索引、原始告警和诊断数据按需从节点查询。

### 8.3 同步方式

采用应用层同步，不做节点数据库与中心数据库的双向复制。

```text
Node -> Center
├── RegisterNode
├── Heartbeat
├── PushResourceChanges
├── PushStatistics
├── PushAlarms
└── ReportCommandResult

Center -> Node
├── QueryResourceDetail
├── UpdateDevice
├── RequestCatalogSync
├── RequestPlayback
├── StopPlayback
├── ControlPTZ
└── QueryRecords
```

同步事件包含最小必要字段：

```text
event_id
node_id
sequence
event_type
entity_type
entity_id
entity_version
occurred_at
payload
```

Node 使用 PostgreSQL Outbox 保存待发送事件，Center 以 `event_id` 和序号幂等处理。首期不引入 Kafka、NATS 或数据库双向复制。

### 8.4 Node App ↔ Node Access 同步（当前切片）

设备业务变化在 node-app PostgreSQL 事务内提交，同时写入 `access_profile_outbox`。后台 worker 通过 Access JSON-RPC 将 profile 同步到 Kamailio：

- 设备创建/启停用 → 事务内生成 outbox 行 → `access.v1.applyDeviceProfile`；
- 同版本内容冲突或 Access 重连 → `access.v1.replaceDeviceProfiles` 全量校准；
- Access 不可用时设备保持 `access_sync_status=pending`，恢复后自动重试/全量校准；
- 同步成功后设置 `access_synced_version`，多个旧 outbox 行一次标记处理。

同步分三层：

1. 增量事件：设备、通道、告警和状态变化；
2. 周期摘要：节点、租户、在线率、容量和媒体指标；
3. 全量校准：定期按版本和分页检查中心投影；Access 侧按 profile 版本和 generation 校准。

## 9. 中心授权管理

中心可以直接操作子节点业务，但不能直接连接子节点数据库。所有远程修改必须通过节点服务执行。

```text
Center Admin
  -> Center 权限检查
  -> Node CenterGrant 检查
  -> node-app 本地业务服务
  -> Node PostgreSQL 事务
  -> Node 本地审计
  -> Center 集中审计
```

### 9.1 用户作用域

```text
Center User
├── platform_admin
├── region_admin
├── central_operator
├── security_admin
└── auditor

Node Local User
├── node_admin
├── tenant_admin
├── operator
└── viewer
```

普通节点用户只能属于一个租户。中心和节点用户不强制同步成相同账号。

中心操作通过委托身份传给节点：

```text
center_id
center_user_id
requested_action
target_tenant
target_resource
operation_id
issued_at
expires_at
```

### 9.2 Node授权

节点通过 `CenterGrant` 授予中心固定权限组：

```text
只读监控
├── 查看节点状态
├── 查看租户摘要
├── 查看设备状态
└── 查看告警摘要

运维管理
├── 设备诊断
├── 触发目录同步
├── 重启流
└── 节点诊断

业务管理
├── 修改设备
├── 修改通道
├── 配置录像
└── 处理告警

媒体访问
├── 实时播放
├── PTZ
├── 录像查看
└── 录像下载
```

不使用通用 ABAC 策略语言。首期采用固定权限点 + Center 管理范围 + Node 授权模板。

### 9.3 区域和节点

区域、地市、项目只是可选管理维度，不绑定福建或固定九地市。

设备可以同时拥有：

```text
tenant_id
organization_id
region_id
node_id
```

区域管理员可跨租户管理其区域内的节点和资源，但默认不拥有实时视频、PTZ、录像查看和下载权限。

## 10. Redis和PostgreSQL边界

### PostgreSQL

保存持久业务数据：

- 租户；
- 用户；
- 角色；
- 设备（业务记录 + 接入 profile）；
- 通道；
- 配置；
- 录像索引；
- 告警；
- 审计；
- 节点授权；
- Access profile outbox。

### Redis

作为标准部署组件，从第一版开始使用。Redis 启用 AOF（`appendfsync everysec`）和命名卷，普通 `compose down` 保留 profile 和未确认事件；profile 与事件丢失窗口受 Redis 持久化窗口限制。

Redis 使用 ACL 用户隔离，两个运行身份只能访问各自的私有命名空间：

- Kamailio（node-access）：`nv:access:v1:*`；
- node-app：`nv:nodeapp:v1:*`；
- admin/health 用户：受限运维命令，不参与运行时访问。

Kamailio keys：

```text
nv:access:v1:active-generation
nv:access:v1:profile:{device_access_id}
nv:access:v1:registration:{device_access_id}
nv:access:v1:event-sequence
nv:access:v1:events
nv:access:v1:acked:node-app
nv:access:v1:instance-id
nv:access:v1:session-epoch
```

NodeApp keys：

```text
nv:nodeapp:v1:device-runtime:{business_device_id}
nv:nodeapp:v1:access-cursor:{access_instance_id}
nv:nodeapp:v1:access-snapshot:{access_instance_id}
```

职责划分：

- 设备在线状态；
- `device_id -> node-access instance`；
- `stream_id -> media instance`；
- Access 和 Media 实例心跳；
- 短期命令状态；
- 限流；
- 有限分布式锁。

Redis不是业务数据库，也不作为完整SIP Dialog的唯一持久副本。SIP Transaction/Dialog主要由所属 Kamailio 实例持有，实例故障后允许设备重新注册和流会话重建。Kamailio 进程每次启动生成新的 `session_epoch`；旧 epoch 的注册在设备重新注册或发送有效 KeepAlive 前不作为当前状态。

## 11. Docker部署

不使用 Kubernetes 或 Docker Swarm。统一使用 Docker 和 Docker Compose。

### Center Compose

```text
center-app
center-postgres
center-redis
center-web
reverse-proxy
```

### 标准小型Node Compose

```text
node-app
node-access（Kamailio 6.1.3 + gb28181 模块）
postgres
redis
zlmediakit
node-web
reverse-proxy
```

单个 `node-access` 时，摄像头可以直接连接 `node-access` 的稳定SIP地址。

### 扩展Node

```text
node-app
node-access-01..N
zlmediakit-01..N
postgres
redis
node-web
reverse-proxy
```

### 超大Node

```text
SIP VIP / L4入口
├── Kamailio-01
└── Kamailio-02
        │
        ▼
node-access-01..N
```

L4入口只负责把网络流量分配给 Kamailio，不替代SIP语义路由。可使用现有云/硬件四层负载均衡，或在Docker环境中使用Nginx Stream、LVS/IPVS等经过压测验证的方案。

## 12. 技术选型

| 层 | 选择 |
|---|---|
| Center后端 | Go模块化单体 |
| Node后端 | Go模块化单体 |
| 前端 | Vue 3 + TypeScript |
| 持久数据库 | PostgreSQL |
| 运行状态 | Redis（ACL 隔离 + AOF） |
| SIP/GB28181接入 | Kamailio 6.1.3（固定源码构建 + 自定义 gb28181 C 模块）；OpenSIPS仅作为PoC备选 |
| GB28181业务 | Kamailio gb28181 C 模块（不再由 Go node-access 实现） |
| 媒体 | ZLMediaKit |
| 录像/转码 | ZLMediaKit，必要时接入FFmpeg |
| Node App ↔ Node Access 契约 | 版本化 JSON-RPC 2.0 over HTTP（`access.v1.*`） |
| Center ↔ Node 契约 | 版本化Protobuf + mTLS gRPC（阶段二，未来） |
| 部署 | Docker Compose |
| 指标 | Prometheus格式 |
| 日志 | 结构化日志 |
| 许可证约束 | Kamailio GPLv2+：本项目当前作为学习项目使用；任何对外分发需单独复核许可证义务 |

Rust和Supabase不是当前已确定的技术选型。Rust可以作为未来特定组件候选，但平台控制面默认采用Go。Supabase不作为节点自治架构的基础数据库；节点使用PostgreSQL，以满足私有化、离线和自治部署。

### 12.1 后端代码目录基线

后端采用标准 Go 多进程单模块布局。架构按部署进程划分，进程内部按业务能力渐进组织；目录只表达已经存在的进程、协议和业务，不为未来功能创建空包，也不增加 `backend/`、`src/` 或全局 `controller/service/repository/domain` 分层。

这是一套面向 1-3 人团队的模块化单体结构，不以形式上的“最新架构”替代当前需求：

- `node-app` 是 Go 进程；`node-access` 是 Kamailio 进程（不再有 Go Access 目标角色）；
- 未来 `center-app` 与 Node 位于同一仓库、同一 Go module，但独立构建和部署；
- 进程内部先用同包文件表达职责，满足拆包条件后才形成业务能力子包；
- 跨进程只依赖版本化契约：Access 面 JSON-RPC over HTTP，Center 面 Protobuf/gRPC；不共享内部 Go 类型、数据库模型或 Redis 私有 key；
- 当前代码已经足够扁平，不为应用本节规则而搬迁源码。

#### 12.1.1 当前物理目录

```text
.
├── cmd/
│   └── node-app/
│       └── main.go
├── internal/
│   ├── nodeapp/
│   │   ├── app.go
│   │   ├── config.go
│   │   ├── health.go
│   │   ├── devices.go            # 设备业务权威 + HA1 派生
│   │   ├── device_http.go        # 内部设备 API（/internal/v1/devices）
│   │   ├── access.go             # Access JSON-RPC client + 契约类型
│   │   ├── sync.go               # outbox worker + profile 同步
│   │   ├── projection.go         # 事件轮询 + 运行时投影
│   │   └── *_test.go
│   └── platform/
│       ├── config.go
│       ├── http.go
│       ├── metrics.go
│       └── *_test.go
├── deploy/
│   ├── kamailio/
│   │   ├── kamailio.cfg
│   │   └── modules/gb28181/      # 自定义 C 模块（profile/digest/registration/keepalive/events/rpc）
│   ├── node-access/Dockerfile    # Kamailio 6.1.3 源码构建
│   ├── node-app/Dockerfile
│   ├── node-web/Dockerfile
│   ├── nginx/Dockerfile
│   └── migrate/
├── migrations/
│   ├── 000001_baseline.*
│   └── 000002_devices.*          # devices + access_profile_outbox
├── web/
├── compose.yaml
├── go.mod
└── go.sum
```

Go `cmd/node-access` 与 `internal/nodeaccess` 残留骨架已移除，仓库中不再存在无目标角色的 Go Access 运行进程；node-access 运行进程唯一是 Kamailio 6.1.3 + 自定义 gb28181 模块。当前不创建 `api/access/v1`、`api/federation/v1`、`cmd/center-app`、`internal/centerapp`、业务能力空包或 `tests/system`。这些目录只有在对应真实代码、协议或测试场景出现时才创建。

#### 12.1.2 演进蓝图

下面是未来能力出现后的目标形态，不是现在预建目录的要求：

```text
.
├── api/
│   └── federation/v1/                # Center <-> Node（阶段二开始时创建）
│       ├── federation.proto
│       ├── federation.pb.go
│       └── federation_grpc.pb.go
├── cmd/
│   ├── node-app/
│   └── center-app/                   # 阶段二开始时创建
├── internal/
│   ├── nodeapp/
│   │   ├── app.go
│   │   ├── config.go
│   │   ├── health.go
│   │   ├── device.go                 # 初期设备业务主流程
│   │   ├── device_postgres.go        # 真实持久化出现时
│   │   ├── access_client.go          # Access JSON-RPC 客户端
│   │   ├── federation_client.go      # Center连接出现时
│   │   └── device/                   # 达到拆包条件后才创建
│   ├── centerapp/                    # 阶段二开始时创建
│   └── platform/
├── deploy/kamailio/modules/gb28181/  # 随协议能力扩展
├── migrations/                       # 只有Node时继续平铺
└── tests/system/                     # 真实跨进程场景出现时
```

注意：`api/access/v1` 不再出现在蓝图。Node App 与 Node Access 的跨进程契约是版本化 JSON-RPC over HTTP（见 §12.1.5），由 Kamailio 自定义模块直接承载，不需要独立的 `.proto` 契约与生成代码目录。

#### 12.1.3 目录职责

| 目录 | 职责 | 禁止内容 |
|---|---|---|
| `cmd/<process>` | 读取配置、创建日志和退出信号、组装依赖、启动并关闭 HTTP/gRPC 服务 | 业务规则、SQL、HTTP处理细节、SIP/GB28181实现 |
| `internal/nodeapp` | Node业务管理面和本地业务数据权威；用户、租户、设备、通道、权限、媒体编排、录像索引、告警、审计、运维、Access JSON-RPC 客户端、事件投影及Center Connector | SIP/GB28181报文实现、协议会话状态、Access私有Redis读取、Center全局业务 |
| `deploy/kamailio/modules/gb28181` | SIP/GB28181 协议接入；REGISTER/Digest/KeepAlive、设备 profile、运行事件、JSON-RPC 控制面 | 用户/租户授权、业务PostgreSQL权威写入、告警规则、录像生命周期、媒体数据处理 |
| `internal/centerapp` | 未来Center管理面；节点目录、全局租户映射、区域权限、轻量资源索引、同步游标、远程命令和中心审计 | 直连Node数据库、保存Node完整业务明细、直接操作Access协议状态 |
| `internal/platform` | 至少被两个进程稳定复用的无业务运行能力，如配置解析、HTTP生命周期、日志和指标 | 业务模型、协议实现、数据库Repository、单进程专用代码、`common/utils`杂物 |
| `api/federation/v1` | 未来Center↔Node 跨进程`.proto`源文件和生成的Go契约代码 | 业务实现、数据库模型、Redis key、应用内部helper |
| `migrations` | 当前Node PostgreSQL的显式迁移；应用启动不自动修改数据库结构 | Center表、运行时自动建表逻辑 |
| `tests/system` | 真实跨进程、Compose或端到端测试 | 可以由包级单元测试覆盖的逻辑、占位测试 |

`api/federation/v1` 中的生成文件提交仓库。普通开发和 `go test ./...` 不依赖本地安装 `protoc`；CI 使用固定版本重新生成并检查无差异。发布后的 Protobuf field number 不复用，兼容新增留在 `v1`，破坏性变化创建 `v2`。

#### 12.1.4 依赖规则

当前依赖保持为：

```text
cmd/node-app -> internal/nodeapp -> internal/platform
```

未来依赖扩展为：

```text
cmd/node-app    -> internal/nodeapp -> internal/platform
                                      -> api/federation/v1

cmd/center-app  -> internal/centerapp  -> internal/platform
                                          -> api/federation/v1
```

必须遵守以下规则：

1. `cmd` 只负责进程组装和生命周期，不被其他包导入。
2. `nodeapp`、`centerapp` 不直接引用彼此内部实现。
3. 跨进程通信只通过版本化契约：Node App ↔ Node Access 使用 `access.v1` JSON-RPC over HTTP；Center ↔ Node 使用 `api/federation/v1` gRPC；不共享数据库模型、Redis私有key或Go领域实体。
4. 业务子包不得反向依赖所属应用的组装包，不允许循环依赖。
5. gRPC client/server adapter 放在所属进程包内，不为每个实现机械创建 `adapter/service/repository` 层。
6. 数据库访问实现归数据权威所有者；不能因为另一个进程需要数据就抽到公共包。
7. 少量局部重复优于错误的公共抽象；只有语义、变化原因和复用关系都稳定时才进入 `platform`。
8. 接口只用于真实外部边界、替换需求或必要测试替身，不为每个Service或Repository机械创建接口。
9. Access JSON-RPC client 实现归 node-app；Access JSON-RPC server 实现归 Kamailio `gb28181` 模块；不建立独立的 Go Access 服务进程。

#### 12.1.5 Node App与Node Access契约

契约形态：**版本化 JSON-RPC 2.0 over HTTP**，方法名为 `access.v1.*`，由 Kamailio `gb28181` 模块通过 xhttp 承载。

连接方式固定为：

- Node Access（Kamailio）只开放一个 HTTP `/rpc` 入口，仅 Compose 内部网络可达，不发布到宿主机；
- Node App 作为客户端主动发起 JSON-RPC 调用；
- Node App 通过该连接同步设备 profile、查询运行快照并轮询事件；
- 不为 Access 回调开放第二个 Node App 服务端入口；
- Access API 不实现 TLS/mTLS 和公网暴露（当前切片）。

契约包含两类能力：

1. **Profile 同步（语义化命令）**：`access.v1.applyDeviceProfile`、`access.v1.removeDeviceProfile`、`access.v1.replaceDeviceProfiles`。node-app 发送业务意图（设备接入标识、SIP 用户名、Realm、MD5 HA1、启用状态、单调版本）；Access 返回 `applied`/`unchanged`/`stale` 结果，同版本不同内容返回冲突错误且不改变已存 profile。请求携带 `request_id` 幂等标识。
2. **运行事件与快照**：`access.v1.pollEvents`（非阻塞，`after_sequence` + `limit` 1~500）、`access.v1.ackEvents`（单调、幂等）、`access.v1.getRuntimeSnapshot`（全量当前注册状态）。Access 发送注册、上线、下线、KeepAlive 超时等运行事实，不携带 node-app 的权限结论。

Access 事件模型采用"Redis Streams + 游标确认 + 快照校准"：

- 事件以单调递增 sequence 写入 Redis Stream（`nv:access:v1:*`）；
- node-app 每秒轮询，返回满页立即再查；
- node-app 按 `(access_instance_id, event_id)` 幂等处理，只写自己的 Redis 命名空间，然后 ACK 最高连续处理序号；失败事件不 ACK；
- 序号缺口或 `session_epoch` 变化将投影标记为过期，获取运行时快照、替换投影、存储新游标后恢复轮询；
- ACK 前事件不从 Kamailio 的事件缓冲删除；
- 校准完成前，不把旧在线状态当作新的实时事实；
- node-app PostgreSQL 仍是设备业务数据权威，Access 事件和快照只是协议运行投影。

#### 12.1.6 Center与Node契约

第一条Center/Node管理调用出现时创建 `api/federation/v1`：

- Node主动向Center建立mTLS gRPC长连接，适应私有网络、NAT和短暂离线；
- 同一连接承载Node注册、心跳、摘要和事件上报、Center远程命令；
- Node离线时继续自治运行，重新连接后按游标和全量校准恢复Center投影；
- Center不直接连接Node PostgreSQL、Redis或ZLMediaKit。

Node业务变化通过PostgreSQL Outbox至少一次发送。事件包含`event_id`、`node_id`、单调`sequence`、实体版本和发生时间，Center按`event_id`幂等处理。Center命令携带`command_id`和委托身份，Node App在本地权限和事务边界内执行；重复命令不得产生重复业务副作用。

#### 12.1.7 数据所有权

| 数据 | 权威所有者 | 其他模块获取方式 |
|---|---|---|
| Node用户、租户、设备、通道、权限、录像索引、告警、审计 | `node-app` PostgreSQL | Node App内部调用；Center通过federation RPC |
| SIP事务、Dialog、设备连接、KeepAlive、在线会话 | Kamailio `gb28181` 模块运行时 | Access事件和全量快照 |
| Access协议运行状态（profile、注册、事件缓冲） | Kamailio `gb28181` 模块的 `nv:access:v1:*` Redis | 不共享key，只使用Access JSON-RPC |
| Node App业务缓存、游标与投影 | `node-app` 自有 `nv:nodeapp:v1:*` Redis | 不读取Access私有key |
| Center节点目录、全局索引、同步游标、中心审计 | `center-app` PostgreSQL | Center内部调用；Node通过federation RPC |
| RTP/PS/RTSP/WebRTC媒体流 | ZLMediaKit | Node App编排，Kamailio发信令，不共享媒体数据库 |

当前 `migrations` 只服务Node PostgreSQL。Center出现第一条真实数据库迁移时，再拆为 `migrations/node` 和 `migrations/center`。

#### 12.1.8 业务拆包标准

业务代码默认先在所属进程根包内按能力使用文件表达，例如：

```text
internal/nodeapp/
├── app.go
├── config.go
├── health.go
├── device.go
└── device_postgres.go
```

一项业务只有同时满足以下条件才拆为子包：

1. 已经存在真实业务代码；
2. 具有独立且可命名的业务规则；
3. 具有清晰的数据边界；
4. 具有可独立验证的测试边界；
5. 继续留在根包已经明显影响阅读或维护。

只满足“代码量变大”“未来可能复用”“名称看起来像领域”或“形式更整齐”不构成拆包理由。`auth`、`tenant`、`device`、`media`、`recording`、`alarm`、`audit`、`federation`、`ops`只是可能的能力名称，不是必须预建的固定目录。

#### 12.1.9 演进顺序与测试

首个真实业务切片是设备注册与目录，按以下顺序演进：

1. 建立 Redis ACL 隔离和 AOF 持久化基线。
2. 建立设备业务权威：PostgreSQL `devices` + `access_profile_outbox` 迁移、HA1 派生、内部设备 API。
3. 将 node-access 替换为固定 Kamailio 6.1.3 构建，提供 profile JSON-RPC 与健康端点。
4. 贯通 node-app profile outbox 同步（增量 + 全量 replace + 重试/合并）。
5. 实现 REGISTER 与 Digest MD5（UDP、qop=auth 与 no-qop、nonce 重放防护、未知/禁用/错误凭据拒绝、Expires 0 注销）。
6. 实现 KeepAlive、事件流与 node-app 状态投影（非阻塞 poll、单调 ACK、缺口/epoch 快照校准）。
7. 完成设备软禁用协议效果（同步后清除注册、上报一次 offline、拒绝后续 REGISTER）。
8. 建立 Compose 系统测试与故障恢复验收（合成 SIP 客户端，不依赖真实摄像头）。
9. 更新运行说明并执行静态与构建检查。

单元测试和包级集成测试继续与实现同目录。Access 契约至少验证：重复/旧版本 profile 幂等或拒绝、同版本内容冲突、事件幂等与 ACK、缺口触发快照校准、Redis ACL 双向拒绝。只有出现真实跨进程、Compose 或端到端测试时才创建 `tests/system`。

#### 12.1.10 已确认的后续工程方向

以下方向已经确认，但不属于当前代码目录调整，也不得表述为已完成：

- `node-web` 不需要独立发布或扩缩容时，将前端静态文件并入统一reverse proxy，减少一层Nginx和一个常驻容器；
- 本地 `npm run dev` 需要联调Go API时，为Vite增加 `/api` 开发代理；
- 当前 `node-access`（Kamailio）和ZLMediaKit继续作为阶段一默认启动骨架；
- 只有单Node需要多个Access实例或稳定SIP入口时才引入多 Kamailio 入口池与 L4 VIP。

## 13. 容量口径

“支持多少设备”必须拆开定义：

- 注册设备数；
- 在线设备数；
- SIP KeepAlive周期；
- REGISTER峰值；
- Catalog峰值；
- 同时活跃视频输入；
- 同时观看数；
- 同时录像数；
- 单流码率；
- PTZ和报警峰值；
- RTP输入和媒体输出带宽。

单节点约 1 万设备是设计目标，不是未经压测的硬性承诺。100,000设备级逻辑Node需要通过多个接入实例、多个媒体实例和必要的Kamailio入口池扩展。

100,000设备每60秒KeepAlive约产生1,667次请求/秒，包含响应约3,334条SIP消息/秒。集中重注册、Digest认证和Catalog同步可能远高于稳态流量，必须单独压测。

## 14. GB28181上下级级联

GB28181上级/下级级联作为可选的标准互联能力，不作为中心管理的唯一同步协议。

适合级联：

- 目录；
- 设备状态；
- 报警；
- 实时视频；
- PTZ；
- 录像查询；
- 向第三方平台共享资源。

不通过级联承担：

- 租户和用户管理；
- 中心授权；
- 节点容量和版本；
- 全局运维；
- 私有审计；
- 节点配置和部署管理。

中心与节点内部管理优先使用私有管理协议；需要标准平台互联时启用GB28181级联适配器。

## 15. 分阶段实施

### 阶段一：自治Node（接入闭环优先）

实现：

- Go `node-app`；
- Kamailio `node-access`（固定 6.1.3 + 自定义 gb28181 模块）；
- PostgreSQL（设备权威 + outbox）；
- Redis（ACL 隔离 + AOF）；
- ZLMediaKit；
- Docker Compose；
- 首个切片：REGISTER/Digest MD5、KeepAlive、事件 poll/ack/snapshot、设备软禁用；
- 后续切片：Catalog、INVITE/ACK/BYE、WebRTC播放、录像、告警、审计；
- 单节点容量压测。

阶段一不再包含 PTZ（按 2026-08-17 D2 决策移出）。

### 阶段二：Center纳管

实现：

- Center Go模块化单体；
- Node注册和mTLS；
- CenterGrant；
- 摘要和事件同步；
- 全局租户和节点目录；
- 设备轻量索引；
- 区域管理范围；
- 中心远程查询和授权操作；
- 双端审计。

### 阶段三：Node内部扩展

实现：

- 多 `node-access`（多 Kamailio 实例）；
- 多ZLMediaKit；
- Redis运行路由；
- SIP实例亲和；
- 节点内部健康检查；
- 分批扩容和维护排空。

### 阶段四：多区域和大型租户

实现：

- 多自治Node纳管；
- 租户专属Node；
- 大租户分区；
- 全局设备检索；
- 统计和告警汇总；
- 必要的L4入口和多Kamailio；
- 可选GB28181上下级级联。

## 16. 尚未定稿、需要验证的内容

以下不是已确定设计，而是实施前需要通过PoC和业务信息确认的项目：

- 目标设备厂商和GB28181兼容矩阵；
- 2016/2022版本差异；
- 单节点实际设备上限；
- KeepAlive和REGISTER峰值；
- Catalog最大规模；
- H.264/H.265比例；
- 同时播放、录像和转码路数；
- PostgreSQL和Redis的具体规格；
- Kamailio 与 OpenSIPS 的同场景 PoC 结果（当前已锁定 6.1.3 源码构建，OpenSIPS 仅作备选）；
- Kamailio 自定义模块在真实 Docker daemon 环境中的编译、健康检查与 Compose 全链路（本机无 Docker daemon 时无法完成，需在具备环境的机器验证）；
- RTP网络和WebRTC访问拓扑；
- 节点跨网络部署方式；
- 具体RTO、RPO和SLO；
- 录像对象存储和保留周期；
- 国产CPU、操作系统和网卡兼容性；
- Kamailio GPLv2+ 许可义务在对外分发场景下的复核。

这些数字在压测和设备兼容性验证完成前，不应写成最终容量承诺。
