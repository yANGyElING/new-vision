# 设计文档索引

> 设计文档是**目标架构**的描述，不等同于当前实现。
> 当前实现状态见 [`../knowledge-base.md`](../knowledge-base.md)。

---

## 文档列表

| 文档 | 主题 | 状态 | 最后更新 |
|---|---|---|---|
| [`federated-video-platform-architecture.md`](federated-video-platform-architecture.md) | 架构基线：中心-自治节点联邦式架构（节点自治、中心纳管、多租户、SIP 接入） | ✅ 已确认基线 | 2026-08-18 |
| [`identity-scope-refactor.md`](identity-scope-refactor.md) | 身份与权限模型重构：区域(Region)→组织单元(Org Unit)，去 Casbin | ✅ 已实现（D4/D5/D7 被 `permission-model.md` 推翻） | 2026-08-23 |
| [`permission-model.md`](permission-model.md) | **权限模型：数据权限（组织树范围）、功能权限（角色/菜单）、租户套餐、权限点清单、审计** | ✅ 设计定稿 / ⏳ 未实施 | 2026-08-28 |
| [`identity-and-auth.md`](identity-and-auth.md) | **账号与登录：租户与账号生命周期、身份标识、登录方式、密码、会话与限流** | ✅ 设计定稿 / ⏳ 未实施 | 2026-08-28 |
| [`catalog-channel-model.md`](catalog-channel-model.md) | **Catalog 与通道模型：设备目录同步、通道数据模型与生命周期** | ⏳ **待讨论**（仅提纲，无已定设计） | 2026-08-29 |
| [`design-spec.md`](design-spec.md) | 前端设计规范（v2 iOS + Notion 系统风格） | ✅ 已确认基线 | — |

---

## 按主题查找

| 想找什么 | 看哪个文档 |
|---|---|
| 系统整体架构、中心/节点拓扑、多租户约束 | `federated-video-platform-architecture.md` |
| 用户能看到哪些设备（数据权限、组织树范围、根节点、通道归属） | `permission-model.md` §2 |
| 用户能做什么（功能权限、角色、越权约束） | `permission-model.md` §3 |
| 权限点完整清单（对象 × 动作 × 租户级/平台级） | `permission-model.md` §3.6 |
| 租户买了什么（套餐、模块开关） | `permission-model.md` §4 |
| 审计记什么、谁能看 | `permission-model.md` §5 |
| 租户怎么开通/停用/删除 | `identity-and-auth.md` §2 |
| 登录时怎么定位租户、用户名/邮箱/手机的唯一性 | `identity-and-auth.md` §3 |
| 密码规则、重置路径、平台管理员忘记密码 | `identity-and-auth.md` §4 |
| 停用账号后 token 什么时候失效、登录限流 | `identity-and-auth.md` §5 |
| 区域为什么变成组织单元、为什么去掉 Casbin | `identity-scope-refactor.md` |
| 前端页面风格、颜色 token、组件规范 | `design-spec.md` |
| 当前已实现的代码行为 | `../knowledge-base.md` |
| 通道/Catalog 的课题背景与待讨论问题 | `catalog-channel-model.md` |
| **已上线代码里待修的安全缺陷** | `permission-model.md` §9.1（对应不变量见 §6） |

---

## 状态说明

- ✅ **已确认基线**：与代码一致或已由用户确认的现行设计
- 🔄 **设计提案**：尚未实现，或实现中；实现后应更新状态
- ⏳ **待确认**：设计未定，开放问题未拍板