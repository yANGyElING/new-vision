# new-vision 当前状态知识库

> 基于实际代码（working tree）生成，反映已完成实现的业务逻辑与工程状态。
> 最后更新于 2026-08-19。

---

## 1. 项目概述

联邦式「中心管理平台 + 自治节点平台」视频接入架构。每个自治节点是一套完整的小型视频平台，独立完成 GB/T 28181/RTSP 设备接入、本地用户管理、实时视频等业务。中心负责全局纳管多个节点。

**当前阶段**：阶段一 —— 单一自治节点的工程骨架与控制面闭环。尚未部署中心。

---

## 2. 当前状态摘要

| 领域 | 状态 |
|---|---|
| **Go node-app HTTP 进程** | ✅ 完成：配置加载、结构化日志、HTTP 生命周期、健康/指标 |
| **PostgreSQL 设备权威** | ✅ 完成：设备 CRUD、HA1 派生、profile outbox |
| **Access 同步（outbox → JSON-RPC）** | ✅ 完成：reconcile / sync / poll 循环 |
| **Redis 运行时投影** | ✅ 完成：GetMany 批量读取、Reconcile 全量替换、事件 Apply |
| **设备管理 API** | ✅ 完成：公开 `/api/v1/devices` CRUD，含编辑元数据 |
| **健康检查** | ✅ 完成：`/livez`、`/readyz`、`/api/health`、`/metrics` |
| **测试控制台页面（Vue 3）** | ✅ 完成：设备管理、链路测试、Access 运行时查看 |
| **SIP 模拟器（测试用）** | ✅ 完成：REGISTER + Digest、KeepAlive、Unregister |
| **Kamailio 6.1.3 构建配置** | ✅ 完成：Dockerfile、kamailio.cfg、自定义 gb28181 模块源码 |
| **Kamailio 模块（profile/event）** | ⚠️ 代码完成，未编译验证：apply/remove/replace profiles、getRuntimeSnapshot、pollEvents、ackEvents、REGISTER/Digest、KeepAlive、runtime 过期 |
| **ZLMediaKit 容器固定** | ✅ 完成：digest 固定，服务声明 |
| **Docker Compose 编排** | ✅ 完成：所有服务定义、依赖关系、健康检查、网络、卷 |
| **数据库迁移** | ✅ 完成：基线、设备表、设备元数据 |
| **Docker daemon 容器运行验收** | ❌ 未完成（本机无 Docker daemon） |
| **Kamailio 真实镜像编译** | ❌ 未完成（需 Docker daemon） |
| **SIP/RPC 闭环验证** | ❌ 未完成（需容器运行） |
| **媒体编排（播放/PTZ/录像）** | ❌ 后续阶段 |
| **内部设备 API（/internal/v1/devices）** | ❌ 已删除：工作区改动通过移除 `registerDeviceRoutes` 删除 `/internal/v1/devices` 路由，仅保留公开 `/api/v1/devices` |
| **access-snapshot Redis 键** | ❌ 已删除：`Replace` 不再写入 `access-snapshot` key，快照由 Kamailio 端持有 |

> **注意**：README.md 第 71 行仍声称内部设备 API「保持不变」，待当前 refactor 最终确认后更新；设计文档的 `access-snapshot` 引用也已过期。

---

## 3. 服务架构

```text
┌───────────────────────────────────────────────────────────┐
│                    reverse-proxy（Nginx）                   │
│                    :8080（统一入口）                          │
│    /api/* → node-app:8080      其他 → node-web:8080      │
└──────────────────────┬────────────────────────────────────┘
                       │
┌──────────────────────┴────────────────────────────────────┐
│                        node-app（Go）                        │
│   ┌─────────────┐   ┌──────────────┐   ┌───────────────┐  │
│   │ Device Mgr   │   │ Access Client│   │  Sync Runner  │  │
│   │ (CRUD+HA1)  │   │  (JSON-RPC)  │   │(reconcile/    │  │
│   │             │   │              │   │ sync/poll)    │  │
│   └──────┬──────┘   └──────┬───────┘   └───────┬───────┘  │
│          │                │                   │          │
│          ▼                ▼                   ▼          │
│   ┌──────────┐    ┌──────────────┐    ┌──────────────┐   │
│   │PostgreSQL │    │ Redis         │    │  Kamailio    │   │
│   │ (devices+ │    │ (nodeapp 投影)│    │ node-access  │   │
│   │  outbox)  │    │              │    │ JSON-RPC API │   │
│   └──────────┘    └──────────────┘    └──────────────┘   │
└──────────────────────────┬────────────────────────────────┘
                           │ SIP UDP :5060
                           ▼
                    摄像头 / 测试模拟器
```

