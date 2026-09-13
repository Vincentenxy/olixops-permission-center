# PostgreSQL 初始化与第一部分表设计

更新日期：2026-09-14。**Owner 已明确确认数据库表设计并授权继续开发。** 本文同时落实本次追加要求：本地用户名密码登录、固定系统密钥、不实现轮换，并且系统密钥必须由所属租户的有效管理员创建。批准开发不等于已经在业务数据库执行 DDL；以下 SQL 供人工初始化，应用启动不自动建表。

已确认的一期范围见 [第一部分计划](phase-one-plan.md)：**系统必须且只能属于一个租户，一个租户可以有多个系统**；同一后台用户可以管理多个租户。后台管理员、系统调用账号和第二部分的业务用户是不同身份，不混用。本期 PostgreSQL 只保存管理状态与管理审计，不创建 SpiceDB 关系镜像或 outbox。SpiceDB 使用独立数据库，其内部表与迁移由 SpiceDB/Operator 管理，本服务只通过官方 API 访问它。

## 1. 初始化前提

- 以下脚本需要使用 PostgreSQL 管理员角色（通常是 `postgres`）执行。
- `app_role` 是应用连接数据库使用的非超级用户角色。
- `migration_owner` 是实际执行迁移/建表的角色。PostgreSQL 的默认权限按“对象创建者”隔离，必须为每个实际建表角色设置一次。
- `app_password` 只作为脚本变量示例，执行前必须替换为强密码；不要把真实密码提交到仓库。
- `CREATE DATABASE` 不能放在事务中，脚本应通过 `psql` 执行，不要包在外层事务里。

## 2. 创建角色、数据库和授权

执行方式示例（先将第二节的 SQL 代码块另存为 `permission-center-init.sql`）：

```bash
psql -U postgres -d postgres -v ON_ERROR_STOP=1 -f permission-center-init.sql
```

`psql` 的 `\connect` 和 `\gexec` 是客户端元命令，不是 PostgreSQL 服务端 SQL，因此不能直接把 Markdown 文档作为 `-f` 输入。

```sql
-- 使用 postgres 等管理员角色执行。
\set app_db 'permission_center'
\set app_role 'permission_center'
\set app_password 'CHANGE_ME_BEFORE_RUN'
\set migration_owner 'postgres'

-- 角色不存在时创建；已存在时下面的 ALTER ROLE 会同步密码和最小权限属性。
SELECT format(
    'CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT',
    :'app_role',
    :'app_password'
)
WHERE NOT EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname = :'app_role'
)
\gexec

ALTER ROLE :app_role
    WITH LOGIN
    PASSWORD :'app_password'
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    INHERIT;

-- 数据库不存在时创建，并将应用角色设为 owner。
SELECT format(
    'CREATE DATABASE %I OWNER %I ENCODING %L TEMPLATE template0',
    :'app_db',
    :'app_role',
    'UTF8'
)
WHERE NOT EXISTS (
    SELECT 1 FROM pg_database WHERE datname = :'app_db'
)
\gexec

GRANT CONNECT, TEMPORARY ON DATABASE :app_db TO :app_role;

\connect :app_db

-- public schema 由应用角色维护；不授予应用角色数据库级 CREATEDB 等管理员权限。
ALTER SCHEMA public OWNER TO :app_role;
GRANT USAGE, CREATE ON SCHEMA public TO :app_role;

-- 让应用角色可以访问初始化脚本执行前已经存在的对象。
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO :app_role;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO :app_role;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO :app_role;

-- 让 migration_owner 后续新建的表、序列、函数自动授权给应用角色。
-- 如果应用角色本身执行迁移，请将 migration_owner 改成 permission_center。
ALTER DEFAULT PRIVILEGES FOR ROLE :migration_owner IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON TABLES TO :app_role;

ALTER DEFAULT PRIVILEGES FOR ROLE :migration_owner IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO :app_role;

ALTER DEFAULT PRIVILEGES FOR ROLE :migration_owner IN SCHEMA public
    GRANT EXECUTE ON FUNCTIONS TO :app_role;
```

