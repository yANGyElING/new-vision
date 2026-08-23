# 身份与权限模型重构：区域(Region) → 组织单元(Org Unit)

- 状态：🔄 设计提案（0.x，可破坏性重构，无向后兼容负担）
- 范围：node-app 本地身份与权限模型（identity / authn / authz / device / web）
- 目标：消除「区域」与「组织架构」两套概念并存的冗余，收敛为一套权限架构
- 相关：架构基线 [`federated-video-platform-architecture.md`](federated-video-platform-architecture.md)（多租户、区域只是管理属性）
- 当前实现对照：[`../knowledge-base.md`](../knowledge-base.md)（实现后需同步更新 §5/§6）

---

## 0. 背景与问题

现状模型有三层实体：

```
tenants（租户，Casbin dom，顶层硬隔离）
├── users（用户，tenant_id + roles）
│    └── user_region_scopes（区域范围，多对多）
└── devices（设备，tenant_id + region_id）
regions（全局树，无 tenant_id，播种一个 "root" 根）
```

问题：

1. **「区域」与「组织架构」语义重叠**。`/identity` 页面导航叫「组织架构」，内容却是「租户 + 区域」两个 tab；而区域树承担的正是"数据范围"职责——这本质上是组织/管理分组的职责。维护两套树 = 维护两套权限架构，概念冗余。
2. **区域树是全局的（regions 无 tenant_id）**，跨租户共享同一棵树。一个租户的节点被另一个租户的设备/用户引用，隔离边界模糊。
3. **"root" 根节点是人造物**：它的存在只是为了 (a) seedAdmin 获得全量可见、(b) 设备 `region_id` 有默认值、(c) 树有顶层。它不是用户定义的树的一部分，却污染了用户的树结构。
4. **Casbin 是过度设计**。权限矩阵是编译期常量（`rolePermissions`），却用 Casbin 按租户建 enforcer + 缓存 + 失效广播。固定的角色→权限映射不需要规则引擎。
5. **根节点重名不受约束**：`UNIQUE (parent_id, name)` 在 PostgreSQL 中对 `parent_id IS NULL` 的行不生效（NULL ≠ NULL），扁平/多根树场景下同级重名可入库，与 UI 宣称的"同级名称不可重复"矛盾。
6. **seedAdmin 硬编码魔法 UUID**（`00000000-...-0002` root region），与迁移耦合。
7. `Principal.RegionIDs` 字段是死代码（JWT claims 不携带它，运行时恒为空），易误导。

设计基线（`docs/design/federated-video-platform-architecture.md`）确认：**"地市、区域、行业只是管理属性，不写死为固定行政层级"** —— 这直接支持把区域收敛为组织树的节点属性，而不是独立实体。

## 1. 目标模型

### 1.0 默认状态（设备未分配组织时）

**默认的模型关系：设备与组织单元之间没有关系。** 它们是租户下两个并列的独立实体：

```
tenants ──────────── 租户
├── org_units ────── 用户自由定义的组织树（从空开始，自引用树）
├── users ────────── 用户（roles + scopes → org_units）
└── devices ──────── 设备（独立列表，org_unit_id = NULL，不指向任何组织）
```

操作流：

1. 注册设备 → 设备进入租户下的独立列表（无组织）
2. 建组织树 → 树存在，与设备无关
3. 分配设备 → PATCH 设置 `org_unit_id`，设备进入某人的 scope 可见范围
4. 不分配 → 设备永远只在平台级用户（node_admin / all_orgs）下可见

**设备是设备，组织是组织。组织树是用户用来组织设备的一个工具，不是设备的存在容器。**

### 1.1 实体

```
tenants ──────────────── 租户：认证域 + 顶层隔离域（保留）
├── org_units ────────── 组织单元树：每租户一棵树，取代 regions，无播种根，从空开始
│                        （树内 parent_id 自引用；节点可省/市/仓/部门/项目，语义自由）
├── users ────────────── 用户：roles（能力）+ all_orgs（全租户标记）
│                        + user_org_scopes（管辖范围，可选指向 org_units）
└── devices ──────────── 设备：org_unit_id 可空，可选指向 org_units（非容器/包含关系）
```

> **devices 与 org_units 是并列实体，不是包含关系**。树是 org_units 的自引用树；
> 设备通过一个**可空外键可选地指向**某个组织节点——像“标签/归属”，不是“挂在树下”。
> 设备的存在与接入不依赖树（创建解耦）；但要被 scope 受限用户看到，必须有一个归属
> （可见性绑定）——归属是授权的锚点，不是存在的条件。

