# 第一部分管理接口与公共能力

更新日期：2026-10-30。第一部分表设计已获确认，下面的 HTTP 管理接口已实现。本文与 [OpenAPI](../api/openapi.yaml)、实际路由及 DTO 一起维护；源码部署且数据库按 [数据库设计](database.md) 初始化后即可调用。

## 1. 范围与身份

- 管理员使用本地用户名密码登录，后端校验 bcrypt 摘要并签发 RS256 JWT；`sub` 为内部用户 UUID。每次管理请求仍须查询有效本地用户，不直接采信 JWT 内角色。
- 平台超管为 `platformRole=platform_admin`；其余后台用户为 `none`，通过有效租户成员关系获得 `tenant_admin`。JWT 中的角色声明不能直接授予本地后台权限。
- 系统属于唯一租户；系统创建用例在同一事务创建一个系统账号。系统账号与后台用户分开，不创建用户表中的“机器用户”。
- 普通管理接口使用 JWT。系统调用入口使用其 API Key，经独立中间件/拦截器校验；API Key 不能用于后台管理接口，JWT 失败也不能回退成系统账号认证。
- 系统授权数据的主体、资源、关系和权限检查属于第二部分，第一部分的服务身份探测不写入 SpiceDB。

## 2. 协议与公共字段

接口使用 `/api/v1`；无参读取 GET，带参读取和变更 POST + JSON。响应固定 `{code,msg,data}`，成功为 `code=0`；普通业务冲突默认 HTTP 200、`code=-1` 和安全可读消息。参数格式错误 HTTP 400、未通过认证 HTTP 401、无操作资格 HTTP 403、目标不存在或不在可见范围 HTTP 404，均通过标准响应封装。

乐观版本不匹配采用 HTTP 409 标准响应，提示调用方重新获取数据；后续统一拦截器只提示一次，不自动重试写操作。不新增独立业务错误码。

分页请求匿名嵌入 `page.Request`，Handler 绑定后归一化一次；`pageNum/pageSize` 默认 1/20，最大 200；返回 `{total,list}`，空列表 `[]`。范围过滤同时作用于计数和列表，默认按 `createAt DESC,id DESC` 排序。

所有实体 ID 为内部 UUID v4 字符串。本期不提供外部用户身份重绑。时间为 RFC3339 UTC；更新请求包含 `version`，响应实体包含最新版本。创建请求不传 ID、操作者或审计字段；服务端生成/填充。

`code` 规范化为小写 `[a-z][a-z0-9_-]{0,63}`，创建后不可编辑。名称最长 128 字符；身份字段按数据库设计的 UTF-8 字节上限验证；备注和筛选条件在最终 DTO 中设置明确上限。

`x-request-id` 由 HTTP/gRPC 共用 `pkg/requestid`：1～128 字节可见 ASCII 合法值透传；非法/缺失值复用合法 context 值或生成新值。响应、日志、审计使用同一最终 ID；该字段不是业务幂等键。

## 3. HTTP 接口清单

请求只接受单个 JSON 对象，拒绝未知字段和尾随 JSON，body 上限 64 KiB。依赖未配置或存储不可用返回 HTTP 503，登录失败返回统一 401，不区分用户名不存在和密码错误。