## 3. 多个建表角色的默认权限

`ALTER DEFAULT PRIVILEGES` 只对指定的对象创建者生效。如果迁移由多个角色执行，需要以管理员身份为每个角色重复下面的语句；例如迁移角色是 `permission_center_migrator`：

```sql
\connect permission_center

ALTER DEFAULT PRIVILEGES FOR ROLE permission_center_migrator IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON TABLES TO permission_center;

ALTER DEFAULT PRIVILEGES FOR ROLE permission_center_migrator IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO permission_center;

ALTER DEFAULT PRIVILEGES FOR ROLE permission_center_migrator IN SCHEMA public
    GRANT EXECUTE ON FUNCTIONS TO permission_center;
```

如果未来新增独立 schema，必须同时为该 schema 执行现有对象授权和默认权限授权；不能假设 `public` 的权限会自动继承到其他 schema。

## 4. 应用连接配置

可提交的配置文件为 [config/config.example.yaml](../config/config.example.yaml)。填写 `database.host/port/user/password/name/schema/ssl_mode`，或通过 `DATABASE_DSN` 覆盖连接配置；本地 `config/config.yaml` 原样保留且不提交。安全模板中的密码可通过 `DATABASE_PASSWORD` 注入。

```text
DATABASE_DSN
```

应用 DSN 示例：

```text
host=database.example.internal port=5432 user=permission_center password=<secret> dbname=permission_center search_path=public sslmode=require
```

## 5. 业务表设计约束

- 业务表必须先写入本文件并提交 Owner 审核，再创建 model、repository、migration 或 handler。
- 所有可变业务表必须使用软删除字段和审计字段，查询默认过滤已删除记录。
- 禁止外键依赖；表间关系由领域层和应用层维护。
- 不在数据库侧添加业务 CHECK 等特殊校验；业务校验必须在代码层明确实现。

## 6. 第一部分表结构总览

本期只提供停用/恢复，不提供删除接口；六张可变表仍完整预留统一软删除字段。`authorization_audit_log` 只追加。组织层级、自定义角色、业务用户、业务资源授权和 SpiceDB 同步不在本期。

| 表 | 当前状态的唯一归属与关键字段 |
|---|---|
| `tenant_info` | 租户 `code/name/status` |
| `system_info` | 系统及不可变 `tenant_id`，系统编码在租户内唯一 |
| `user_info` | 后台用户内部 UUID、用户名、密码摘要、身份映射、显示名、平台角色与状态 |
| `tenant_member_info` | 用户与租户的管理员关联，固定 `tenant_admin` |
| `system_account_info` | 一个系统只有一个账号，账号不属于 `user_info` |
| `system_credential_info` | 一个账号至多一把有效固定密钥，保存 key ID、摘要与吊销历史 |
| `authorization_audit_log` | 不可修改的操作事实与名称快照，不作为当前权限判断依据 |

所有 `id` 为应用以密码学安全随机源生成的 UUID v4，不使用序列、数据库扩展或 UUID 默认函数。所有时间采用 `timestamptz`，应用按 UTC 处理。`version` 是并发修改版本号，创建为 1，成功修改时加 1。

`status` 在租户、用户、成员、系统、账号中仅允许应用写入 `active/disabled`；密钥仅允许 `active/revoked`。`platform_role` 仅允许 `none/platform_admin`，`member_role` 只允许 `tenant_admin`。这些取值由应用验证，SQL 不加 enum/CHECK/触发器。用户 `display_name` 是本地展示值，对外基础身份 DTO 仍使用 `name`；不把姓名当作唯一身份。

## 7. 七张表的完整 DDL（手动执行）

以下是七张业务表唯一的权威 DDL 代码块。连接已经初始化好的业务数据库后，以确定的 `migration_owner` 执行整个事务；不包含建库、密码、真实用户或初始管理员数据。不得由应用启动自动执行。本节不使用 `IF NOT EXISTS`：表已存在时中止，先比对结构并编写单独审核的升级变更，避免把不兼容结构误判为成功。