- **组织单元（Org Unit）是唯一的分组/范围实体**。节点可以是省/市/区县、仓库、部门、项目——语义由使用方定义，与架构基线"管理属性"一致。
- **树从空开始，无默认根**：顶层 = `parent_id IS NULL` 的节点。用户自由定义结构——可以是扁平列表（全部顶层节点），可以是任意深度树。展示层按实际结构渲染，扁平树自然平铺，不需要特殊展示模式。
- **租户保留**：多租户是产品需求（架构基线：节点支持多租户共享/专属）；租户 = 认证域 + 权限域 + 组织树的租户归属。不是"第二套权限架构"，而是与组织树分层：租户是域边界，组织树是域内数据范围。
- **设备可选指向组织单元**：见 1.0（默认无关系）与 D10（创建解耦）。
- **设备可选指向组织单元**：`devices.org_unit_id` 可空，取代 `region_id`。**创建层解耦**：设备的技术接入（SIP 凭据/注册）不依赖组织树，可先接入后归位。未分配（NULL）设备**存在但仅对平台级可见**（node_admin / all_orgs），scope 受限用户不可见（它在任何子树之外）；分配 = PATCH 设置 org_unit_id，之后进入正常 scope。

  > **设备与 org_units 不是“挂在树下”，而是“可选指向”**。存在和接入都解耦；
  > 只有“被受限用户看见”这个授权动作需要归属。
- **用户管辖范围**：`user_org_scopes`（多对多，多选，子树继承），取代 `user_region_scopes`；另有 `users.all_orgs` 显式标记 = 全租户可见（含未分配设备，见 1.4）。
- **不设用户单值「所属组织」字段**：多选 scopes 已覆盖"一个人管多个组织"的场景；单一归属是 UI 默认值问题（取 scopes[0]），不需要冗余列。

### 1.2 权限模型（去掉 Casbin）

角色→权限矩阵是常量，直接查表：

```go
// authz
func Allow(roles []string, obj, act string) bool {
    for _, role := range roles {
        if acts, ok := rolePermissions[role][obj]; ok && contains(acts, act) {
            return true
        }
    }
    return false
}
```

- 删除：casbin 依赖、EnforcerCache、per-tenant enforcer、RoleLoader、onRoleChanged 失效回调。
- 中间件：认证 → 从 DB 读该用户 roles（或短缓存）→ `Allow` → 通过/403。角色变更即时生效（现状 enforcer 也是 DB 加载，无性能回退）。
- 数据范围：仍由 `visibleOrgIDs`（子树展开）+ SQL `org_unit_id = ANY(...)` 过滤。

### 1.3 数据模型（schema）

```sql
-- 租户（不变）
tenants (id, name, status)

-- 组织单元树：取代 regions，每租户一棵，无播种根
org_units (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    parent_id UUID REFERENCES org_units(id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX org_units_tenant_idx ON org_units (tenant_id);

-- 同级同名唯一（含根）：COALESCE 修复 NULL 不参与唯一约束的问题
CREATE UNIQUE INDEX org_units_sibling_name_idx
    ON org_units (tenant_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), name);

-- 用户：新增 all_orgs 显式全租户标记（默认拒绝）
users (
    ...,
    all_orgs BOOLEAN NOT NULL DEFAULT FALSE
);

-- 用户管辖范围：取代 user_region_scopes
user_org_scopes (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_unit_id UUID NOT NULL REFERENCES org_units(id) ON DELETE CASCADE,
    UNIQUE (user_id, org_unit_id)
);

-- 设备：region_id → org_unit_id，可空（未分配设备仅平台级可见）
devices (..., tenant_id UUID NOT NULL REFERENCES tenants(id),
             org_unit_id UUID REFERENCES org_units(id));
CREATE INDEX devices_org_idx ON devices (org_unit_id);
```

**一致性要求**：`devices.org_unit_id` 和 `user_org_scopes.org_unit_id` 必须属于同一 tenant —— 由应用层校验（handler 层在创建/更新时校验 org unit 的 tenant_id 与目标租户一致），DB 层不强行跨表复合外键。

### 1.4 范围解析

可见性 = 显式全租户标记 ∨ 所选组织单元的子树展开。

```sql
-- 一次查询：输入所有 scoped org ids，输出全部子树
WITH RECURSIVE subtree AS (
    SELECT id FROM org_units WHERE id = ANY($1)
    UNION ALL
    SELECT c.id FROM org_units c JOIN subtree s ON c.parent_id = s.id
)
SELECT id FROM subtree;
```

`visibleOrgIDs(ctx, tenantID, userID)` 规则：

| 条件 | 结果 |
|---|---|
| 角色含 node_admin | 全部（平台级，不做组织过滤，含未分配设备） |
| `users.all_orgs = true` | 该租户全部组织单元 + 未分配设备 |
| 有 scopes | 各 scope 子树展开的并集（**不含未分配设备**） |
| 空 scopes 且非上述 | 空集 = 看不到任何设备（默认拒绝） |

