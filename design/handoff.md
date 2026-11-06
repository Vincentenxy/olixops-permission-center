# 前后端开发交接与待确认事项

更新日期：2026-11-06。本文件是前后端唯一的交接与待确认清单；两个仓库 README 提供各自的能力索引。后续 Agent 更新本文件，不另建重复状态清单。记录不确定事项时写明事实、影响、当前处理和所需信息，不记录敏感值；继续完成不依赖待确认事项的已授权工作。

## 1. 当前结论与阅读顺序

**管理基础的前后端代码已完成；SpiceDB 核心部分完成方案与离线模型验证，实际授权服务和授权页面未实现。** 管理代码完成、离线模型通过、真实服务联调和生产部署是不同状态。

| 仓库 | 地址 | 交接前代码基线 |
|---|---|---|
| Go 后端 | [olixops-permission-center](https://github.com/Vincentenxy/olixops-permission-center) | `c8fcb80`：模型与测试；之前 `d115bd3`：核心方案，`8514c4e`：管理页面后端增量 |
| Vue 前端 | [olixops-permission-center-web](https://github.com/Vincentenxy/olixops-permission-center-web) | `4e01488`：完整管理工作台；`cafc3ae`：随项目携带设计规范和 skills |

以上用于定位已完成工作，后续文档提交会推进 main，不要求回退到这些提交。接手先检查两个仓库 Git 状态并同步远程，保留用户已有修改。

建议阅读顺序：

1. 两个仓库各自的 `AGENTS.md`、README 和本清单。
2. 后端 [领域上下文](../CONTEXT.md)、[第一部分计划](phase-one-plan.md)、[管理 API](management-api.md) 与 [页面接入契约](management-pages.md)。
3. 后端 [核心授权方案](core-authorization.md)、[模型原理与验证说明](../authorization/README.md)、[Schema](../authorization/schema.zed) 和测试快照。
4. 涉及数据时读 [唯一数据库 SQL](database.md)；涉及外部身份时读 [对外认证评估](external-auth-assessment.md)。
5. 前端 UI 开发前读取前端 `design/DESIGN.md`、`.agents/skills/apple-design/SKILL.md`；组件细节和动画再读 `emil-design-eng/SKILL.md`。skills 已随仓库保存，无需依赖个人电脑安装。
6. 新电脑装配见 [workspace.md](workspace.md)；默认按同父目录克隆，但不要求个人盘符一致。

## 2. 已完成的后端与代码定位

| 内容 | 当前实现与入口 |
|---|---|
| 架构/基础设施 | Go 1.26、DDD、Gin、grpc-go、GORM/PostgreSQL、Redis、zap；`internal/bootstrap/` 装配 |
| 本地登录与身份 | bcrypt、RS256、exp/iss/aud、实时用户状态；`pkg/auth/`、`internal/application/management/service.go` |
| 首个管理员与密钥工具 | `cmd/bootstrap-admin/` 仅空用户库初始化；`cmd/generate-keys/` 拒绝覆盖已有文件 |
| 租户、后台用户、租户管理员 | 创建/编辑/查询/停用恢复、成员分配、平台角色、最后超管保护；`internal/domain/management/`、`internal/application/management/` |
| 系统、账号、固定 Token | 每系统唯一租户和账号；单有效密钥、显式签发/吊销；`system.go`、`pkg/credential/` |
| Token 签发资格 | 实时资格查询与写事务复用规则；`credential_check.go`；超管无租户管理员关联不能签发 |
| 事务、查询与审计 | 统一事务、版本控制、范围分页与审计；`internal/infrastructure/persistence/`；成功变更和审计同事务 |
| HTTP | 31 条路由契约；`internal/interfaces/http/`、[OpenAPI](../api/openapi.yaml)；系统身份探测为 `GET /api/v1/systemAccess/me` |
| gRPC | 仅注册标准 health 与自定义 Healthy；`internal/interfaces/grpc/server.go`；非公开方法拦截器当前是 JWT 路径 |
| SpiceDB 客户端 | 官方 authzed-go、连接复用、TLS、取消/超时、启动健康检查；`internal/infrastructure/spicedb/client.go` |

系统固定 Token 的可信身份用例已在 `management.Service.AuthenticateSystem` 实现，业务 RPC 应复用它。当前 gRPC 还未接入此用例，不能直接将现有 JWT 拦截器当作系统调用认证。

当前七张批准表为 tenant_info、system_info、user_info、tenant_member_info、system_account_info、system_credential_info、authorization_audit_log。唯一 DDL、数据库与用户 `permission_center` 的初始化授权均在数据库文档；服务不 AutoMigrate。

## 3. 已完成的前端与代码定位

前端路由均来自 `src/router/index.ts`，管理 API 在 `src/api/management.ts`，DTO 在 `src/types/management.ts`。

| 路径 | 内容 | 入口 |
|---|---|---|
| `/login` | 真实用户名密码登录、内联失败、loading/防重复 | `src/views/LoginView.vue` |
| `/workspace` | 身份概览、刷新身份、管理导航 | `src/views/WorkspaceView.vue` |
| `/workspace/tenants` | 租户及管理员关联、远程用户名搜索、状态操作 | `src/views/TenantsView.vue` |
| `/workspace/users` | 后台用户、角色和状态；平台超管路由保护 | `src/views/UsersView.vue` |
| `/workspace/systems` | 系统/账号、Token 签发资格/创建/吊销 | `src/views/SystemsView.vue` |
| `/workspace/audits` | 审计筛选、分页与详情 | `src/views/AuditsView.vue` |

公共能力复用 `src/api/http.ts`、`src/stores/auth.ts`、`src/composables/use-paged-list.ts`、`src/components/common/` 和 `src/assets/styles/`。只有统一请求层提示普通错误，页面不重复弹错。分页入参 pageNum/pageSize，响应 total/list。

前后端已适配的关键增量：

- 非平台超管在系统页使用 `/auth/tenant/list` 获取实际有效管理员关联；普通租户列表由后端过滤管理范围，前端不从 `platformRole=none` 推断身份。
- `/system/credential/checkCreate` 返回当前签发资格/原因；页面展示阻止原因，创建事务仍复查，资格查询不构成授权凭据。
- 后台用户支持 username 筛选；分配租户管理员使用远程用户名搜索，不以固定第一页用户代替完整搜索。
- Token 明文只在创建结果的一次性对话框显示，关闭后清空；管理员 JWT、系统 Token 和 SpiceDB 服务凭据用途不同。
- Apple 设计规范、固定版本 skills、许可证及相对路径工作区均已提交。未重新初始化或替换现有管理页面。

## 4. SpiceDB 模型完成了什么

模型唯一源码为 `authorization/schema.zed`。Schema 定义 tenant/system/subject/role/resource/access；access 表示某资源的某动作，统一计算 `allowed = grantee + inherits->allowed`。

- grantee 支持直接主体或 role#member，任一来源允许即可；角色成员变更无需逐资源重写授权。
- inherits 表达父目录或类型范围的同动作继承；新资源必须显式建立继承边，系统归属本身不授予业务权限。
- read/update 是不同 access。未来应用层按系统、外部主体/资源 ID 和动作映射内部 UUID，不能直接采用客户端传入的 SpiceDB ID。
- 撤销单条来源后，其他直接/角色/范围/父目录来源仍可能允许；首版没有 deny 优先级。
- `authorization/tests/` 保存 7 组独立状态快照，`authorization/validate.ps1` 执行官方 zed 验证并在警告或失败时返回非零状态。
- 2026-11-05 本机 zed v1.2.1 编译与 37 条断言全部通过；其中 4 条属于非法写入风险复现。临时移除继承公式会失败，恢复后从仓库外运行仍通过。

**关键限制：system/resource 关系没有参与 allowed 公式，不能自动隔离非法关系。** 测试 07 故意写入跨系统直接授权、跨租户角色、跨系统继承、跨动作继承，预期 allowed=true，以证明当前模型不能防住这些错误写入。该测试是风险事实，不是产品允许行为。后续必须封闭写入口并在应用层拒绝这些输入，加入对应的拒绝测试。

离线快照只验证给定关系下的权限计算；尚未验证在线 WriteRelationships/DELETE、强一致性与撤权时序、并发恢复、循环/深度约束、Bulk/Lookup 或性能。未将本 Schema 写入现有集群，用户此前导入的 Playground Schema 仍需单独核对。

## 5. 尚未实现与建议接手顺序

| 顺序 | 工作 | 完成标准/前置条件 |
|---|---|---|
| 1 | 第二部分 SQL 与写入状态机 | 将 9 张候选表细化到字段/索引/生命周期/唯一映射/任务顺序，写入 database.md 并交 Owner 审核 |
| 2 | 正式接口与调用能力 | 类型化 subject/resource/action；明确系统 Token 登记能力和管理员授权入口；替换占位 proto 并同步 OpenAPI |
| 3 | 应用层安全规则 | 同系统/同动作、不可换绑、有效状态、管理员资格、单父链/环/深度与并发校验；拒绝测试覆盖 07 的风险 |
| 4 | 可靠写入基础 | PostgreSQL 事务记录幂等任务、每系统顺序执行、TOUCH/DELETE、未知结果恢复、系统门禁、ZedToken 水位和审计 |
| 5 | 最小服务端闭环 | 主体/资源类型/动作/资源登记，管理员 grant/revoke，operation detail，系统 Token check/checkBulk；HTTP/gRPC 共用应用用例 |
| 6 | 真实 SpiceDB 集成 | 隔离服务验证在线授权/撤权、fully_consistent、另一客户端查询、依赖失败、重试及服务重启 |
| 7 | 角色、类型范围、目录继承 | 管理成员及来源，多来源撤权语义，与模型保持一致 |
| 8 | 前端授权页面 | 主体、资源类型/动作、角色、资源授权、操作状态/失败恢复、权限测试；区分待生效、无权和服务故障 |
| 9 | 后续能力 | 目标规模压测后评审 LookupResources/精确列表、SSO/checkSelf、SDK 和实际系统接入 |

表设计先审批是 [AGENTS.md](../AGENTS.md) 的硬性要求。9 张表目前仅在核心方案中列明职责，**没有完整 SQL 或第二部分表审批，不得先写关联 model/repository/handler/migration**。本轮交接和离线验证不改变此审批状态。

`api/proto/permission/v1/permission.proto` 的 user_id/resource/action 和生成代码只是占位；未注册 PermissionCenterService，现有 HTTP 也没有 grant/revoke/check。不把缺失服务做成前端假成功。

建议的最小验收：

1. 系统 A 登记 user-a 和 folder-1/read；所属租户的有效管理员授权后，HTTP 与 gRPC 均允许。
2. 撤销唯一授权来源，操作确认生效后，另一客户端新发起的鉴权均返回 allowed=false。
3. 系统 B 即使使用相同外部用户名和资源 ID 也不共享权限；非法绑定在写入前拒绝。
4. 无管理员关联的超管不能绕过签发规则；系统 Token 未经委托不能自助 grant/revoke。
5. SpiceDB/目录/队列异常时拒绝执行业务并返回明确协议错误，不将故障伪装为成功的 allowed=false。
6. 变更只有关系、目录状态及成功审计确认完成才报告生效；门禁和重试覆盖写入结果未知的情况。

## 6. 已确认的边界

- 本项目是独立权限中心，Env Vault 是接入方与通用规范参考，不复制其主密钥或 Secret 业务。
- 一个租户多个系统、系统唯一归属租户；后台用户与租户多对多。后台管理员、系统调用账号、接入方业务主体是独立身份。
- 第一期七表已获 Owner 确认；不等于实际业务库已执行 SQL。禁止外键和启动自动建表；结构变更继续先审表。
- 系统 Token 固定、每账号最多一把有效密钥、不轮换/自动到期；只能由所属租户的有效管理员创建，超管身份不能绕过。
- 权限中心集中保存业务授权并提供判断，接入方后端负责在真实业务执行前强制鉴权。
- 外部业务浏览器直连权限中心是已提出的接入目标，业务用户已有统一登录中心/SSO；不能重新把“是否已有 SSO”当成待确认项。
- 自用模板允许系统 member 读取其加入系统的全部数据；这是自用策略，不硬编码为所有外部系统默认授权。此模板尚未实现。
- DDD 依赖边界、HTTP/gRPC 共享用例、规则单一实现、统一响应/分页/错误提示和日志规范继续有效。

## 7. 待确认与环境核对

| 事项 | 当前事实与影响 | 接手处理 |
|---|---|---|
| 模型与规模 | 通用 access 已完成离线语义验证，性能成本尚未验证，方案仍为设计基线 | 补首个系统的资源量、动作数、继承深度、QPS、延迟及列表规模；压测后评审容量和列表方案 |
| 授权/登记委托 | 建议管理员 grant/revoke、系统 Token 默认不自助授权；系统登记能力范围尚待契约定稿 | 明确每种调用身份可登记/变更哪些对象，不从认证成功推导无限授权 |
| 第二部分数据面 | PostgreSQL 与 SpiceDB 无共同事务；9 张候选表未完成 SQL 审批 | 明确 relation_info 是管理声明/来源而非备用权限计算引擎；任务/水位/门禁及审计恢复规则与 SQL 一起评审 |
| SSO 具体契约 | 已有外部身份源，但协议、issuer/audience、JWKS/introspection、主体和客户端映射未核对 | 取得契约后实现业务用户认证/checkSelf；不以后台 JWT 代替 SSO，不新建重复业务密码库 |
| SDK/RPC 范围 | 无可分发 SDK 或正式授权 RPC | 明确首批语言、是否流式、所需调用能力，先完成服务端共享用例 |
| 业务库与管理员 | 只记录隔离测试结果，未核对实际库、账号和密钥 | 对照 database.md 核对现有结构；新库手工初始化，旧库做增量方案；已有用户库不清空重建超管 |
| 部署/TLS | 应用入口代码为 HTTP/gRPC，出站 SpiceDB TLS 不等于入站 HTTPS；未确认应用生产部署 | 核对域名、Service、网关/TLS 终止、配额、可信代理和 SDK 参数；未经明确部署要求不改集群 |
| 远程 CI | 两仓库有工作流；没有本轮远程结果证据，历史 GitHub API 曾限流 | 分别查看 [后端 Actions](https://github.com/Vincentenxy/olixops-permission-center/actions) 与 [前端 Actions](https://github.com/Vincentenxy/olixops-permission-center-web/actions)；zed 尚未接入 CI |

## 8. 验证证据与本轮交付

| 时间/范围 | 实际记录 | 不能据此推断 |
|---|---|---|
| 历史管理后端 | 隔离 PostgreSQL 17.6 的七表 DDL及 137 项 HTTP 断言；后续隔离 PostgreSQL 18.4 的管理页面增量/并发/事务回归通过；Go test/vet/build、OpenAPI 检查通过 | 当前业务库结构、实际部署或远程 CI 已通过 |
| 管理前端交付 | pnpm check：6 文件、41 测试，lint/类型/构建通过；历史 audit 有通过记录 | 本轮重新审计依赖或新增管理页真实业务流已验收 |
| 历史真实浏览器 | 登录失败/成功、刷新、退出、路由保护及原登录/账号页桌面/390px 窄屏通过 | 租户/用户/系统/Token/审计全部管理流或生产环境已验收 |
| 2026-11-05 模型 | zed v1.2.1，7 组快照/37 条断言通过，故障探测与仓库外执行通过 | 真实 SpiceDB 服务写入、撤权一致性、应用防越权或性能已通过 |
| 2026-11-06 交接 | 本轮修改 README、模型说明、统一交接及旧阶段文字；57 个本地文档链接和 17 个关键源码路径存在性检查通过，两个仓库 git diff --check 通过 | 本轮重新执行全量 Go/前端测试、真实数据库或浏览器联调 |

本轮只整理文档，不改变业务 API、数据库或 Schema。历史临时预览和 /tmp 路径不构成交付依赖：本轮没有启动、停止或确认那些旧进程，不保证旧地址、临时账号或运行状态有效。新电脑按两个 README 自行准备环境。Docker 引擎在 2026-11-05 模型验证时未运行，不以历史某次可用描述当前状态。

新管理页面真实验收还需覆盖平台超管、租户管理员、无有效成员用户、Token 单次展示/吊销/停用、资格变化、桌面/窄屏、控制台和重复提示；当前没有完整结果。

## 9. 验证命令与交付规则

后端根目录（变更对应代码时执行）：

```powershell
go vet ./...
go test ./...
go build ./...
./authorization/validate.ps1
# 未加入 PATH 时：
./authorization/validate.ps1 -ZedPath /path/to/zed
```

真实 PostgreSQL 回归为 `go test -count=1 -v ./internal/integration`，必须使用 TEST_DATABASE_DSN 指向名称以 _test 结尾的隔离库；测试仅清理自己创建的随机 schema。未设置时是跳过，不能写成真实数据库验证通过。模型验证无需连接数据库或集群。

前端根目录：

```sh
pnpm install --frozen-lockfile
pnpm check
git diff --check
```

Node/pnpm 跟随前端 manifest；当前要求 Node >=20.9、pnpm 10.11.1，CI 使用 Node 22。不因文档整理反复运行所有业务测试。修改协议/代码后必须执行受影响验证并更新本清单；推送时分别提交两个仓库。

本地 config/config.yaml、.env、PEM/KEY、实际凭据和数据库内容不进 Git；不通过交接记录密码/Token。默认登录仅当前标签页会话，无 refresh token、服务端跨设备 JWT 撤销、公开找回密码；这些是后续范围，不应误认为已实现。