### 3.1 服务职责

| 服务 | 职责 |
|---|---|
| **node-app（Go）** | 设备业务权威（PostgreSQL）、HA1 派生、profile outbox、Access 同步循环、Redis 运行时投影、HTTP API（设备 / 健康 / 控制台） |
| **node-access（Kamailio 6.1.3）** | SIP/GB28181 协议接入面：REGISTER + Digest、KeepAlive、profile 管理、runtime 管理、事件生产；自定义 `gb28181` 模块暴露 JSON-RPC 2.0 控制面 |
| **node-web（Vue 3 + TypeScript）** | 设备管理页面、控制台页面、健康状态 |
| **reverse-proxy（Nginx）** | 统一 HTTP 入口，路由分发 |
| **zlmediakit** | 媒体数据面（推流、转码、转发），当前仅固定 digest 声明，未接入业务 |
| **postgres** | 设备权威存储（devices 表、access_profile_outbox 表） |
| **redis** | 运行时投影（device-runtime）、事件游标（access-cursor）；ACL 隔离（nodeapp / access 两个用户） |

---

## 4. 核心业务流程

### 4.1 设备生命周期

```text
创建设备
   │
   ▼
  1. 校验输入：center_code 8 位数字、device_type（132/118/111/200）、
     device_name、manufacturer、sip_realm、password
   │
  2. 生成 14 位前缀：center_code + "00" + device_type + "0"
   │
  3. 分配 6 位序号：SELECT MAX(SUBSTRING(device_access_id FROM 15 FOR 6))
     WHERE device_access_id LIKE prefix || '%'，递增，上限 999999
   │
  4. 派生 HA1：MD5(sip_username + ":" + realm + ":" + password)
   │
   ▼
  5. INSERT INTO devices（device_access_id、sip_username=access_id、digest_ha1、enabled）
   │
  6. INSERT INTO access_profile_outbox（device_id, profile_version=1）
   │
   ▼
  7. 同步循环拾取 outbox → 经 Access RPC 同步 profile
```

**关键规则：**
- 密码仅用于派生 MD5 HA1，**不落数据库**。
- `device_access_id` 必须为 20 位数字，由系统从 14 位前缀 + 6 位递增序列自动生成。
- `sip_username` 强制等于 `device_access_id`（数据库 CHECK 约束）。
- 启用/停用（SetEnabled）递增 `profile_version` 并写入 outbox。
- 元数据变更（device_name、manufacturer）**不**触发 profile_version 和 outbox —— 它们是管理面字段，永不同步到接入层。
- 并发创建同一前缀的设备可能撞 UNIQUE 约束，DeviceManager 在 `ErrConflict` 时重试 3 次（每次重新分配序号）。

### 4.2 HA1 密码派生

```go
HA1 = MD5(username + ":" + realm + ":" + password)
```

- 使用 Go 标准库 `crypto/md5`。
- HA1 以 32 位 hex 存储（`digest_ha1 CHAR(32)`）。
- 仅在创建时计算，之后不可逆。
- 通过 outbox 同步到 Kamailio，Kamailio 使用 HA1 回复 REGISTER Digest 挑战。

### 4.3 Profile Outbox 与 Access 同步

**outbox 表结构：**

| 列 | 用途 |
|---|---|
| `device_id` | 外键 → devices，级联删除 |
| `profile_version` | 待同步的版本号 |
| `attempt_count` | 失败重试次数（跨重启持久化） |
| `next_attempt_at` | 下次可重试时间（指数退避） |
| `processed_at` | 成功处理时间（NULL = 未处理） |
| `last_error` | 最近一次错误消息（≤1000 字符，脱敏） |