第 2、3 节的原有建库授权 SQL 予以保留；其中应用角色拥有数据库/schema 且授予广泛对象权限，属于现有初始化基线，**不提供数据库强制的“只能追加审计”保证**。本期通过封闭 Repository 和接口限制审计为 INSERT/SELECT；如需数据库级不可修改保证，应另行审核分离对象 owner、运行角色及审计表授权的方案，不能仅凭表名宣称已实现。

```sql
BEGIN;

CREATE TABLE public.tenant_info (
    id               uuid         PRIMARY KEY,                         -- 应用生成的租户 UUID
    code             varchar(64)  NOT NULL,                             -- 规范化租户编码，全局永久占用
    name             varchar(128) NOT NULL,                             -- 租户名称
    status           varchar(16)  NOT NULL DEFAULT 'active',            -- active / disabled
    remark           text         NOT NULL DEFAULT '',                 -- 说明，不保存敏感值
    version          bigint       NOT NULL DEFAULT 1,                  -- 乐观并发版本
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT '',                 -- 可信操作者标识
    update_at        timestamptz  NOT NULL DEFAULT now(),
    update_by        text         NOT NULL DEFAULT '',
    delete_at        timestamptz,
    delete_by        text         NOT NULL DEFAULT '',
    is_deleted       boolean      NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX uk_tenant_info_code ON public.tenant_info (code);
CREATE INDEX idx_tenant_info_list ON public.tenant_info (create_at DESC, id DESC)
    WHERE is_deleted = false;

CREATE TABLE public.system_info (
    id               uuid         PRIMARY KEY,                         -- 应用生成的系统 UUID
    tenant_id        uuid         NOT NULL,                             -- tenant_info.id，不可跨租户迁移
    code             varchar(64)  NOT NULL,                             -- 在所属租户内永久唯一
    name             varchar(128) NOT NULL,
    status           varchar(16)  NOT NULL DEFAULT 'active',
    remark           text         NOT NULL DEFAULT '',
    version          bigint       NOT NULL DEFAULT 1,
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT '',
    update_at        timestamptz  NOT NULL DEFAULT now(),
    update_by        text         NOT NULL DEFAULT '',
    delete_at        timestamptz,
    delete_by        text         NOT NULL DEFAULT '',
    is_deleted       boolean      NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX uk_system_info_tenant_code ON public.system_info (tenant_id, code);
CREATE INDEX idx_system_info_tenant_list ON public.system_info (tenant_id, create_at DESC, id DESC)
    WHERE is_deleted = false;

CREATE TABLE public.user_info (
    id               uuid         PRIMARY KEY,                         -- 权限中心内部后台用户 UUID
    username         varchar(64)  NOT NULL,                             -- 规范化本地登录名，全局永久唯一
    password_hash    text         NOT NULL,                             -- bcrypt 摘要，禁止明文密码
    external_issuer  varchar(512) NOT NULL,                             -- 本期为配置的本地 JWT issuer
    subject_claim    varchar(32)  NOT NULL DEFAULT 'sub',               -- 本期固定 sub
    external_subject varchar(255) NOT NULL,                             -- 本期为内部 UUID 字符串
    display_name     varchar(128) NOT NULL,                             -- 对外基础身份字段 name
    platform_role    varchar(32)  NOT NULL DEFAULT 'none',              -- none / platform_admin
    status           varchar(16)  NOT NULL DEFAULT 'active',
    remark           text         NOT NULL DEFAULT '',
    version          bigint       NOT NULL DEFAULT 1,
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT '',
    update_at        timestamptz  NOT NULL DEFAULT now(),
    update_by        text         NOT NULL DEFAULT '',
    delete_at        timestamptz,
    delete_by        text         NOT NULL DEFAULT '',
    is_deleted       boolean      NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX uk_user_info_username ON public.user_info (username);
CREATE UNIQUE INDEX uk_user_info_external_identity
    ON public.user_info (external_issuer, subject_claim, external_subject);
CREATE INDEX idx_user_info_list ON public.user_info (create_at DESC, id DESC)
    WHERE is_deleted = false;
CREATE INDEX idx_user_info_active_platform_admin ON public.user_info (id)
    WHERE is_deleted = false AND status = 'active' AND platform_role = 'platform_admin';

CREATE TABLE public.tenant_member_info (
    id               uuid         PRIMARY KEY,
    tenant_id        uuid         NOT NULL,                             -- tenant_info.id
    user_id          uuid         NOT NULL,                             -- user_info.id，内部 UUID
    member_role      varchar(32)  NOT NULL DEFAULT 'tenant_admin',      -- 一期只有租户管理员
    status           varchar(16)  NOT NULL DEFAULT 'active',
    version          bigint       NOT NULL DEFAULT 1,
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT '',
    update_at        timestamptz  NOT NULL DEFAULT now(),
    update_by        text         NOT NULL DEFAULT '',
    delete_at        timestamptz,
    delete_by        text         NOT NULL DEFAULT '',
    is_deleted       boolean      NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX uk_tenant_member_info_pair ON public.tenant_member_info (tenant_id, user_id);
CREATE INDEX idx_tenant_member_info_user ON public.tenant_member_info (user_id, tenant_id)
    WHERE is_deleted = false AND status = 'active';
CREATE INDEX idx_tenant_member_info_tenant_list
    ON public.tenant_member_info (tenant_id, create_at DESC, id DESC) WHERE is_deleted = false;

CREATE TABLE public.system_account_info (
    id               uuid         PRIMARY KEY,                         -- 系统调用账号 UUID，非后台用户
    system_id        uuid         NOT NULL,                             -- system_info.id，不可换绑
    name             varchar(128) NOT NULL,
    status           varchar(16)  NOT NULL DEFAULT 'active',
    remark           text         NOT NULL DEFAULT '',
    version          bigint       NOT NULL DEFAULT 1,
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT '',
    update_at        timestamptz  NOT NULL DEFAULT now(),
    update_by        text         NOT NULL DEFAULT '',
    delete_at        timestamptz,
    delete_by        text         NOT NULL DEFAULT '',
    is_deleted       boolean      NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX uk_system_account_info_system ON public.system_account_info (system_id);

CREATE TABLE public.system_credential_info (
    id               uuid         PRIMARY KEY,                         -- 密钥记录内部 UUID
    account_id       uuid         NOT NULL,                             -- system_account_info.id，不可换绑
    key_id           varchar(32)  NOT NULL,                             -- 16 字节随机值的小写 hex，永久全局唯一
    token_digest     varchar(64)  NOT NULL,                             -- 完整 token SHA-256 的小写 hex
    name             varchar(128) NOT NULL,                             -- 用途标签，例如生产服务
    status           varchar(16)  NOT NULL DEFAULT 'active',            -- active / revoked，不提供恢复吊销
    revoked_at       timestamptz,                                      -- 吊销时由事务写入
    revoked_by       text         NOT NULL DEFAULT '',                 -- 可信吊销操作者
    version          bigint       NOT NULL DEFAULT 1,
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT '',
    update_at        timestamptz  NOT NULL DEFAULT now(),
    update_by        text         NOT NULL DEFAULT '',
    delete_at        timestamptz,
    delete_by        text         NOT NULL DEFAULT '',
    is_deleted       boolean      NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX uk_system_credential_info_key_id ON public.system_credential_info (key_id);
CREATE INDEX idx_system_credential_info_account_list
    ON public.system_credential_info (account_id, create_at DESC, id DESC) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_system_credential_info_active_account ON public.system_credential_info (account_id)
    WHERE is_deleted = false AND status = 'active';

CREATE TABLE public.authorization_audit_log (
    id               uuid         PRIMARY KEY,                         -- 应用生成的审计事件 UUID
    actor_type       varchar(32)  NOT NULL,                             -- user / system_account / bootstrap / anonymous
    actor_id         uuid,                                             -- 已确认内部主体 ID；未认证或 bootstrap 可空
    actor_name       varchar(128) NOT NULL DEFAULT '',                  -- 当时名称快照
    tenant_id        uuid,                                             -- 服务端确认的目标租户；平台动作可空
    tenant_name      varchar(128) NOT NULL DEFAULT '',
    system_id        uuid,                                             -- 服务端确认的目标系统
    system_name      varchar(128) NOT NULL DEFAULT '',
    account_id       uuid,                                             -- 已确认的调用/目标系统账号
    key_id           varchar(32)  NOT NULL DEFAULT '',                  -- 仅已确认的密钥标识，不含 secret/摘要
    target_type      varchar(64)  NOT NULL,                             -- tenant / user / tenant_member / system 等
    target_id        uuid,                                             -- 创建成功后或经可信解析的目标 UUID
    target_name      varchar(128) NOT NULL DEFAULT '',                  -- 删除或改名后仍可理解历史
    action           varchar(64)  NOT NULL,                             -- 稳定动作名，如 systemCredential.revoke
    result           varchar(16)  NOT NULL,                             -- success / denied / failed
    reason_code      varchar(64)  NOT NULL DEFAULT '',                  -- 内部安全原因分类，不是 HTTP 业务码
    request_id       varchar(128) NOT NULL DEFAULT '',                  -- 已验证/生成的 trace-id，非幂等键
    changes          jsonb        NOT NULL DEFAULT '{}'::jsonb,         -- 白名单字段变化，禁止整份请求或凭据
    create_at        timestamptz  NOT NULL DEFAULT now(),
    create_by        text         NOT NULL DEFAULT ''                  -- 与 actor 一致的规范化操作者标识
);
CREATE INDEX idx_authorization_audit_log_time
    ON public.authorization_audit_log (create_at DESC, id DESC);
CREATE INDEX idx_authorization_audit_log_tenant_time
    ON public.authorization_audit_log (tenant_id, create_at DESC, id DESC);
CREATE INDEX idx_authorization_audit_log_system_time
    ON public.authorization_audit_log (system_id, create_at DESC, id DESC) WHERE system_id IS NOT NULL;
CREATE INDEX idx_authorization_audit_log_actor_time
    ON public.authorization_audit_log (actor_type, actor_id, create_at DESC, id DESC) WHERE actor_id IS NOT NULL;
CREATE INDEX idx_authorization_audit_log_action_time
    ON public.authorization_audit_log (action, create_at DESC, id DESC);
CREATE INDEX idx_authorization_audit_log_result_time
    ON public.authorization_audit_log (result, create_at DESC, id DESC);
CREATE INDEX idx_authorization_audit_log_request
    ON public.authorization_audit_log (request_id) WHERE request_id <> '';

COMMIT;
```

