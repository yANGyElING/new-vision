# Catalog 与通道模型

- 状态：✅ 设计已定稿（2026-09-12 与用户逐条确认），⏳ 未实施
- 范围：GB28181 设备目录（Catalog）同步、通道（channel）的数据模型与生命周期
- 姊妹文档：[`permission-model.md`](permission-model.md)（D28/D36 预设了通道作为授权对象，本文是该对象的实体设计）
- 决策编号：D41–D48，接续 `identity-and-auth.md` 的 D40

---

## 0. 一句话

设备上报什么通道，平台就有什么通道；通道永远只标记不删除；平台维护的数据设备永远冲不掉。

---

## 1. 既有约束（来自其他文档，本文不重新决策）

| 来源 | 约束 |
|---|---|
| `permission-model.md` D28 | 设备与通道是两个授权对象：设备归属 = 管理权锚点，通道归属 = 查看权锚点 |
| `permission-model.md` D36 | `channels.org_unit_id` 可空，NULL = 跟随设备；可见性解析 `COALESCE(channels.org_unit_id, devices.org_unit_id)`；UI 初期不提供单独挂组织的入口 |
| `permission-model.md` D29 | 媒体权限点（`stream:play` / `record:*` / `alarm:*`）的范围锚点是通道 |
| `permission-model.md` I3 | 指向 `org_units` 的外键两端租户必须一致，由数据库复合外键强制——通道表同样适用 |
| 基线 §2 原则 12 | 提交先于同步：业务事务先提交，同步失败不回滚 |
| 基线 §6.2 | node-access 只做 SIP/GB28181 协议语义转换；目录数据权威在 node-app |

---

## 2. 通道的身份（D41）

**通道以 `(device_id, 通道国标编码)` 匹配：该二元组就是通道的「身份」。**

- 设备每次上报 Catalog，平台按通道国标编码在所属设备下查找已有行：找到 = 更新；找不到 = 新建。
- 平台仍给每行发内部主键（`id`），供组织归属、录像、告警等外键引用——但匹配永不靠它。
- **接受的代价**：摄像头恢复出厂 / 被重新分配编码后，平台视为全新通道，旧行的组织归属、（将来的）录像与告警记录**断链，不自动接续**（用户 2026-09-12 明确拍板：「恢复出厂则无解，断链就断了」）。恢复出厂在实际部署中是低频操作，不值得为此引入人工认领流程。

## 3. 数据模型

```
channels
  id                uuid PK                    -- 内部主键，外键引用用它
  tenant_id         uuid NOT NULL              -- I3：与 devices/org_units 同租户
  device_id         uuid NOT NULL              -- 所属设备
  channel_code      varchar(20) NOT NULL       -- 通道国标编码（身份的一半）
  report_name       text                       -- 设备上报的原始名称
  display_name      text                       -- 平台自定义名；NULL = 用 report_name
  org_unit_id       uuid NULL                  -- D36：NULL = 跟随设备
  reported_status   varchar(8)                 -- 最近一次 Catalog 的 Status（ON/OFF）
  missing           boolean NOT NULL DEFAULT false   -- D45：缺失标记
  created_at / updated_at / last_seen_in_catalog_at
  UNIQUE (device_id, channel_code)
  复合外键 (org_unit_id, tenant_id) → org_units(id, tenant_id)   -- I3
```

显示名称的规则：**`COALESCE(display_name, report_name)`**（D42）——与通道归属的 `COALESCE` 同一个模式，整套系统里「设备值 + 平台覆盖」只有一种做法。

## 4. 字段权威（D42 / D43）

同一行通道数据有两个写入方，裁决规则：

| 字段 | 权威 | 设备上报能否覆盖 |
|---|---|---|
| `channel_code`、设备级技术参数 | 设备 | （本身就是设备的） |
| `report_name`、`reported_status` | 设备 | 每次完整同步刷新 |
| `display_name`、`org_unit_id`、`missing` 的解除方式之外的标记 | 平台 | **永不** |

- **D42 双列名称**：设备名（`report_name`）与平台名（`display_name`）各存各的。场景：运维小哥在 NVR 上把「大门口」改成 `Channel 01`，老张在平台备注的「大门口」不受影响，界面仍显示「大门口」。
- **D43 增量字段保留旧值**：Catalog XML 字段不齐时，缺省的字段保留数据库旧值，不清 NULL。NULL 只表示「从未上报过」。

## 5. 同步的触发、完整性与流向（D47）

### 5.1 三个触发时机

1. **设备注册成功后自动查一次**——新 NVR 上线即拉目录；
2. **每小时定时轮询**所有在线设备（用户拍板的周期）；
3. **界面手动「刷新目录」按钮**——小李刚加了摄像头可立即补拉。

### 5.2 命令流向

```
node-app ──JSON-RPC──> node-access: 发送 Catalog 查询
device ──MESSAGE (Catalog, 分批)──> node-access（Kamailio gb28181 C 模块解析）
node-access ──Redis Stream（沿用现有事件通道）──> node-app 消费入库
```

通道数据是**上行**（access → app），**不经过**业务 outbox（`access_profile_outbox` 是设备 profile 下发专用，保持不变）。Catalog 事件走 node-access 已有的事件 Stream 通道（`nv:access:v1:events`），node-app 侧新增一类消费者。

### 5.3 分批完整性

GB28181 Catalog 分多条 MESSAGE 返回，可能丢包。规则：**收齐一批次（按协议的分批序号/总数判定）才应用整体变更；未收齐的批次丢弃，该次同步视为未完成，不改动任何通道状态**——包括不产生「缺失」标记。这保证「一次没收到」永远不会误判缺失。