| 方法与路径（均在 `/api/v1` 下） | 主要入参 | 结果与范围 |
|---|---|---|
| POST `/pub/auth/login` | `username/password` | `{accessToken,expiresAt,user}`；user 与 `/auth/me` 一致 |
| GET `/auth/me` | JWT | `{userId,username,name,platformRole}`；身份必须是本地有效用户 |
| POST `/auth/tenant/list` | `code/name/pageNum/pageSize` 可选 | 仅当前账号具有有效管理员关联的有效租户；超管不自动获得全部租户 |
| POST `/system/credential/checkCreate` | `systemId` | `{allowed,reason}`；系统须在可管理范围内，签发时重新校验 |
| POST `/tenant/list` | `code/name/status/pageNum/pageSize` 可选 | 超管全部；租户管理员仅所属有效租户 |
| POST `/tenant/detail` | `id` | 租户详情；超管可查看停用对象 |
| POST `/tenant/create` | `code/name/remark` | 超管创建租户，返回实体 |
| POST `/tenant/update` | `id/name/remark/version` | 超管编辑基础资料，编码不变 |
| POST `/tenant/setStatus` | `id/status/version` | 超管停用/恢复租户 |
| POST `/user/list` | `username/name/status/platformRole/pageNum/pageSize` 可选 | 超管查看后台用户 |
| POST `/user/detail` | `id` | 超管查看后台用户安全资料，不返回密码摘要 |
| POST `/user/create` | `username/password/name/remark` | 超管创建本地用户，初始平台角色 `none` |
| POST `/user/update` | `id/name/remark/version` | 超管修改展示资料，不重绑外部身份 |
| POST `/user/setStatus` | `id/status/version` | 超管停用/恢复用户，保护最后有效超管 |
| POST `/user/setPlatformRole` | `id/platformRole/version` | 超管变更固定平台角色，保护最后有效超管 |
| POST `/tenant/member/list` | `tenantId/username/name/status/pageNum/pageSize` | 超管管理；租户管理员可查看自己租户成员 |
| POST `/tenant/member/allocate` | `tenantId/userId` | 超管首次分配管理员；已有效关联按幂等成功处理 |
| POST `/tenant/member/setStatus` | `id/status/version` | 超管停用/恢复已有管理员关联；不新建重复关系 |
| POST `/system/list` | `tenantId/code/name/status/pageNum/pageSize` 可选 | 仅可管理范围；租户参数不能覆盖认证范围 |
| POST `/system/detail` | `id` | `{system,account}`，系统资料与唯一账号概览 |
| POST `/system/create` | `tenantId/code/name/remark` | 返回 `{system,account}`；同事务创建并写审计，不自动签发密钥 |
| POST `/system/update` | `id/name/remark/version` | 编辑资料，租户与编码不变 |
| POST `/system/setStatus` | `id/status/version` | 停用/恢复系统，保持下层自身状态 |
| POST `/system/account/setStatus` | `id/status/version` | 停用/恢复系统账号，不换绑系统 |
| POST `/system/credential/list` | `systemId/status/pageNum/pageSize` | 密钥元数据，不包含 token 或 digest |
| POST `/system/credential/create` | `accountId/name` | 仅该租户有效管理员可创建；返回 `{credential,token}`，明文只此一次 |
| POST `/system/credential/revoke` | `id/version` | 吊销终态；重试已吊销操作不重复产生变更 |
| POST `/audit/list` | `tenantId/systemId/actorId/action/result/requestId/from/to/pageNum/pageSize` 可选 | 超管全局；租户管理员必须附加所属租户范围 |
| POST `/audit/detail` | `id` | 同列表范围校验，返回安全事件信息 |
| GET `/systemAccess/me` | 系统 API Key | 返回可信 `tenantId/systemId/accountId/keyId`，不接收请求作用域 |

`status` 普通实体为 `active/disabled`；密钥为 `active/revoked`。系统固定密钥没有到期时间和轮换接口，每账号最多一把有效密钥；重复创建返回普通业务失败。平台超管单有平台角色不能签发，必须先具有系统所属租户的有效管理员关联。

分配接口只负责新增关联或确认现有有效关联；发现停用关联时提示使用带版本的恢复接口，不在无版本的请求中隐式恢复。成功更新返回相应公开 DTO；密钥创建使用专用结果 DTO，列表没有 token 或 digest。

首次超管采用离线受控命令，按数据库设计在空用户库中初始化；不提供公开 HTTP bootstrap。租户、系统、用户暂不开放删除接口。

## 4. gRPC 与系统接入

当前系统身份探测使用 HTTP `/systemAccess/me`；本次没有新增系统身份业务 RPC。未来 gRPC 接入必须复用 `application/management.AuthenticateSystem`，不能复制密钥校验和作用域规则。现有 `CheckPermission` proto 仍未注册为可调用服务。

系统接口请求头为 `Authorization: Bearer <system-api-key>`；未来 gRPC metadata 使用小写 `authorization`。账号校验检查密钥摘要、吊销及账号/系统/租户状态，不披露密钥是否存在。数据库不可用仍返回服务错误，不伪装成凭据错误。