**同步策略（「提交先于同步」）：**
1. 设备创建 / 启用 / 停用 → PostgreSQL 事务提交（devices + outbox 同事务）。
2. 异步 outbox 消费者拾取 → 调用 Kamailio `access.v1.applyDeviceProfile`。
3. 成功 → `MarkSynced`：更新 `access_synced_version`、置 `processed_at`、清 `last_error`。
4. 失败 → `MarkFailed`：递增 `attempt_count`、按指数退避重算 `next_attempt_at`。
5. 同步失败**不回滚业务提交** —— outbox 保证最终一致。

**退避公式（SQL 侧，`devices.go` MarkFailed）：**

```sql
next_attempt_at = now() + LEAST(
    interval_microseconds * (1 << LEAST(attempt_count, 15)) * interval '1 microsecond',
    interval '1 minute'
)
```

- `attempt_count` 是 SET 之前的值：首次失败乘数 1（不翻倍），之后 2x、4x、8x……
- 上限 1 分钟；`attempt_count` 存数据库，重启后继续累积。

### 4.4 Sync Runner 循环

Sync Runner 在 `App.New()` 中以 goroutine 启动，每 `NV_ACCESS_POLL_INTERVAL`（默认 1s）触发一轮。

```text
Run 循环（每轮）：
  │
  ├── 尚未 reconcile：
  │     reconcile()
  │       1. 取全部设备 profile → access.v1.replaceDeviceProfiles（原子替换）
  │       2. 取 access.v1.getRuntimeSnapshot（替换后快照，覆盖替换期间产生的
  │          旧注册清理事件）
  │       3. MarkReconciled（把已替换版本写入 access_synced_version、清 outbox）
  │       4. projection.Replace(states)（全量覆盖 Redis 运行时投影）
  │       5. SetCursor + AckEvents
  │
  └── 已 reconcile：
        ├─ syncOne()
        │    1. NextPending：取最早到期的 outbox 行（next_attempt_at <= now()）
        │    2. access.v1.applyDeviceProfile
        │    3. 成功 → MarkSynced
        │    4. PROFILE_VERSION_CONFLICT → 返回 ErrReconcile，下一轮重新 reconcile
        │    5. 其他失败 → MarkFailed（DB 侧指数退避），错误消息脱敏
        │
        └─ poll()
             1. currentCursor：读 getRuntimeSnapshot 取 instance，读 Redis cursor
             2. access.v1.pollEvents(cursor, 500)
             3. 校验：instance / session_epoch 未变、事件序号严格递增无间隙
             4. 按 device_access_id 查设备 → projection.Apply（逐个写 Redis）
             5. SetCursor + AckEvents（through = next）
```

**状态机：**
- `reconciled=false` → 执行 reconcile；失败保持 false，成功置 true。
- `reconciled=true` → 执行 syncOne；若返回 ErrReconcile 则置 false。
- poll 出错（实例切换、epoch 变化、序号间隙、校验失败）→ 置 false，下一轮 reconcile。

### 4.5 Redis 运行时投影

**key 空间（nodeapp ACL 用户）：**

| Key | 类型 | 用途 |
|---|---|---|
| `nv:nodeapp:v1:device-runtime:{device_id}` | Hash | 设备当前运行时状态 |
| `nv:nodeapp:v1:device-runtime-ids` | Set | 存在运行时状态的设备 ID 索引 |
| `nv:nodeapp:v1:access-cursor:{access_instance_id}` | Hash | 事件轮询游标（epoch + sequence） |

**运行时状态 Hash 字段：**

| 字段 | 说明 |
|---|---|
| `state` | `online` / `offline` / `stale` |
| `reason` | 状态原因 |
| `remote_address` | 客户端地址 |
| `expires_at` | 注册到期时间（RFC3339Nano，零值存空串） |
| `last_seen` | 最后活跃时间 |
| `session_epoch` | Access 会话纪元 |
| `stale` | `"true"` / `"false"` |

**操作：**
- `Get(id)` → `HGetAll` 单设备。
- `GetMany(ids)` → Pipeline `HGetAll` 批量加载，单次往返（替代 N+1）。
- `Apply(id, state)` → 事务化 `HSet` + `SAdd` 索引。
- `Remove(id)` → `Del` + `SRem`。
- `Replace(states)` → 删除索引集合内全部旧 key，重写当前状态（reconcile 全量校准）。
- `Cursor(instance)` / `SetCursor(instance, epoch, seq)` → 游标读写。

