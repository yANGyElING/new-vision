# Catalog 与通道同步实现计划

> 依据：`docs/design/catalog-channel-model.md`（D41–D50 ⏳ 未实施）。
> 计划仅分解任务，不改设计；实现时归置的三个口子在各任务里已按 D50 定值。

---

## Phase 1：最小闭环——设备注册后通道出现在管理页面

先证明核心合约「设备上报 → 通道入库 → 页面可见」端到端成立，这是风险最高的一段（C 模块从未处理过 Catalog）。

### 1. C 模块：解析 Catalog 应答，向外发进度与结果事件

- 边界：`deploy/kamailio/modules/gb28181/gb28181.c` 在现有 Keepalive 分支外新增 `CmdType=Catalog` 解析；聚合分批（SN + SumNum）；**每收一帧向 events 流发一条 `catalog.progress {device_id, sn, received, total}`**；收齐后再发一条 `catalog.result {ok:true, channels:[…]}`；`total` 取最近一帧的 SumNum（协议允许中途变，以最新为准）。不含主动查询命令与超时判定（统一在任务 5 定）。
- Done：伪造多帧 XML 流时，Redis Stream 依次出现 `catalog.progress` × N + `catalog.result`；分批未收齐则不出现 `result`。
- Verify：(1) C 模块级测试：多帧 XML → 断言 progress 数量与 received/total 递增；(2) `XREAD` 手工核对 `result.channels.len == SumNum`。
- Depends on: 无

### 2. node-app：通道表迁移 + 同步状态字段 + 事件消费者

- 边界：`migrations/` 新增 `channels` 表（tenant_id、device_id、channel_code 唯一、report_name、display_name、org_unit_id、reported_status、missing、last_seen_in_catalog_at；I3 复合外键）；`devices` 表加 `catalog_state`（never/in_progress/ok/failed）、`catalog_last_query_at`、`catalog_last_ok_at`、`catalog_last_count`、`catalog_last_error`；新增消费者：处理 `catalog.progress`（仅更新设备行 state + last_query_at）与 `catalog.result`（ok → upsert 通道行 + 未出现的置 missing=true；fail → devices.catalog_state=failed + 填 last_error，**不清旧数据**）；缺省字段保留旧值（D43）。
- Done：`catalog.result{ok:true}` 后 channels 行齐全、display_name 等保留；`ok:false` 后 devices.catalog_state=failed 而 channels 不变。
- Verify：`go test ./internal/nodeapp/...`——四场景：首同步 / 缺失 / 复活 / 失败保留旧数据。
- Depends on: 1

### 3. node-app：通道只读 API（双视角）

- 边界：`GET /api/v1/devices/{id}/channels`（管理页，全列）；`GET /api/v1/channels?org_unit_id=`（用户页平铺、按 D28/D29 数据权限 + COALESCE 出展示名与归属）；响应里带所属设备的 `catalog_state/last_ok_at`，让用户页面能渲染「同步中/失败/骨架」。
- Done：老张调用看到 8 行含 report/display 分离；小李只看到他范围内的通道、显示名是 COALESCE 结果，且附带同步状态。
- Verify：API 集成测试——不同范围用户调用，断言返回集合按组织树裁剪 + 状态字段齐全。
- Depends on: 2

### 4. 前端：两页面的通道呈现 + 同步状态渲染

- 边界：管理页设备表格新增「目录同步」列（四态：✓ 时间+路数 / 圆环+`百分比 · 实收/预计` / ✗ 红环停位+超时 / —）；通道列 = 点阵 + `在线数/总数 在线`；所属组织只显直接父级；组织树用嵌套容器 + 左侧导轨线（父节点 ▸ 可折叠、叶子小圆点）；用户页工具栏「更新通道列表」按钮 + 顶部横幅三态（圆环+百分比 / 失败提示 / 骨架占位）；离线设备通道整组灰显（保留最后快照）；缺失通道带徽标；全部视觉细节按 D51 与 .emma/skills/apple-design（:active 缩放、毛玻璃、暗色、tabular-nums 防抖、减动效）；**不挂 SSE**（先静态渲染，SSE 在任务 7 接入）。
- Done：用 mock API 数据渲染出 `device-page-demo.html` v4 的全部状态；组织树可折叠；进度数字更新无抖动。
- Verify：e2e（Playwright 现有套件扩用例）：注入四态设备 → 断言四类行渲染与禁用态；暗色模式快照；`prefers-reduced-motion` 下无位移动画。
- Depends on: 3

---

## Phase 2：触发链路 + 异步回执

