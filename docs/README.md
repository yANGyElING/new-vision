# new-vision 文档 Wiki

> 文档分两条线：**设计文档**（目标架构，在 `design/` 下）和 **当前状态知识库**（`knowledge-base.md`，从代码生成）。
> 需要找设计决策 → 从 `design/README.md` 入口；需要看当前实现 → 读 `knowledge-base.md`。

---

## 目录

| 路径 | 内容 | 维护者 |
|---|---|---|
| [`design/`](design/README.md) | 设计文档索引（目标架构、重构提案、规范） | 人工 |
| [`design/design-spec.md`](design/design-spec.md) | 前端设计规范（v2 iOS + Notion 系统风格） | 前端 |
| [`design/federated-video-platform-architecture.md`](design/federated-video-platform-architecture.md) | 架构基线：中心-自治节点联邦式架构 | 架构 |
| [`design/identity-scope-refactor.md`](design/identity-scope-refactor.md) | 身份与权限模型重构：区域→组织单元 | 架构 |
| [`design/permission-model.md`](design/permission-model.md) | 权限模型：数据权限、功能权限、租户套餐、权限点清单、审计 | 架构 |
| [`design/identity-and-auth.md`](design/identity-and-auth.md) | 账号与登录：租户与账号生命周期、身份标识、登录方式、密码、会话与限流 | 架构 |
| [`design/catalog-channel-model.md`](design/catalog-channel-model.md) | Catalog 与通道模型（⏳ 待讨论，仅课题提纲） | 架构 |
| [`knowledge-base.md`](knowledge-base.md) | 当前实现状态（从 working tree 生成，含已完成的业务逻辑与工程状态） | 工程 |

---

## 文档间关系

```
架构基线 (federated-video-platform-architecture.md)
├── 定义了：中心-自治节点、多租户、节点本地完整平台、断网自治
├── 约束：区域/地市/行业只是管理属性，不是固定层级
└── 被引用：identity-scope-refactor / permission-model / identity-and-auth

身份与权限模型重构 (identity-scope-refactor.md)
├── 范围：自治节点本地身份与权限模型
├── 状态：已实现（commit 7d4eb3e）
└── 其中 D4（不播种根）/ D5（all_orgs）/ D7（角色矩阵为常量）已被 permission-model.md 推翻

权限模型 (permission-model.md)          ← 进来之后能做什么
├── 数据权限：组织树范围、租户根节点、设备与通道两个授权对象
├── 功能权限：角色入库、授权子集规则、权限点分级
├── 租户套餐：模块开关，判定链的第一关
├── 权限点完整清单（§3.6）与审计（§5）
├── §6 不变量与强制点：每条缺陷对应一条本该存在的约束
└── §9.1 列出已上线代码里待修的安全缺陷

账号与登录 (identity-and-auth.md)        ← 谁是谁、怎么进来
├── 租户开通/停用/删除、首个管理员
├── 用户名租户内唯一 + 邮箱手机全局唯一，登录撞名才问租户
└── 密码策略、会话失效、登录限流

Catalog 与通道 (catalog-channel-model.md)  ← ⏳ 下一个待讨论的课题
├── 权限模型 D28/D36 已假设 channels 表存在，但代码里一行都没有
└── 三个真岔路：通道身份 / 设备与平台两个权威 / 通道消失怎么办

前端设计规范 (design-spec.md)
├── 范围：所有 Vue 页面的视觉规范
└── 登录页、用户表单、组织树、弹窗需遵循其 token 系统

当前状态知识库 (knowledge-base.md)
├── 描述：已实现的业务逻辑和工程状态
└── 注意：§5 API 表面、§6 数据模型仍停留在 org_units 重构之前，待同步
```

---

## 文档约定

- **一个领域一个专题文档**：新的设计讨论若成体系（如权限模型、媒体编排、中心纳管），单独建文档，不追加进既有文档
- 新建专题文档后，**必须同时更新两处索引**：本页的目录表 + [`design/README.md`](design/README.md) 的文档列表与「按主题查找」
- 设计文档优先使用 Markdown，代码示例用 fenced code block
- 决策记录（D1, D2, ...）放在设计文档末尾，作为可追溯的决策日志；**编号跨文档连续**，后文推翻前文的决策须写明理由
- 开放问题（Q1, Q2, ...）标注"待确认"，确认后收入决策记录
- 跨文档引用使用相对路径（本页指向设计文档用 `[架构基线](design/federated-video-platform-architecture.md)`，`design/` 内部互指用 `[权限模型](permission-model.md)`）