---

## 5. API 表面

### 5.1 公开设备 API（`/api/v1/devices`）

| 方法 | 路径 | 描述 |
|---|---|---|
| `GET` | `/api/v1/devices` | 设备列表（批量附运行时状态） |
| `POST` | `/api/v1/devices` | 创建设备 |
| `GET` | `/api/v1/devices/{id}` | 单台设备（附运行时状态） |
| `PATCH` | `/api/v1/devices/{id}` | 启用/停用 `{"enabled": bool}`，或更新元数据 `{"device_name"}` / `{"manufacturer"}` |
| `DELETE` | `/api/v1/devices/{id}` | 删除设备（级联清 outbox，下一次 reconcile 从接入层移除 profile） |

**请求约束：** 最大 64KB、`DisallowUnknownFields`、严格单 JSON 对象、密码仅用于派生 HA1 不落库。

**错误响应格式：**

```json
{"error": {"code": "device_not_found", "message": "device not found"}}
```

错误码：`invalid_device`(400)、`device_exists`(409)、`device_not_found`(404)、`service_unavailable`(503)。

### 5.2 健康检查

| 端点 | 用途 | 可达性 |
|---|---|---|
| `GET /livez` | Go 进程存活 | 容器内部 |
| `GET /readyz` | 依赖就绪（PostgreSQL + Redis） | 容器内部 |
| `GET /api/health` | 公开健康摘要（同 readyz） | 统一入口 |
| `GET /metrics` | Prometheus 指标 | 容器内部 |

- PostgreSQL 和 Redis 都可探测 → `200` / `ready`。
- 任一依赖异常 → `503` / `not_ready`，进程保持存活，依赖恢复后自动恢复就绪。

### 5.3 Access 控制台（调试用）

| 方法 | 路径 | 描述 |
|---|---|---|
| `GET` | `/api/v1/access/snapshot` | Access 运行时快照（透传 access.v1.getRuntimeSnapshot） |
| `GET` | `/api/v1/access/events?after=&limit=` | 事件轮询（透传 access.v1.pollEvents，limit 1–500） |
| `POST` | `/api/v1/access/ack` | 确认事件 `{"through_sequence": n}` |

### 5.4 SIP 测试控制台

| 方法 | 路径 | 描述 |
|---|---|---|
| `POST` | `/api/v1/test/sip/register` | 完整 SIP REGISTER + Digest 认证交换 |
| `POST` | `/api/v1/test/sip/keepalive` | GB28181 KeepAlive MESSAGE |
| `POST` | `/api/v1/test/sip/unregister` | `Expires: 0` 注销 |

请求体：`{"device_access_id": "34020000001320000001"}`

SIP 模拟器使用存储的 HA1 回复 Digest 挑战；测试专用，非生产媒体网关。

---

## 6. 数据模型

### 6.1 PostgreSQL

**`devices` 表（migration 000002 + 000003）：**

| 列 | 类型 | 约束 / 说明 |
|---|---|---|
| `id` | UUID | PK，默认 `gen_random_uuid()` |
| `device_access_id` | VARCHAR(20) | UNIQUE，格式 `^[0-9]{20}$` |
| `device_name` | VARCHAR(255) | 管理面元数据，不同步 |
| `manufacturer` | VARCHAR(255) | 管理面元数据，不同步 |
| `device_type` | VARCHAR(3) | 管理面元数据，不同步 |
| `sip_username` | VARCHAR(20) | 格式校验，且必须等于 device_access_id |
| `sip_realm` | VARCHAR(255) | — |
| `digest_algorithm` | VARCHAR(10) | 固定 `'MD5'` |
| `digest_ha1` | CHAR(32) | 格式 `^[0-9a-f]{32}$` |
| `enabled` | BOOLEAN | 默认 true |
| `profile_version` | BIGINT | 默认 1，> 0 |
| `access_sync_status` | VARCHAR(16) | `pending` / `synced` |
| `access_synced_version` | BIGINT | NULL 或 > 0 |
| `created_at` / `updated_at` | TIMESTAMPTZ | 默认 now() |