### 7.1 索引与唯一性说明

- 身份与编码唯一索引覆盖已停用和软删除记录：租户编码、租户内系统编码、用户名、身份映射、租户/用户关联、系统/账号关系和 key ID 永久占位。`uk_system_credential_info_active_account` 是明确例外，只约束有效密钥数量，允许吊销历史与后续新密钥并存；它不允许复用历史 key ID。
- `code` 由应用 trim 后转小写，只接受 `[a-z][a-z0-9_-]{0,63}`，创建后不修改；名称可修改且不要求唯一。不同租户可以有同名/同编码系统，但系统内部 UUID 全局不同。
- 外部 issuer/subject 区分大小写，不统一小写；应用分别限制 UTF-8 编码长度最多 512/255 字节，并拒绝首尾空白而不是静默改写身份；字节上限同时控制复合唯一索引项大小。`subject_claim` 作为身份命名空间的一部分，避免 `sub=123` 与 `staffuserid=123` 被意外合并。
- 管理列表索引与默认排序 `(create_at DESC,id DESC)` 一致；用户→租户索引覆盖有效管理员范围；系统账号唯一索引同时支持系统详情查询。
- 审计按租户、系统、操作者、动作、结果、请求 ID 和时间筛选。数据库可以组合索引；具体高频组合通过真实查询计划优化，不预先添加所有组合。租户管理员查询始终先附加可信租户范围，列表和 `total` 使用相同条件。
- 代码先校验唯一性，唯一索引负责并发兜底；冲突转换为可读业务错误，不暴露 SQL。账号与系统在同一事务创建，唯一 `system_id` 保证每系统最多一个账号；创建用例保证恰好一个。
- `now()` 仅是创建时间默认值，不会自动维护 `update_at/version`；每次应用修改须显式设置。`changes` 的 JSON 对象类型、结构和长度限制由应用验证。