### 5. node-access：下发 Catalog 查询命令 + 滑动超时（首包 30s / 滑动 10s）

- 边界：node-app JSON-RPC 新增 `access.v1.queryCatalog{device_access_id, sn}`；C 模块收到后发 SIP MESSAGE 查询；**超时：查询发出后 30s 无首包 → 失败；有首包后超时重置为 10s 滑动，任一 10s 无新帧 → 失败**；失败发 `catalog.result{ok:false, error, received, total}`。不含调度。
- Done：RPC 一下，设备三态（不来/来一半/全来）都按规则产生正确事件流。
- Verify：RPC 集成测试：mock 三种应答，断言事件序列与错误字段里的错误原因。
- Depends on: 1, 2

### 6. 注册即查 + 每小时轮询 + 防抖

- 边界：node-app 起 ticker：每小时对所有在线设备发任务 5 的 RPC；**设备注册成功事件**触发首查；「从未同步成功的设备每次注册都补查」；同一设备两次查询最小间隔 5 分钟（防抖，实现时确认）；轮询 jitter ±10% 防整点风暴。
- Done：设备上线 1 分钟内首次出现通道；一轮小时级轮询后所有在线设备 catalog_last_query_at 都更新。
- Verify：集成测试：伪造注册事件 + 时钟推进，断言查询被发且节流生效。
- Depends on: 5

### 7. 手动刷新按钮 + SSE 异步回执（新权限点 catalog:refresh）

- 边界：`POST /api/v1/devices/{id}/catalog/refresh` **立即返回**（不阻塞等待），权限点 `catalog:refresh`（老张/小李两个域都开）；新增 **SSE 通道**（订阅 `device:{id}:catalog`，转发 `catalog.progress` / `catalog.result`；断线重连后客户端主动拉一次该设备当前 `catalog_state` 兜底）；管理页按钮「就地转圈 + 进度条由事件驱动」；用户页同样刷新自己的横幅。
- Done：点击后 HTTP 毫级返回；进度条随 SSE 事件实时增长；成功/失败由 SSE 决定终态；断线重连能看到当前状态。
- Verify：SSE 集成测试：伪造多帧 progress → 断言客户端收到时序正确；e2e 点击 → 到终态。
- Depends on: 5, 6

---

## Phase 3：运维可用性

### 8. 通道改名与平台名展示

- 边界：`PATCH /api/v1/channels/{id}` 只接受 display_name；前端改名弹窗里对照显示「设备上报名」；用户/管理页全链路 COALESCE 展示。
- Done：设备上报 `Channel 01`，老张设的「大门口」在两页面都不被覆盖。
- Verify：API 测试（改名→模拟设备上报新名→断言显示名不变）+ e2e。
- Depends on: 4

### 9. 详情抽屉与失败文案

- 边界：设备「⋯」抽屉只保留排障字段（国标码、SIP 用户名、profile 版本、access 同步详情）；**目录同步状态不再放抽屉**（已在行内）；失败文案带「上次成功 11:00 · 本次收 26/32 后超时」这类可定位信息。
- Done：正常设备行零噪音；失败设备文案足够运维定位。
- Verify：e2e 渲染断言三类（正常 / 离线 / 各种失败态）。
- Depends on: 4, 6, 7

---

## Overall validation

- 全流程 e2e：创建设备 → 注册 + Catalog 上报（8 路分批）→ 管理页✓ 8 路 → 改名 1 路 → 重报（改名不被覆盖）→ 少报 1 路 → 该行 missing → 回报 → 复活 → 设备下线 → 用户页全部灰显。
- 进度条链路：点击刷新 → 管理页与用户页同步看到进度从 0→100，失败时进度停在最后位置并带「收 26/32 后超时」文案。
- 权限回归：小李在用户页 / API 两个入口都只能看到他组织范围内的通道（permission-model I 系列）；`catalog:refresh` 权限点被 I9 校验。
- 长短任务组合：轮询与手动刷新并发不双发；分批丢帧绝不产生误 missing。

## 已按 D50 定死的三个值

| 口子 | 值（D50） | 落点 |
|---|---|---|
| 首包超时 | 30s | 任务 5 |
| 首包后滑动超时 | 10s | 任务 5 |
| 防抖最小间隔 | 5 分钟 | 任务 6 |
| 轮询 jitter | ±10% | 任务 6 |

## 未实现前置

- SSE 基建（任务 7 首次引入，将来告警 / 录像 / 命令执行进度都走这一路，**本次只实现 catalog 主题**）