**`access_profile_outbox` 表：**

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL | PK |
| `device_id` | UUID | FK → devices，ON DELETE CASCADE |
| `profile_version` | BIGINT | > 0 |
| `attempt_count` | INTEGER | ≥ 0，默认 0 |
| `next_attempt_at` | TIMESTAMPTZ | 默认 now() |
| `processed_at` | TIMESTAMPTZ | NULL = 未处理 |
| `last_error` | VARCHAR(1000) | 脱敏错误消息 |

索引：`(next_attempt_at, id) WHERE processed_at IS NULL`（取 pending 用）、`(device_id, profile_version)`。

### 6.2 Redis key 布局

| Key | 类型 | ACL 用户 | 用途 |
|---|---|---|---|
| `nv:nodeapp:v1:device-runtime:{device_id}` | Hash | nodeapp | 运行时状态 |
| `nv:nodeapp:v1:device-runtime-ids` | Set | nodeapp | 设备 ID 索引 |
| `nv:nodeapp:v1:access-cursor:{instance}` | Hash | nodeapp | 事件游标 |
| `nv:access:v1:profile:{device_access_id}` | Hash | access | Kamailio 设备 profile |
| `nv:access:v1:event-queue:{instance}` | Stream | access | 事件队列 |
| `nv:access:v1:registration:{device_access_id}` | Hash | access | 注册绑定 |

---

## 7. 同步与一致性模型

1. **提交先于同步**：设备业务事务先提交，再经 outbox 异步同步至 Access；同步失败不回滚业务提交。
2. **原子 profile 替换**：reconcile 时 `replaceDeviceProfiles` 原子替换全部 profile，保证接入层与服务端一致。
3. **事件单调顺序**：事件序号严格递增，poll 校验无间隙，间隙即触发 reconcile。
4. **实例 / 会话检测**：Access 重启或故障转移导致 instance / session_epoch 变化时，Sync Runner 检测到不匹配即重新 reconcile。
5. **指数退避**：outbox 失败按数据库侧指数退避重试（初始间隔 = 轮询间隔，上限 1 分钟）。
6. **跨重启持久化**：`attempt_count` 存数据库，进程重启后继续累积退避。
7. **凭据脱敏**：同步失败日志只记 `"access synchronization failed"`，不泄露密码 / HA1。

---

## 8. Kamailio gb28181 模块（已实现，未编译验证）

自定义 C 模块通过 `gb28181_rpc_dispatch()` 处理 JSON-RPC 2.0 方法：

| 方法 | 用途 |
|---|---|
| `access.v1.applyDeviceProfile` | 应用/更新单个设备 profile，版本 CAS |
| `access.v1.removeDeviceProfile` | 按 access_id + version 删除 profile |
| `access.v1.replaceDeviceProfiles` | 原子替换全部 profile（reconcile 用） |
| `access.v1.getRuntimeSnapshot` | 当前注册运行时快照 |
| `access.v1.pollEvents` | 非阻塞事件轮询 |
| `access.v1.ackEvents` | 单调事件确认（through_sequence） |
| `access.v1.pushEvent` | 内部事件生产 |

已实现的 SIP 语义：REGISTER + MD5 Digest、GB28181 KeepAlive XML 校验（禁止外部实体）、Expires: 0 注销、runtime 超时过期（模块定时器）。

Redis 连接为进程级，使用容器环境提供的 ACL 身份；profile 存储于 `nv:access:v1:*`，事件使用 Redis Streams。

---

## 9. 配置参考