## 8. 身份映射与固定管理权限

### 8.1 本地登录、JWT 与内部用户

身份映射键为 `(external_issuer,subject_claim,external_subject)`，关联 `user_info.id`。后台成员表只保存内部 UUID；外部业务用户不是本表管理员，也不会因为具有有效 JWT 自动创建管理员记录。

本期采用本地用户名密码登录。`username` 规范化后永久唯一；`password_hash` 仅保存 bcrypt 摘要，不进入对外 DTO、日志或审计。密码长度和 bcrypt 字节上限由应用验证。创建用户时服务端生成 UUID，`external_issuer` 固定为配置的本地 issuer，`subject_claim='sub'`，`external_subject` 等于该内部 UUID；这三个字段保留明确的身份来源，不意味着一期已经接入外部 SSO。JWT 主体固定取 `sub`，不逐令牌回退到 `staffuserid`，也不根据姓名自动合并账号。

每次管理请求先验证 JWT 的 RS256、签名、有效期、issuer 和 audience，再查本地未删除、`active` 用户及有效角色。未知用户、已停用用户即使 JWT 有效也不能管理。一般平台管理权限不要求租户成员关系；**签发系统密钥例外，必须具有该租户的有效管理员关联**。租户管理员同时要求用户、租户、成员关联有效。显示名称仅用于展示，不能改变绑定身份。

