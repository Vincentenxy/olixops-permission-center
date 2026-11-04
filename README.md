# Olixops Permission Center

独立的多租户、多系统权限中心 Go 后端。第一部分管理租户、后台管理员、系统调用身份和审计；第二部分使用 SpiceDB 保存业务授权关系并计算权限。

开发前阅读 [AGENTS.md](AGENTS.md)。数据库唯一 SQL 在 [design/database.md](design/database.md)，接口说明在 [design/management-api.md](design/management-api.md)，进度见 [第一部分计划](design/phase-one-plan.md)。

本次交付基线、待核对环境与后续事项统一记录在 [开发交接清单](design/handoff.md)，接手时先看该清单；实际部署状态与本地验证结果分别记录。

通用业务授权的目标方案见 [核心授权设计草案](design/core-authorization.md)，领域术语见 [CONTEXT.md](CONTEXT.md)。草案覆盖外部系统接入、自用策略及 SpiceDB 模型取舍，尚未实现，不改变下方当前能力清单。

## 当前能力

| 能力 | 状态 |
|---|---|
| Go 1.26、DDD、Gin、grpc-go、GORM/PostgreSQL、go-redis、zap | 已初始化并复用原项目配置 |
| 本地登录 | 用户名密码、bcrypt 摘要、RS256 JWT 签发/验签、每请求检查本地用户状态 |
| 管理员初始化 | 显式离线命令，只允许空用户库；无默认密码和公开初始化接口 |
| 租户与管理员 | 创建、编辑、查询、停用/恢复、租户管理员分配、最后超管保护 |
| 系统与账号 | 每系统唯一租户、每系统一个调用账号，创建时同事务完成 |
| 固定系统 Token | 每账号最多一把有效 Token，无自动到期/轮换；只有所属租户有效管理员可签发 |
| 管理审计 | 变更和成功审计同事务，失败保留可信操作者；范围过滤、分页与详情 |
| 管理页面配套 | 当前账号实际管理租户分页、Token 签发资格查询、用户/成员按登录账号搜索；接入步骤见 [管理页面后端接入](design/management-pages.md) |
| 系统身份探测 | HTTP `GET /api/v1/systemAccess/me`，验证 Token 及完整上层状态 |
| gRPC | 标准 health 与自定义 Healthy；尚无系统身份业务 RPC |
| SpiceDB | 官方 authzed-go 客户端，可选连接、TLS、超时、启动健康检查；独立权限模型通过本地 37 条断言，业务授权未实现 |
| 独立前端 | [olixops-permission-center-web](https://github.com/Vincentenxy/olixops-permission-center-web)：登录、身份恢复、租户/管理员/系统/固定 Token/审计管理页面；真实环境管理流待部署联调 |

`permission.v1.PermissionCenterService/CheckPermission` 当前只有 proto/生成代码，未注册服务。没有用户注册、公开找回密码、refresh token、跨设备会话撤销或自动建表；这些不能从目录或文档占位推断为已实现。

## 架构与目录

依赖方向：HTTP/gRPC → Application → Domain；Infrastructure 实现存储及外部服务端口，由 Bootstrap 装配。管理用例共用授权范围、事务、版本与审计规则，协议层不直连数据库或 SpiceDB。

```text
olixops-permission-center/
├── AGENTS.md / README.md
├── go.mod / go.sum
├── main.go / cmd/server/main.go       # 共用服务启动过程
├── cmd/bootstrap-admin/              # 空库显式初始化超管
├── cmd/generate-keys/                # 一次生成持久 RSA 密钥，拒绝覆盖
├── config/config.example.yaml       # 安全配置模板
├── api/openapi.yaml                 # 31 条 HTTP 路由的 OpenAPI 3 契约
├── api/proto/ / api/gen/             # proto 和生成代码
├── design/
│   ├── database.md                  # 七表 DDL、建库/授权
│   ├── management-api.md            # 管理 API 和范围约定
│   ├── phase-one-plan.md            # 进度与后续范围
│   └── workspace.md                 # 跨电脑/跨仓库开发
├── internal/
│   ├── bootstrap/                   # 依赖装配、服务生命周期
│   ├── domain/management/           # 实体、错误、存储/事务端口
│   ├── application/management/      # 身份、租户、系统、Token、审计用例
│   ├── application/health/
│   ├── infrastructure/
│   │   ├── persistence/             # GORM 适配、事务、范围查询
│   │   ├── cache/                   # 可选 Redis
│   │   └── spicedb/                 # 官方 Authzed SDK 适配
│   ├── integration/                # 隔离 PostgreSQL 的完整 HTTP 回归测试
│   └── interfaces/
│       ├── http/                    # 路由、DTO、swag、认证/限流/错误
│       └── grpc/                    # health、共享拦截器
├── pkg/
│   ├── auth/                        # JWT 签发/验签、bcrypt
│   ├── credential/                  # 固定 Token 随机生成和摘要比对
│   ├── config/ / logger/
│   ├── page/ / response/
│   └── requestid/ / userctx/
├── Dockerfile / .dockerignore
└── .gitignore                       # 忽略本地配置、PEM、生成 docs
```

租户拥有多个系统，系统不能跨租户迁移；用户与租户多对多。平台超管负责租户和后台用户管理，租户管理员管理自己有效租户内的系统。系统 Token 签发是明确例外：**超管也必须具有目标租户的有效管理员关联**。

后台人类管理员、系统调用账号、接入系统内的业务用户是三种身份。PostgreSQL 七表保存管理状态和审计；SpiceDB 的内部表由其服务/Operator 维护。第一部分不会写入 Schema、关系或预建 outbox。

## 首次运行并登录

以下以 PowerShell 为例，在后端根目录执行。需要 Go 1.26 和已准备好的 PostgreSQL；Redis、SpiceDB 不影响本期登录。

1. 按 [数据库文档](design/database.md) 创建数据库、用户/授权并执行第 7 节完整七表 DDL。服务不会调用 AutoMigrate；旧草案已经执行过时先比对结构，不能重复运行建表事务。
2. 下载 Go 依赖并生成本地 RSA 密钥。命令不会覆盖已有文件；以后重启复用同一密钥。
3. 配置自己的数据库与 JWT，显式初始化首个管理员，再启动服务。

```powershell
go mod download
go run ./cmd/generate-keys --private-file config/jwt-private.pem --public-file config/jwt-public.pem

$env:CONFIG_FILE = 'config/config.example.yaml'
$env:AUTH_JWT_PRIVATE_KEY = [IO.File]::ReadAllText((Join-Path $PWD 'config/jwt-private.pem'))
$env:AUTH_JWT_PUBLIC_KEY = [IO.File]::ReadAllText((Join-Path $PWD 'config/jwt-public.pem'))
$env:AUTH_JWT_ISSUER = 'olixops-permission-center'
$env:AUTH_JWT_AUDIENCE = 'olixops-permission-center-web'
# 通过自己的秘密管理方式设置 DATABASE_DSN，或使用本地 config/config.yaml。
# 不把真实密码粘贴进仓库内的模板或文档。

$env:BOOTSTRAP_USERNAME = Read-Host '首个管理员账号'
$env:BOOTSTRAP_NAME = Read-Host '显示名称'
$env:BOOTSTRAP_PASSWORD = [Net.NetworkCredential]::new('', (Read-Host '初始密码' -AsSecureString)).Password
try {
  go run ./cmd/bootstrap-admin
  if ($LASTEXITCODE -ne 0) { throw '管理员初始化失败，请检查配置、密码规则和空用户库要求' }
} finally {
  Remove-Item Env:BOOTSTRAP_PASSWORD -ErrorAction SilentlyContinue
}
go run ./cmd/server
```

用户名为 3～64 位字母/数字及 `_.@-`，首位为字母或数字，规范化为小写；密码 12～72 字节。初始用户为 `platform_admin`；后续创建用户默认 `none`，由超管分配租户管理员关系。初始化命令重跑会拒绝，不改变已有账号或角色。不得把测试账号当作产品默认账号。

macOS/Linux 使用同一 Go 命令，可将文件内容通过环境变量注入，例如 `export AUTH_JWT_PRIVATE_KEY="$(cat config/jwt-private.pem)"`；密码应从交互输入或秘密管理工具读取，避免进入 shell 历史。PEM 文件和本地配置均被 Git/Docker 忽略，生产通过 Secret 挂载配置或环境注入。

然后进入独立前端仓库，按其 README 运行 `pnpm install --frozen-lockfile` 和 `pnpm dev`，默认代理后端 `127.0.0.1:8080`，使用刚初始化的账号登录。新电脑目录布局见 [workspace.md](design/workspace.md)。

只想验证健康接口时，可直接指定安全模板启动；没有数据库/签发配置时，管理/登录用例返回 503，健康服务仍可用。默认未指定 CONFIG_FILE 时读取被忽略的 `config/config.yaml`，本次未覆盖用户已有文件。

## 配置

优先级：环境变量 > 配置文件 > 默认值。`.env.example` 仅是清单，程序不自动加载 `.env`。YAML 先解析再展开环境占位，避免密码中的 YAML 特殊字符改变结构。显式配置文件不存在、类型错误或密钥不匹配均安全失败，不打印配置内容。

| 环境变量 | 配置字段 | 默认/用途 |
|---|---|---|
| CONFIG_FILE | — | config/config.yaml |
| APP_ENV | app.environment | development；production/release 用 JSON 日志 |
| HTTP_ADDR / GRPC_ADDR | server.http / server.grpc 的 host、port | :8080 / :9090 |
| HTTP_READ_TIMEOUT | server.http.read_timeout | 10s |
| HTTP_WRITE_TIMEOUT | server.http.write_timeout | 30s，同时为 HTTP 用例设置执行时限 |
| HTTP_IDLE_TIMEOUT / SHUTDOWN_TIMEOUT | server.http.idle_timeout / server.shutdown_timeout | 60s / 5s |
| HTTP_TRUSTED_PROXIES | server.http.trusted_proxies | 空；逗号分隔实际代理 IP/CIDR |
| DATABASE_DSN | database.host/port/user/password/name/schema/ssl_mode | 未配置则跳过业务 DB |
| REDIS_ADDR / REDIS_PASSWORD / REDIS_DB | redis.addrs/password/db | 单实例；默认关闭 |
| AUTH_JWT_PRIVATE_KEY | auth.jwt-private-key | RSA 私钥 PEM 或 base64 DER，登录签发用 |
| AUTH_JWT_PUBLIC_KEY | auth.jwt-public-key | RSA 公钥 PEM 或 base64 DER，必须与私钥匹配 |
| AUTH_JWT_ISSUER / AUTH_JWT_AUDIENCE | auth.issuer / auth.audience | 模板为 olixops-permission-center / olixops-permission-center-web |
| AUTH_JWT_TOKEN_TTL | auth.token-ttl | 8h，可设 1s～24h |
| SPICEDB_ENABLED | spicedb.enabled | false |
| SPICEDB_ENDPOINT / SPICEDB_TOKEN | spicedb.endpoint / spicedb.token | 服务地址与后端专用凭据 |
| SPICEDB_INSECURE | spicedb.insecure | false，开发明文连接需显式开启 |
| SPICEDB_CA_FILE / SPICEDB_SERVER_NAME | spicedb.ca-file / spicedb.server-name | 可选可信 CA 与服务端名称 |
| SPICEDB_TIMEOUT | spicedb.timeout | 5s，启动探测和 unary 调用上限 |

旧 `auth.jwt_public_key` 仅兼容读取，新增配置用连字符形式。已启用 PostgreSQL/Redis/SpiceDB 的连接失败会阻止启动；健康接口只说明进程可服务，不等于依赖始终健康或授权正确。

登录限流按已验证的客户端 IP：突发 10 次，每 6 秒恢复一次，返回标准 429。默认不信任任何转发头。经过 Nginx/Higress 时必须将**实际代理对端地址范围**配置为 trusted_proxies，并由边界代理覆盖/正确维护客户端地址头；不要设置任意来源可信。多副本生产需在网关增加统一限流，进程内计数不是分布式限流。

## 接口和认证

| 接口 | 说明 |
|---|---|
| GET /api/v1/pub/healthz | 公开健康检查 |
| POST /api/v1/pub/auth/login | 用户名密码 → accessToken、expiresAt、user |
| GET /api/v1/auth/me | JWT + 实时用户状态 → userId、username、name、platformRole |
| POST /api/v1/auth/tenant/list | 当前账号具有有效管理员关联的有效租户，超管也按关联过滤 |
| POST /api/v1/system/credential/checkCreate | 查询当前固定 Token 签发资格和阻止原因，不产生写入 |
| POST /api/v1/tenant/*、/user/* | 租户、后台用户、成员管理 |
| POST /api/v1/system/* | 系统、账号、固定 Token 管理 |
| POST /api/v1/audit/list、/audit/detail | 范围过滤的审计查询 |
| GET /api/v1/systemAccess/me | 固定系统 Token → tenantId、systemId、accountId、keyId |

完整请求和 DTO 见 [管理接口](design/management-api.md) 与 [OpenAPI](api/openapi.yaml)。成功 `{code:0,msg:"",data:...}`，普通业务失败 code=-1；HTTP 400/401/403/404/409/429/503 等保留真实状态及标准外壳。分页 `pageNum/pageSize` 默认 1/20、最大 200；返回 `total/list`，空列表 `[]`。

JWT 只允许 RS256，并验证 exp/iss/aud；sub 固定内部 UUID。系统 Token 与管理员 JWT 使用独立入口，失败不互相回退。系统 Token 为 `opc_<32位hex keyId>.<43位base64url secret>`，只存 SHA-256 摘要；仅创建响应显示一次，查询不可找回。租户、系统、账号停用或 Token 吊销会影响后续请求。

管理员 JWT 默认 8 小时；前端退出清理当前标签页凭据，未实现跨设备服务端撤销。用户停用后，即使 JWT 尚未到期也被拒绝。系统固定 Token 不受其创建者后来退出登录影响，停用/吊销应操作对应系统、账号或凭据。

## 事务、审计与日志

一期所有管理写事务通过 PostgreSQL `pg_advisory_xact_lock(147,1)` 串行化，事务内复查用户/租户状态；适用于小规模管理写入，后续高吞吐需单独优化。读取使用 Repeatable Read，范围、计数和列表一致。version 拒绝覆盖旧版本，最后有效超管保护与并发签发在同一事务规则下执行。

成功变更与成功审计共同提交；审计失败回滚变更。失败事件独立尝试记录已验证操作者，对不可安全归属的越权目标留空；审计库不可用时只打印安全分类和请求 ID，不宣称失败审计已入库。暂无外部告警平台集成。

日志统一经 `pkg/logger`。HTTP/gRPC 共用 `pkg/requestid`，合法的 1～128 字节可见 ASCII 请求 ID 原样透传，否则生成；响应、日志和审计一致。禁止记录密码、JWT、Cookie、系统 Token、摘要、完整请求/配置或私密 DSN。

## gRPC 与 SpiceDB

```powershell
Invoke-RestMethod 'http://127.0.0.1:8080/api/v1/pub/healthz'
grpcurl -plaintext -import-path api/proto -proto health/v1/health.proto -d '{}' 127.0.0.1:9090 health.v1.HealthService/Healthy
```

Healthy 返回 healthy=true、message=ok；标准 `grpc.health.v1.Health/Check/Watch` 同样公开。gRPC 拦截器共用认证、trace、日志与恢复机制；新增系统业务 RPC 必须复用应用层身份用例并补生成代码，不能把健康接口当作系统 Token 已验证。

SpiceDB SDK 为官方 [authzed-go](https://github.com/authzed/authzed-go)，版本以 go.mod 为准。当前开发集群入口可按实际环境设为 `192.168.1.103:30031`，明文开发连接显式使用 `SPICEDB_INSECURE=true`；生产使用 TLS/可信 CA。该地址不写进安全模板。

启用后的启动健康检查只验证可达和健康，不证明 PSK 有效、Schema 存在或业务权限正确；进程不会自动导入 Playground 内容或 WriteSchema。第二部分再实现系统隔离的授权模型、关系变更、权限查询及跨 PostgreSQL/SpiceDB 的一致性方案。

## 构建与验证

SpiceDB 模型验证独立于业务库运行：安装 zed v1.2.1 后执行 `./authorization/validate.ps1`，或通过 `-ZedPath` 指定安装路径。Schema、测试快照、验证结果与已知隔离边界见 [authorization/README.md](authorization/README.md)。该命令不导入数据到现有服务。

```powershell
go test ./...
go vet ./...
go build ./...
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g main.go -o docs --outputTypes json,yaml
git diff --check
```

`docs/` 为忽略的生成产物；独立 OpenAPI 随源码提交。proto 生成步骤见 [api/proto/README.md](api/proto/README.md)。

可选真实 PostgreSQL 列表回归：设置 `TEST_DATABASE_DSN` 后运行 `go test ./internal/infrastructure/persistence -run TestNativeManagementListQueries -v`。这是只读测试，不建表；未配置时明确跳过。集成环境必须使用隔离测试库，不使用个人集群或生产凭据。

完整 HTTP/事务集成回归使用 `go test -count=1 -v ./internal/integration`。同样读取 `TEST_DATABASE_DSN`，但只接受数据库名以 `_test` 结尾的隔离测试库；测试创建随机 schema，从本仓库数据库文档装载七表 SQL，并在结束时仅清理自己创建的 schema。无需预先创建 public 业务表，未设置 DSN 时明确跳过。该套测试覆盖两个超管并发降权/停用、失败审计操作者及审计故障回滚，源码随仓库提交。

GitHub Actions 已配置普通 Go 检查和 PostgreSQL 17 服务上的独立集成测试；工作流配置存在不代表远程运行已经通过，实际结果以对应提交的 Actions 为准。

本次在隔离原生 PostgreSQL 17.6 中执行七表 SQL，并完成 137 项 HTTP 流程断言：登录、跨租户、签发资格、并发单密钥、停用/恢复/吊销、并发版本与审计回滚。还验证可信代理限流、失败审计操作者、密钥生成防覆盖、JWT 算法/issuer/audience 和密码规则。前端通过真实浏览器登录/刷新/退出及桌面/390px 窄屏检查。完整 Go 测试、vet、Linux amd64 无 CGO 构建与 OpenAPI 路由/引用验证通过。

```powershell
docker build -t olixops-permission-center:local .
docker run --rm -p 8080:8080 -p 9090:9090 olixops-permission-center:local
```

多阶段镜像只含独立二进制与安全模板，真实配置通过环境或文件挂载。首次交付未运行镜像构建或 Kubernetes 部署。2026-10-30 本轮使用 Docker 中的独立 PostgreSQL 18.4 测试容器，真实 HTTP 集成回归通过；测试随机 schema 已清理。未构建应用镜像、未部署集群、未修改业务数据库。前端 Docker/Nginx 模板在前端仓库，正式上线时配置同源 /api 代理、HTTPS 和可信代理范围。