所有配置通过环境变量加载（`config.go` 的 `LoadConfig()`）：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NV_HTTP_PORT` / `NV_HTTP_ADDR` | 8080 / `:8080` | HTTP 端口与监听地址 |
| `NV_LOG_LEVEL` | info | 日志级别 |
| `NV_SHUTDOWN_TIMEOUT` | 10s | 优雅关停超时 |
| `NV_HEALTH_TIMEOUT` | 1s | 健康检查超时（≤9s） |
| `NV_POSTGRES_HOST` / `_PORT` / `_DB` / `_USER` / `_PASSWORD` | 必填 / 5432 | PostgreSQL 连接 |
| `NV_POSTGRES_SSLMODE` | disable | disable/allow/prefer/require/verify-ca/verify-full |
| `NV_REDIS_HOST` / `_PORT` / `_USERNAME` / `_PASSWORD` / `_DB` | 必填 / 6379 / nodeapp / 必填 / 0 | Redis 连接（ACL 用户 nodeapp） |
| `NV_ACCESS_RPC_URL` | http://node-access:8090/rpc | Access JSON-RPC 端点（仅 http，无凭据） |
| `NV_ACCESS_RPC_TIMEOUT` | 3s | RPC 调用超时（≤30s） |
| `NV_ACCESS_POLL_INTERVAL` | 1s | 同步循环间隔（≤1min） |
| `NV_ACCESS_INSTANCE_ID` | access-01 | 当前 Access 实例 ID |
| `NV_SIP_HOST` / `NV_SIP_PORT` | node-access / 5060 | SIP 目标（测试模拟器用） |

---

## 10. Docker Compose 服务拓扑

| 服务 | 镜像 | 端口 | 依赖 |
|---|---|---|---|
| `postgres` | postgres:17.6-alpine3.22 | — | — |
| `redis` | redis:8.2.1-alpine3.22 | — | — |
| `node-app` | new-vision/node-app | expose 8080 | postgres, redis |
| `node-access` | new-vision/node-access | publish 5060/udp, expose 8090 | redis (healthy) |
| `node-web` | new-vision/node-web | expose 8080 | — |
| `reverse-proxy` | new-vision/reverse-proxy (nginx:1.29.1-alpine3.22) | publish 8080 | node-app, node-web |
| `zlmediakit` | zlmediakit/zlmediakit:master@sha256:2bb6a4…c4b98 | expose 80 | — |
| `migrate` | migrate/migrate:v4.18.3 | — | postgres (healthy) |

网络 `node`（new-vision-node）、卷 `new-vision-postgres-data` / `new-vision-redis-data`。`node-access` 与 `zlmediakit` 的媒体端口（RTSP/RTMP/RTP/WebRTC）均不发布，仅 Compose 网络内可用。

---

## 11. 迁移

应用启动不会自动修改数据库结构。迁移是一次性 Compose 工具：

```sh
docker compose run --rm migrate up
docker compose run --rm migrate version
docker compose run --rm migrate down 1
```

迁移顺序：`000001_baseline`（空基线）→ `000002_devices`（devices + access_profile_outbox）→ `000003_device_meta`（设备元数据字段）。

---

## 12. 测试与验证状态

已通过（本机无 Docker daemon 前提）：

- `go build ./...` ✅
- `go test ./internal/nodeapp/` ✅（含同步、退避、凭据脱敏、API 不泄露凭据等测试）
- `go vet ./...` ✅
- 前端 `npm ci` / `npm run typecheck` / `npm run build` ✅
- `docker compose --env-file .env.example config --quiet` ✅

未验证（需 Docker daemon）：

- Compose 镜像构建、容器启动、统一入口与全部容器健康检查。
- PostgreSQL/Redis 降级恢复、迁移版本持久化、浏览器端到端验收。
- Kamailio 自定义模块真实编译与 SIP/RPC 闭环。

---

## 13. 已知限制与前瞻

### 阶段一已完成边界

- 首个切片范围：REGISTER / Digest / KeepAlive / 事件。
- PTZ、播放、录像、回放、告警等为后续阶段。

### 当前阻塞项

- **Docker daemon 缺失**：本机只有 CLI/Compose，无法完成容器镜像构建和运行验收。
- **Kamailio 自定义模块未编译**：C 代码就绪，需 Docker daemon 编译验证。
- **SIP/RPC 闭环未验证**：需容器内端到端测试。

### 已知设计变更（working tree 未提交）

- 内部 `/internal/v1/devices` 路由已移除，设备操作统一走 `/api/v1/devices`。
- `access-snapshot` Redis key 已从 node-app 投影移除，由 Kamailio 端独有持有。
- 重试 attempt_count 从进程内存移到数据库，跨重启持久化，退避改为 SQL 侧指数。
- readiness 探针由并行 goroutine 改为串行执行（简化）。
- `List` 用 `GetMany` 批量加载运行时状态，消除 N+1。

### 前瞻

- 中心节点联邦管理协议（节点注册、全局目录、远程操作）。
- WebRTC 播放、PTZ 控制、录像与回放。
- 多租户、多 Access 实例、水平扩展。
