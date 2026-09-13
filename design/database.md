# PostgreSQL 数据库初始化与授权

当前文档只负责权限中心业务 PostgreSQL 的数据库角色和权限初始化，不包含任何业务表。租户、系统接入、授权管理和审计等业务表必须先完成领域建模和 Owner 审核，再按根目录 [AGENTS.md](../AGENTS.md) 的规则补充到本文件。

租户表示公司，组织归属租户；接入系统是可被多个租户启用的应用，授权范围绑定租户与系统。具体关系表尚未设计或审批。SpiceDB 使用独立数据库存储关系，其内部表和迁移由 SpiceDB/Operator 管理，不适用本项目业务表的软删除、审计列或命名规则；本服务只通过官方 API 访问它。

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