## 6. 在线状态（D44）

**设备与通道两层状态，各算各的：**

- **设备（NVR）在线** = Keepalive，沿用现有 Redis 运行时投影（`device-runtime:{device_id}`），不变。
- **通道在线** = 最近一次 Catalog 上报的 `Status`（ON/OFF），即 `channels.reported_status`。平台不做通道级独立检测。

场景：NVR 绿点活着，但它上报「第 3 路 OFF」→ 该通道显示离线；NVR 自己断了 → 设备灰点，其通道状态保留最后一次上报值（界面上跟随设备变灰即可，不回写通道行）。

## 7. 生命周期：缺失与复活（D45 / D46）

- **D45 通道永不删除，只标记**：某路通道从 Catalog 中彻底不再出现时，`missing = true`，行保留——组织归属、（将来的）录像与告警记录全部不断链，因为外键指向的行永远在。权限模型 D28/D29 以通道为授权锚点，物理删除会让悬挂外键成为常态，故排除。
- **复活**：同 `(device_id, channel_code)` 的通道再次出现时，原行 `missing = false`，归属与历史自动接回。
- **D46 缺失判定时机**：仅在**一次完整同步**（§5.3）结束后，该设备下出现过的通道若不在本次结果中，即标记缺失。不做「连续 N 次」缓冲——完整性已由 §5.3 保证，单次完整结果就是可信快照。
  - 附带的 UX 说明：小李能在界面上区分「这路是 OFF（还在但掉线）」和「这路已缺失（设备不再承认它）」。

## 8. 与权限模型的接口（确认项，照抄既有决策）

- 新通道 `org_unit_id` 一律 NULL（跟随设备），UI 初期不提供单独入口——D36；
- 设备换组织时其通道自动跟随（`COALESCE` 天然成立），无额外逻辑；
- 通道表带 `tenant_id` + 复合外键，与设备表同形状——I3。

## 9. 明确不做

- **D48 不做嵌套目录、不留口**：模型固定两层（设备 → 通道）。上下级级联（基线 §14）若将来实施，届时按需新增结构（很可能是一张独立的目录节点表），现在加 `parent_id` 列属于臆测需求。
- INVITE/媒体编排、录像、PTZ、告警业务规则——后续切片。
- 通道级独立在线检测（D44 已排除）。

## 9A. 前端页面形态（D49）

**用户页面与管理页面分开，主语不同：**

- **用户页面（看的人：小李、小王）——主语是通道。** 通道宫格列表，每路 = 在线圆点 + 通道名（`COALESCE(display_name, report_name)`）+ 位置一行灰字 + [播放] [录像]。不出现「设备/NVR」概念；组织以横排筛选片呈现，不要侧边树。
- **管理页面（管的人：老张、小陈）——主语是设备。** 表格布局：设备名、国标编码、所属租户（**仅小陈可见**——老张永远只看本租户，该列对他是噪音）、所属组织、通道健康点阵、状态、行内操作。**保留侧边组织树**做嵌套筛选；「刷新目录」作为**行内按钮**（不放 ⋯ 菜单）。
- **信息分层原则：正常状态零噪音。** Access 同步版本、SIP 用户名、心跳时间等进「⋯」详情抽屉；「心跳时间」仅在设备离线时升级为列表字段（「最后心跳 3 小时前」）。
- **设备离线 ⇒ 其全部通道在用户页面显示为离线**（展示层推导，不回写 `reported_status`）；缺失通道在用户页面保留入口，可看历史录像，播放禁用。
- 用户页面的播放/录像操作按权限点 `stream:play` / `record:*` 控制（permission-model D29）。
- 交互 demo：`docs/design/device-page-demo.html`。

## 10. 决策记录

- **D41 通道身份 = (device_id, 通道国标编码)**；内部主键仅供外键；编码因恢复出厂而变 = 断链，接受。
- **D42 名称双列**：`report_name`（设备权威）+ `display_name`（平台覆盖），显示用 `COALESCE(display_name, report_name)`。
- **D43 未上报字段保留旧值**，NULL 只表示从未上报。
- **D44 两层在线状态**：设备 = Keepalive 投影；通道 = 最近一次 Catalog Status。
- **D45 通道永不物理删除**，只标 `missing`；同编码再现即复活。
- **D46 缺失判定 = 单次完整同步即生效**；分批未收齐的同步整体丢弃、不算数。
- **D47 三个同步触发**：注册即查 + 每小时定时 + 手动刷新。
- **D48 不做嵌套目录**，不留 parent 口；级联将来另议。
- **D49 双页面模型**：用户页面以通道为主语，管理页面以设备为主语；「所属租户」列仅平台运维可见；正常态不显示内部同步细节。

## 11. 实现差距备注（2026-09-12 探查结论）

- Kamailio gb28181 C 模块目前只处理 REGISTER 与 Keepalive MESSAGE，`CmdType=Catalog` 无任何分支（`deploy/kamailio/modules/gb28181/gb28181.c:1770-1808`）——需新增：解析 Catalog XML、发送查询请求、写事件 Stream。
- 现有 outbox 是设备 profile 下发专用（`internal/nodeapp/sync/sync.go`），通道同步为上行方向，不复用、不改动它。
- Redis 运行时投影只有设备级（`internal/nodeapp/access/projection.go`），无需为通道新增投影——通道状态落 PostgreSQL 即可。