| 操作 | 平台超管 | 租户管理员 |
|---|---|---|
| 租户创建、编辑、停用/恢复 | 全局 | 只查看自己管理且有效的租户 |
| 用户和平台角色管理 | 全局，受最后超管保护 | 不允许 |
| 分配、停用/恢复租户管理员 | 全局 | 不允许自行提升或分配 |
| 系统、账号管理与密钥查询/吊销 | 全局 | 仅自己有效管理的租户 |
| 创建系统密钥 | 单有超管身份不允许，须同时是该租户有效管理员 | 仅自己有效管理的租户 |
| 审计查询 | 全局 | 仅自己有效管理的租户 |
| SpiceDB 业务资源授权 | 第二部分设计 | 第二部分设计 |

租户管理员关系停用后不能靠旧 JWT 或旧缓存继续访问。平台超管可查看并恢复停用租户，但停用租户内不允许创建系统或签发密钥；平台超管仍可吊销已存在的密钥以处置风险。

### 8.2 首个管理员与防锁死

- 不设置默认账号、默认密码、默认超管 JWT 或公开 bootstrap 接口，不在普通 HTTP 服务启动时读取配置自动提升用户。
- 独立离线命令 `go run ./cmd/bootstrap-admin` 由受信任运维显式执行，使用 `BOOTSTRAP_USERNAME/BOOTSTRAP_PASSWORD/BOOTSTRAP_NAME` 环境变量；本地 issuer 来自同一服务配置。命令只保存密码摘要，不输出密码或 JWT，不在公开 HTTP 中提供初始化入口。
- bootstrap 只在 `user_info` 完全没有记录时允许。事务首先取得固定的全局管理员维护锁 `pg_advisory_xact_lock(147,1)`，重新确认空用户库，再以应用生成 UUID 创建 `active/platform_admin` 用户并追加一条 `bootstrap` 审计，同事务提交。并发执行只有一个成功，重跑不修改现有身份或角色。
- 之后所有平台角色变更、后台用户停用及未来删除操作也必须取得同一个锁。修改后检查有效平台超管数量至少为 1；不满足时回滚业务与审计，覆盖两个超管同时停用对方等并发场景。
- 租户允许暂时无管理员，由平台超管分配/恢复；不得给首个登录用户自动授予租户权限。
- 如全部超管遗失密码，应单独设计受控离线恢复流程，在同一维护锁与事务下记录恢复审计；禁止清空用户表或重跑 bootstrap 绕过保护。本期不提供公开找回密码或恢复后门。

