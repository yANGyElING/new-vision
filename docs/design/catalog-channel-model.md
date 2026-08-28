# Catalog 与通道模型（讨论提纲）

- 状态：⏳ **待讨论** —— 本文只是课题的背景与问题清单，**不包含任何已确定的设计**
- 范围：GB28181 设备目录（Catalog）同步、通道（channel）的数据模型与生命周期
- 为什么是现在：见 §1
- 讨论方式：按 `.emma/skills/brainstorm` 的决策树分轮推进（该目录未纳入版本库）；事实由 agent 查证，决策由用户拍板

---

## 1. 为什么现在讨论这个课题

**① 刚定稿的权限模型悬空在它上面。**

[`permission-model.md`](permission-model.md) §2.8 已经决定「设备和通道都可以挂组织树」（D28），并写死了解析规则：

```sql
COALESCE(channels.org_unit_id, devices.org_unit_id)
```

但 `channels` 表、Go 类型、API **一行代码都不存在**（全仓库对 `channel|catalog|通道` 零匹配）。也就是说 D28/D36 建立在一个尚未设计的实体上。若通道的身份规则与该假设不符（例如通道编码会随设备恢复出厂而变化），§2.8 与那条 `COALESCE` 都要返工。

**② 它是架构基线明确的下一个切片。**

[`federated-video-platform-architecture.md`](federated-video-platform-architecture.md) §6.2 第 265 行：

> 尚未实现（后续切片）：**Catalog**、DeviceInfo、DeviceStatus、Alarm、RecordInfo、PTZ、INVITE/ACK/BYE 和媒体行为。

§15 阶段一的后续切片顺序也是 Catalog 打头。

**③ 后面三个切片都依赖它。** 点播要指定通道、录像挂在通道上、告警来源是通道。它是播放 / 录像 / 告警的共同前置。

**④ 零代码 = 零迁移成本。** 现在是最便宜的设计窗口。

---

## 2. 背景：当前实现到哪一步

### 2.1 已经跑通的链路

```
Camera ──REGISTER/Digest/KeepAlive──> node-access（Kamailio + 自定义 gb28181 C 模块）
                                          │
                                     Redis 运行状态（nv:access:v1:*）
                                          │
                        node-app 事件 poll/ack ──> 运行时投影（nv:nodeapp:v1:*）

node-app ──profile outbox──JSON-RPC──> node-access
   access.v1.applyDeviceProfile / removeDeviceProfile / replaceDeviceProfiles
```

- 设备业务数据权威在 node-app 的 PostgreSQL；
- 只存 MD5 HA1，不存明文密码；
- 「提交先于同步」：业务事务先提交，再经 outbox 异步同步至 Access，同步失败不回滚业务提交（基线核心原则 12）。

### 2.2 现有设备模型（`internal/nodeapp/device/device.go:39-62`）

```go
type Device struct {
    ID, TenantID, OrgUnitID
    DeviceAccessID       // 20 位国标编码
    DeviceName, Manufacturer, DeviceType
    SIPUsername, SIPRealm, DigestAlgorithm, DigestHA1
    Enabled
    ProfileVersion, AccessSyncStatus, AccessSyncedVersion   // outbox 同步状态机
    Runtime *access.RuntimeState                            // 来自 Redis 投影
}
```

设备类型只有四个常量（`device.go:68-74`）：`IPC=132` / `NVR=118` / `DVR=111` / `Server=200`，取自 20 位编码的第 11–13 位。创建设备时传的是 `CenterCode`，编码由平台侧组装。

**模型到注册与鉴权为止，没有任何目录 / 通道 / 媒体概念。**

### 2.3 node-access 的职责边界（基线 §6.2）

`node-access` 只做 SIP/GB28181 协议语义转换，**不负责**：播放权限、租户授权、告警业务规则、录像生命周期、设备业务数据权威。

> 边界原则：`node-app` 通过版本化 Access API 表达业务意图，Kamailio 自定义模块负责协议语义转换。

Catalog 的设计必须落在这条边界的两侧——协议交互在 Kamailio 侧，目录数据权威在 node-app 侧。

---

## 3. 已被其他文档预设的约束

讨论时不能推翻、或推翻了必须回头改文档的既有决定：