- 消除 seedAdmin 对 root 魔法 UUID 的依赖：admin 不再需要"被分配 root 组织"。
- `all_orgs` 是显式授予（授予方是 node_admin），新建组织单元自动纳入可见范围，不会像"全选顶层节点"那样漂移。
- **未分配设备是 limbo 状态**：创建不需要组织，但分配组织前只有 node_admin / all_orgs 用户可见。这是"创建解耦"的必然代价，语义明确：没归组 = 没纳入任何人的管辖。

## 2. 迁移步骤（0.x，直接改迁移文件，无数据迁移）

| # | 改动 | 文件 |
|---|------|------|
| 1 | 迁移重写：regions → org_units（+tenant_id，删 root seed）；user_region_scopes → user_org_scopes；devices.region_id → org_unit_id（可空）；users + all_orgs；同级同名唯一索引 | `migrations/000002_devices.up/down.sql`、`000004_auth.up/down.sql` |
| 2 | identity：Region → OrgUnit 重命名（types / repository / handler / 接口） | `internal/identity/*` |
| 3 | authn：Principal.RegionIDs 删除；/me 返回 `org_scopes` + `all_orgs` | `internal/authn/authn.go`、`handler.go` |
| 4 | authz：删 Casbin，实现 `Allow(roles, obj, act)` + 新中间件 | `internal/authz/*` |
| 5 | nodeapp：visibleRegionIDs → visibleOrgIDs（一次 CTE + all_orgs/node_admin 短路）；scope 中间件 | `internal/nodeapp/routes.go`、`app.go` |
| 6 | device：ListByTenant 参数与 SQL 改名；创建设备校验 org 归属租户 | `internal/nodeapp/device/*` |
| 7 | 前端：api types、UsersView（管辖组织多选 + 全租户选项）、DevicesView（归属组织下拉）、IdentityView（组织树 tab）、ConsoleView；空状态引导"先创建组织单元" | `web/src/api/*`、`web/src/views/*` |
| 8 | 测试与 e2e：字段名同步 | `tests/*`、`deploy/.e2e-test.mjs`、`deploy/.audit-check.sh` |

## 3. 测试计划

- `authz`：`Allow` 单测（角色矩阵全覆盖：每角色×每 obj×每 act）。
- `identity`：OrgUnit 树 CRUD、同级重名（含根级）拒绝、子树展开（一次 CTE 版）、scope 替换事务、all_orgs 读写。
- `nodeapp`：`visibleOrgIDs` 展开正确性（多 scope、跨租户隔离、node_admin 短路、all_orgs）。
- `device`：按组织过滤、组织归属租户校验、未分配（NULL org）设备仅 node_admin/all_orgs 可见。
- e2e：用户创建/分配组织、设备按组织可见性、node_admin 全量可见、all_orgs 全量可见。

## 4. 决策记录

- **D1 区域 → 组织单元**：合并为单一 `org_units` 树，删除独立区域概念。依据：架构基线"区域只是管理属性"+ 用户明确不希望维护两套。
- **D2 组织树按租户隔离**：`org_units.tenant_id`，修复全局树跨租户耦合。
- **D3 保留租户**：租户 = 认证/隔离域（架构基线要求节点多租户），与组织树分层而非并列。
- **D4 不播种默认根**：树从空开始，用户自由定义结构（扁平或任意深度）；顶层 = parent_id IS NULL。
- **D5 用户用多选 scopes + all_orgs 显式标记**：不设单值「所属组织」列；"全租户可见"是显式授予，不是默认。
- **D6 空 scopes = 看不到任何设备**（默认拒绝），不因角色改变语义。
- **D7 去掉 Casbin**：固定角色矩阵用常量查表，删除规则引擎及缓存/失效机制。
- **D8 node_admin 不做组织过滤**：平台级角色全量可见，消除魔法 UUID 依赖。
- **D9 同级重名唯一索引用 COALESCE**：修复 PostgreSQL 中 NULL 不参与唯一约束的现存 bug（根级重名可入库）。
- **D10 设备创建与组织解耦**：`devices.org_unit_id` 可空。技术接入（SIP 凭据/注册）不依赖组织树；未分配设备存在但仅对平台级可见（node_admin / all_orgs），分配后进入正常 scope。

## 5. 开放问题（需用户确认）

- Q1：`all_orgs` 是否在用户表单中作为独立开关展示（如"管辖全部组织"复选框），还是仅作为 node_admin 的隐藏能力？
- Q2：组织单元移动/重挂（变更 parent_id）是否在本次范围内？当前设计**不含**移动操作（0.x 可后补）。
- ~~设备创建门槛~~：已撤销（D10，org_unit_id 可空）。

## 6. 交叉引用

- 架构基线：`docs/design/federated-video-platform-architecture.md`（多租户约束、"区域只是管理属性"）
- 前端规范：`docs/design/design-spec.md`（设备/用户/组织页面的视觉规范）
- 当前实现：`docs/knowledge-base.md`（实现后需更新 §5 API、§6 数据模型）
- 索引：`docs/design/README.md`、`docs/README.md`