## 9. 固定密钥协议与范围

- 密钥格式为 `opc_<keyId>.<secret>`：`keyId` 是密码学安全随机 16 字节的小写 hex（32 字符），`secret` 是独立随机 32 字节的无 padding base64url（43 字符）。数据库存 `sha256(完整token)` 的小写 hex，不存明文、可逆密文或 secret 片段。高熵随机密钥与用户口令不同，不用本规则保存人类密码。
- `key_id` 全局唯一且永不复用；碰撞时重新随机生成，不覆盖旧记录。密钥 digest、key ID 和账号绑定创建后不可变；JWT 和系统密钥使用独立认证入口，不采用 JWT 失败后再尝试密钥的混合逻辑。
- 鉴权按严格格式解析 key ID，查完整记录并对完整 token 摘要做恒定时间比较，再用一个一致查询快照验证 `credential → account → system → tenant` 都未删除且有效、密钥未吊销。租户/系统/账号 ID 由这条链推导，不能信任请求覆盖。
- 密钥创建与审计提交成功后只在本次创建响应展示明文一次；列表、详情、日志和审计不返回 token 或 digest。响应丢失不能“查回”密钥；管理员按标签、创建时间和 key ID 定位新记录，吊销后重发，不以缓存保存原明文换取幂等重放。
- **本期固定密钥无自动到期、无轮换 API、无轮换任务。** 不保留 `expires_at` 或重叠有效期配置；每账号最多一把有效密钥，应用事务锁与局部唯一索引共同保证。历史吊销记录仍保留，不能通过恢复上层重新生效。
- 创建者必须是该系统所属租户的有效管理员；即使是平台超管，也必须先有该租户的有效成员关系。新建系统不自动生成 token，不能借系统创建接口绕过签发资格。
- 上层停用是可逆门禁；恢复只改变对应对象状态，已吊销密钥仍无效。显式撤销设置 `status=revoked`、`revoked_at/revoked_by`，是终态，无恢复接口。密钥丢失或泄露时先吊销，再由租户管理员显式创建新固定密钥。
- 本期不做持久的允许结果缓存，也不在每次校验成功后更新密钥行，避免产生写热点；如以后需要 `last_used_at`，需另行设计异步更新与精度，不把它误作当前鉴权依据。

## 10. 状态、软删除与并发事务