| 来源 | 约束 |
|---|---|
| `permission-model.md` D28 | 设备与通道是**两个授权对象**：设备归属 = 管理权锚点，通道归属 = 查看权锚点，可见性独立计算 |
| `permission-model.md` D36 | `channels.org_unit_id` 可空，NULL = 跟随设备；解析用 `COALESCE`；UI 初期不提供单独入口 |
| `permission-model.md` D29 | 媒体权限点（`stream:play` / `ptz:*` / `record:*` / `alarm:*`）的**范围锚点是通道**，不是设备 |
| `permission-model.md` I3 | 任何指向 `org_units` 的外键，两端租户必须一致，由**数据库复合外键**强制 —— 通道表同样适用 |
| 基线 §2 原则 12 | 提交先于同步：业务事务先提交，同步失败不回滚 |
| 基线 §6.2 | 协议状态由 Kamailio 独占，业务面不共享其内部 Redis key |

---

## 4. 待讨论的问题

标 ⚠️ 的是**真岔路**（不同选择导致不同的数据模型），其余是需要定值但方向明确的。

### 4.1 通道的身份 ⚠️

- GB28181 通道有自己的 20 位国标编码。它在全局唯一，还是仅在所属设备内唯一？
- 我们的主键用什么？设备重新上报时靠什么匹配到已有的那一行？
- 若只能靠国标编码匹配：**设备恢复出厂导致编码变化时，该通道的组织归属、历史录像、告警记录全部断链**。要不要接受？有没有别的锚？

### 4.2 两个权威打架 ⚠️

同一行通道数据有两个来源：

| 字段 | 权威 |
|---|---|
| 通道名称、厂商、型号、在线状态 | **设备上报**（Catalog） |
| 组织归属、平台侧备注、启用状态 | **平台维护** |

设备把通道名从「大门」改成 `Channel 01`，平台侧老张手工起的名字要不要被覆盖？需要一条明确的裁决规则，否则每次同步都可能冲掉人工维护的数据。

### 4.3 通道消失怎么办 ⚠️

摄像头拆了、线断了，设备不再上报这一路。

- 删除？标记离线并保留？保留多久？
- **删掉的话，挂在这条通道上的录像和告警指向谁？**
- 归属信息留不留——重新接上时能不能自动复原？

（这是 `permission-model.md` §8 的开放问题 Q3，在此收口。）

### 4.4 同步的触发与形态

- 什么时候拉 Catalog：注册时？定时？手动？事件驱动？
- 全量还是增量？如何判断「这次上报是完整的」（GB28181 Catalog 分批返回，可能丢包）？
- 规模口径：一台 NVR 32 路，节点几千台设备 ≈ 十几万通道。全量对比的代价是多少？分页与批量写入策略。

### 4.5 在线状态的单一来源

至少三个来源会声称知道通道是否在线：Catalog 的 `Status` 字段、设备级 KeepAlive、现有的 Redis 运行时投影。需要收敛成一个，并明确设备离线时其通道状态如何推导。

### 4.6 要不要支持嵌套目录

GB28181 的 Catalog 可以多级（平台 → 设备 → 通道），级联下级平台时更深（基线 §14 提到上下级级联，但排期很后）。现在要不要在数据模型上留口？留口的成本是多少？

### 4.7 与权限模型的接口

- 新上报的通道，`org_unit_id` 一律为 NULL（跟随设备）——确认；
- 设备换组织时通道自动跟随（`COALESCE` 天然成立）——确认；
- 通道表的 `tenant_id` 与复合外键形状（I3）。

---

## 5. 明确不在本次范围

- INVITE/ACK/BYE 与媒体编排（下一个切片）
- 录像、回放、下载
- PTZ（基线 2026-08-17 D2 已移出阶段一）
- 告警业务规则
- 上下级级联的完整实现（仅讨论是否留数据模型的口）

---

## 6. 交叉引用

- 权限模型：[`permission-model.md`](permission-model.md)（§2.8 设备与通道、§6 不变量、§8 开放问题 Q3）
- 账号与登录：[`identity-and-auth.md`](identity-and-auth.md)
- 架构基线：[`federated-video-platform-architecture.md`](federated-video-platform-architecture.md)（§6.2 服务边界、§7 调用流程、§15 分阶段实施）
- 当前实现：[`../knowledge-base.md`](../knowledge-base.md)（§4 核心业务流程、§8 Kamailio 模块现状）
