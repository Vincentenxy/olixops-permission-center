# 管理页面后端接入清单

更新日期：2026-10-30。本文件描述已实现的后端接口如何支撑下一批管理页面，页面本身尚未开发。完整协议以 [OpenAPI](../api/openapi.yaml) 和 [管理接口](management-api.md) 为准。

## 1. 身份与导航

登录和刷新继续使用 `/pub/auth/login`、`/auth/me`，响应不变。平台角色仅决定平台管理入口；`platformRole=none` 不能解释为没有租户管理权。

- 平台超管：通过 `/tenant/list` 查看全局租户，通过 `/user/list` 管理后台用户和角色，分配或停用租户管理员。
- 所有已登录用户：通过 `POST /auth/tenant/list` 分页查询自己具有有效管理员关联的有效租户。该查询对超管也不会扩大为全部租户；无关联返回 `total=0,list=[]`。
- 普通后台用户没有有效租户关联时显示无管理范围状态。租户管理员仅操作自己有效租户内的系统并查询相应审计，不开放平台用户管理。

`/auth/tenant/list` 请求只支持 `pageNum/pageSize/code/name`。分页默认 1/20、上限 200，响应仅有 `total/list`。下拉选择器需要分页搜索，不能把第一页当成完整租户范围，也不能通过前端缓存代替后端范围校验。全局租户列表和实际管理员关联列表用途不同。

## 2. 页面与接口对应

以下路径均带 `/api/v1` 前缀。

| 页面 | 查询 | 操作 | 范围与注意事项 |
|---|---|---|---|
| 租户管理 | `/tenant/list`、`/tenant/detail` | `/tenant/create`、`/tenant/update`、`/tenant/setStatus` | 超管写入；租户管理员只读自己有效租户 |
| 后台用户 | `/user/list`、`/user/detail` | `/user/create`、`/user/update`、`/user/setStatus`、`/user/setPlatformRole` | 仅超管；最后有效超管由后端保护 |
| 租户管理员 | `/tenant/member/list` | `/tenant/member/allocate`、`/tenant/member/setStatus` | 超管分配/调整；租户管理员可读自己租户成员 |
| 系统管理 | `/system/list`、`/system/detail` | `/system/create`、`/system/update`、`/system/setStatus` | 创建时同时生成唯一账号，不自动签发密钥 |
| 调用账号 | `/system/detail` 的 `account` | `/system/account/setStatus` | 账号固定归属系统，版本独立于系统 |
| 固定 Token | `/system/credential/list`、`/system/credential/checkCreate` | `/system/credential/create`、`/system/credential/revoke` | 元数据查询/吊销与签发资格不同，超管不能绕过签发资格 |
| 管理审计 | `/audit/list`、`/audit/detail` | 无修改/删除 | 超管全局，租户管理员仅所属有效租户范围 |

所有带参查询与写操作使用 POST JSON；除登录外均带管理员 JWT。系统固定 Token 不能调用这些接口。

分配管理员时复用 `/user/list` 并筛选 `status=active`，按 `username` 或 `name` 分页查找用户，再提交选中 `userId`。已停用的关联用带 `version` 的恢复操作，不重新分配。用户和成员列表均支持 `username` 不区分大小写的包含匹配，`%`、`_`、反斜线按字面量处理。

## 3. Token 签发资格

进入系统的密钥区域时调用 `POST /system/credential/checkCreate`：

```json
{"systemId":"33333333-3333-4333-8333-333333333333"}
```

允许时 `data={"allowed":true,"reason":""}`；不允许时 `allowed=false`，`reason` 对应下表。以下是业务数据，不新增特殊响应码。

| reason | 页面含义 |
|---|---|
| `tenantDisabled` | 先恢复所属租户 |
| `tenantAdminRequired` | 当前账号须具有所属租户的有效管理员关联，超管也不例外 |
| `systemDisabled` | 先恢复系统 |
| `accountDisabled` | 先恢复调用账号 |
| `activeCredentialExists` | 当前已有有效 Token，可查看元数据；确需重发时显式吊销后新建 |

按上表顺序检查，多个条件不满足时只返回首个原因。查询和实际签发复用应用规则。普通租户管理员失去目标范围后返回 404，不返回目标状态；超管仍可查询停用对象。依赖错误返回服务错误，不能作为成功的资格结果展示。

该查询不产生写入、不预留名额，也不是核心业务资源权限计算。签发时从 `/system/detail` 取得 `account.id`，调用创建接口；后端仍在事务内实时复查。检查成功后也可能因并发签发、停用或撤销管理员关联而失败。

明文仅在创建成功结果中短暂展示，关闭清理；不进入 URL、日志或持久 store。创建结果未知时不自动重试，查询元数据定位后由管理员决定是否吊销重发。恢复租户、系统或账号不会复活已吊销密钥。

## 4. 通用交互与联调

- 写请求使用当前实体的 `version`；409 保留输入并引导刷新，不自动覆盖。
- 统一请求层处理错误一次；401 清理身份，403/404 不展示目标详情，503/网络错误提供重试机会。
- 查询、提交有各自 loading；写操作禁止重复提交。切换租户、角色变化和退出时清理旧范围数据及迟到响应。
- 列表和总数均由后端按同一范围计算，不在分页后用前端过滤隐藏跨租户数据。
- 使用超管（有/无租户关联）、单租户管理员、多租户管理员、无关联用户及停用身份做端到端验收。

本轮只改后端；已核对前端 `6bb7a41` 的认证 API/类型，现有登录契约兼容。新增查询尚无前端调用方。数据库继续使用已批准七表，无新增 DDL、业务库修改或集群部署。