- 一期业务接口仅编辑、停用和恢复，不删除实体。恢复不会重建账号或管理员关联，也不会自动解除下层停用或密钥吊销；原本有效的下层关联随上层恢复可重新生效，这与重新分配权限不同。
- 六张可变表预留的软删除含义统一：`is_deleted=true` 时同时设置 `delete_at/delete_by`，任何鉴权均拒绝；未来开放删除前必须补充接口契约及级联规则审核，不使用数据库级 cascade。
- 未来租户软删除前须确认所有系统已删除、管理员关联已移除；系统软删除前须吊销密钥并处理唯一账号；用户软删除前须撤去管理员关联且满足最后超管保护。历史编码和外部身份不释放；确需恢复原对象时沿用原 UUID，不能创建新记录冒用其编码。
- 成员“移除”在一期体现为停用原 `tenant_member_info` 记录；重新分配恢复同一关联。系统账号身份始终固定，不能通过删除账号再创建第二个账号绕过唯一性。
- `version` 用于拒绝过期管理提交，更新条件同时包含主键、预期版本与 `is_deleted=false`，成功时递增；影响行数为 0 返回明确冲突，不能悄悄覆盖。列表/详情返回版本供后续写请求使用，具体 DTO 在契约中定义。
- 一期管理写入量较小，所有管理写事务统一先取得 `pg_advisory_xact_lock(147,1)`，随后在事务中重新校验操作者与目标状态。bootstrap、角色变更、停用、成员调整、系统创建和密钥签发共用这一把锁，使并发写入有明确顺序，保护最后超管和单有效密钥。
- 该实现主动串行化管理写入，不用于高吞吐业务授权写入。后续确有吞吐需要时再设计租户级锁和全局管理员锁的统一顺序，不能局部绕过现有事务入口。读取用 Repeatable Read 只读事务，计数、列表与身份链在同一快照内完成。
- 仅查询的系统身份探测使用一致 SQL 快照；停用/吊销提交后的新请求不得命中旧允许缓存。已通过鉴权并在执行中的请求不承诺被追溯取消；业务变更必须按上条锁与事务规则重新验证。第二部分业务授权需要另行设计跨系统一致性。
- `request_id` 仅用于追踪，可对应同请求内多个审计事件，不作为唯一业务幂等键。重复创建由自然唯一键兜底；签发密钥不能自动重试有副作用的请求，否则可能多签密钥。

## 11. 审计写入、查询与故障处理

- 每个成功管理变更与 `result=success` 审计在同一 PostgreSQL 事务内提交。审计写入失败则回滚变更，不允许“业务成功但审计丢失”。无状态变化的重复停用可作为幂等成功，不伪造新的成功变更事件。
- 拒绝或失败事件在业务事务回滚后，以独立短事务记录 `denied/failed`。如果审计库仍不可用，操作保持拒绝/失败，以 zap 仅记录安全原因分类和 request-id；不能据此改成成功，也不能承诺这些失败事件已经入库。运维可据此接入告警，本期没有外部告警平台集成。
- `actor_id/tenant_id/system_id/account_id/target_id` 必须来自认证或服务端查到的对象。未认证或不存在的目标允许空，不能把请求伪造的租户 ID 当可信范围。跨租户拒绝事件仅记录已验证且操作者有权知道的范围；无法安全归属时为平台事件，仅超管可见。
- `actor_type=bootstrap` 的初始化事件可以没有 `actor_id`，`create_by` 记录固定运维操作来源；日常用户使用 `user:<内部UUID>`，系统账号使用 `system_account:<内部UUID>`，不得写 JWT、密钥或未经验证的外部用户输入。
- 名称快照取操作时安全可见的数据；`changes` 采用按动作白名单构建的 before/after，限制为必要的编码、名称、状态和角色。禁止放入密码、token、摘要、Cookie、认证头、完整 JWT claims 或整份 HTTP 请求。
- 审计读取只暴露经过范围过滤的公开 DTO；租户管理员不能查询平台事件或其他租户。列表与计数在同一查询范围，分页遵守 `pageNum/pageSize`、`total/list`。
- 不提供审计修改、删除、恢复、物理清理接口。本期不建立保留期清理任务，后续归档与保留期需单独设计。

## 12. 审核与执行记录

2026-09-14，Owner 明确回复“数据库表没问题，可以继续后续的内容”，同时要求固定密钥、取消本期轮换、签发人必须为租户管理员，并授权前后端真实登录开发。本版在已批准七表模型内补充本地登录名和密码摘要，保留单系统单账号、软删除、并发版本与追加审计约定。

SQL 始终以第 7 节为唯一来源。若曾执行旧草案，不能重复运行建表事务或假设旧结构兼容，应先比对现有结构，再准备明确的升级 SQL。普通服务启动不执行 DDL。验证使用隔离测试数据库，不能把测试结果称为已创建业务数据库。