公开 Healthy/标准 health 保留公开性，不用它们证明调用密钥有效。管理 CRUD 本期优先 HTTP，不复制全部 CRUD 为另一套 gRPC 业务服务。

## 5. DDD 与复用落点

| 位置 | 权威实现职责 |
|---|---|
| `pkg/auth` | 管理员 JWT 签发/验签、可信身份解析与 bcrypt 密码校验 |
| `pkg/credential` | 系统密钥随机生成、规范格式解析、完整 token 摘要比对；不检查数据库状态 |
| `pkg/requestid` | 跨 HTTP/gRPC 的请求 ID 规范化、生成与 context 传递 |
| `pkg/page`、`pkg/response` | 分页归一化、标准响应；业务模块不重新定义 |
| `internal/domain/management` | 管理身份、租户、系统、凭据和审计实体及存储端口 |
| `internal/application/management` | 共享用例、固定管理权限和范围校验、事务与审计编排 |
| `internal/infrastructure/persistence` | GORM 持久化、事务实现和查询；不把 GORM 类型穿透到领域层 |
| `internal/interfaces/http` / `grpc` | 绑定/校验协议字段、调用用例、转换响应与错误 |
| `internal/bootstrap` | 装配共享实例和依赖生命周期，普通启动不建表、不自动提权 |

共享抽象应由真实调用需求驱动；不建立万能 CRUD 服务或反射式 Repository。身份状态检查、租户范围和成功审计不能在每个 Handler 中各实现一份。

## 6. 前端衔接与验收

独立前端为 `olixops-permission-center-web`，本次交付登录、刷新身份恢复、退出及账户概览；管理 CRUD 已有后端接口，具体管理页面后续开发。实际部署还需准备数据库与配置，不能将本地实现描述成已升级集群。

前端统一请求实例、错误提示一次、局部 loading、重复提交限制和本地分页状态。管理员 401 统一处理身份失效；403/404 不展示不可见对象详情；409 保留表单并引导刷新，不自动覆盖并发修改。

后续密钥页面仅在创建完成后短暂展示，关闭视图清理，不进入 store 持久化、URL 或日志；请求结果未知时禁止自动重试签发，应查询元数据并由管理员处置。

后端联调覆盖两个租户、两个系统、多租户管理员、最后超管保护、租户管理员签发限制、并发单有效密钥、吊销、上层停用、分页范围及审计回滚。前端隐藏按钮不替代后端范围校验。

## 7. 管理页面配套查询（2026-10-30）

- `POST /auth/tenant/list` 使用与普通租户列表相同的分页、编码精确匹配和名称模糊筛选，但总数和列表始终只包含当前用户具有有效 `tenant_admin` 关联的有效租户。平台超管同样按真实关联过滤；空列表不影响其平台管理权。请求不接受 `tenantId/userId/status`，不会由请求覆盖当前身份或范围。
- `POST /system/credential/checkCreate` 接受 `{systemId}`，返回 `{allowed,reason}`。仅在调用者可管理的系统内查询：无效 ID 返回 400，不存在或不可见系统返回 404，用户停用返回 401，依赖失败返回 503。查询不写凭据或审计，不返回 Token、摘要或账号内部记录。
- `allowed=true` 时 `reason=""`；否则依次为 `tenantDisabled`、`tenantAdminRequired`、`systemDisabled`、`accountDisabled`、`activeCredentialExists`。多个条件不满足时仅返回第一个。租户管理员在租户/关联失效后已经失去可见范围，此时返回 404；平台超管仍可查看停用对象及相应阻止原因。
- 资格查询仅是当前快照的页面提示，不是权限凭证或预留。实际签发在写事务中复用同一规则并重新检查，查询后状态变化仍可能导致签发失败；不得自动重试签发。
- `/user/list` 和 `/tenant/member/list` 新增 `username`（最长 64 字符），按登录账号不区分大小写的包含匹配；与名称、状态等条件取交集。`%`、`_`、反斜线按字面量匹配，不能扩大范围。现有接口响应和 `/auth/me` 保持兼容。

页面与接口对应、角色范围和异常处理见 [管理页面后端接入](management-pages.md)。独立前端当前只调用登录和 `/auth/me`；本次核对了其 API/类型，无需修改现有登录契约，新页面尚未接入。
