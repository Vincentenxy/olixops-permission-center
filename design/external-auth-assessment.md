# 对外接入与认证评估

评估日期：2026-11-01。代码基线：`8514c4e`。本文是代码现状分析与后续建议，不代表接口、配置或部署已经完成，也不改变已确认的一期范围。

## 结论

现有设计可以继续用于自有管理后台，以及通过业务后端接入的服务调用。但目前还不满足完整的 Web、HTTP、gRPC 与 SDK 对外交付：管理后台 HTTP 认证已经实现，系统 HTTP 只提供身份探测，gRPC 只有公开健康接口，SDK 尚未形成可分发产品。补齐工作主要在协议接入、身份一致性、传输保护和客户端契约，不需要先实现 SpiceDB 权限计算。

用户已确认业务浏览器直接调用权限中心业务接口，并且已有统一登录中心/SSO。因此目标需要第三类认证：由现有 SSO 提供业务用户身份，权限中心验证面向自身 API 的凭据与调用范围。现有后台管理员 JWT 和固定系统密钥继续服务各自场景。SSO 协议与令牌能力等未定信息集中记录在 [handoff.md](handoff.md)。

## 已有的两种认证

| 身份 | 凭据与校验 | 当前可用入口 | 能力边界 |
|---|---|---|---|
| 后台管理员 | 本地用户名密码、bcrypt；签发 RS256 JWT，校验签名、exp、iss、aud，HTTP 固定取 sub；应用用例重新检查用户与管理范围 | HTTP 登录、当前用户和管理接口 | 适合权限中心自己的后台；不是接入系统业务用户的统一登录中心 |
| 系统调用账号 | 随机固定 Token，格式为 opc_ 前缀；只存摘要，签发明文仅显示一次；每次调用检查凭据、账号、系统和租户状态 | HTTP GET /api/v1/systemAccess/me | 可以作为服务 SDK 的认证基础；目前没有权限查询等业务接口，也没有对应业务 RPC |

两者均使用 `Authorization: Bearer <credential>`，但认证入口明确分开，失败不互相回退。Cookie 参数进入请求上下文不代表支持 Cookie 登录。当前未实现 OAuth2/OIDC、客户端凭据交换、HMAC 请求签名或 mTLS 客户端身份。

系统 Token 只证明调用系统及其租户归属，不证明请求中的业务 userId，也不自动授予授权、撤权等操作能力。后续接口必须从认证结果确定系统范围，校验主体、资源与操作，不能直接信任客户端传入的 tenantId/systemId/userId。

固定 Token 无自动到期、不轮换、每账号最多一把有效密钥是已确认的一期选择，可以用于妥善保管凭据的服务端。需要吊销后显式重建；若未来要求多调用方独立追踪、平滑轮换或自动到期，属于新增产品范围，需要重新确认，不在本轮擅自扩展。

## 按调用场景判断

| 场景 | 当前判断 | 对外交付前的工作 |
|---|---|---|
| 自有管理 Web，经同源代理调用 HTTP | 已有认证基础与登录页面 | 核对实际 HTTPS 部署、持久密钥、管理员初始化和代理配置；完整管理页面另行推进 |
| 外部业务 Web → 自己的后端 → 权限中心 | 可保留为服务端调用模式；用户已明确还要求浏览器直连 | 在业务后端使用系统 SDK；补系统 HTTP/gRPC 接入，明确业务主体的可信传递与系统隔离 |
| 服务 → HTTP | 只有系统身份探测可用 | 提供稳定系统接口、错误契约、超时与 SDK；业务权限查询待第二部分 |
| 服务 → 原生 gRPC | 尚不具备系统业务调用能力 | 接入系统认证应用用例、注册业务 RPC、补状态映射和跨协议测试 |
| 浏览器 → 权限中心业务 HTTP | 已确认为目标；已有 SSO，但仓库尚未接入 | 核对 SSO 是否能签发面向权限中心 API 的 access token，补业务用户认证、系统范围映射与跨域支持；固定系统密钥不能下发浏览器 |
| 浏览器 → gRPC | 当前无 gRPC-Web 接入 | 若有真实需求，增加 gRPC-Web 兼容代理与浏览器客户端；可先让浏览器使用 HTTP |

浏览器使用带 Authorization 的跨域请求会触发预检；当前路由没有 CORS/OPTIONS 支持，同源代理则无需这项能力。若支持跨域，应明确允许的来源、方法、请求头以及请求 ID 响应头暴露；OPTIONS 作为基础设施预检需要在仓库约定中说明，业务接口仍保持 GET/POST。CORS 不能替代身份验证。[MDN CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS)

浏览器接入 gRPC 不能直接照搬服务端原生客户端；官方 Web 示例通过 Envoy 代理转发 gRPC-Web 请求。[gRPC Web 官方说明](https://grpc.io/docs/platforms/web/basics/)

## 已确认场景：浏览器直连，复用已有 SSO

### 建议的认证职责

| 调用方 | 凭据 | 权限中心需要检查的内容 |
|---|---|---|
| 自有管理后台 | 现有本地管理员 JWT | 本地用户状态、固定后台角色、有效租户管理员关联 |
| 业务浏览器 | 面向权限中心 API 的 SSO 用户 access token；仅在原 SSO 无法直接支持时另设计受控委托凭据 | 可信身份源、令牌用途与有效期、业务用户标识、可访问系统/租户和允许操作 |
| 服务端 HTTP/gRPC SDK | 现有系统固定 Token | 凭据、账号、系统、租户状态，以及具体接口的操作范围 |

以上是目标设计，仓库当前只完成管理员 HTTP 认证与固定系统 Token 的 HTTP 身份探测，SSO 认证尚未实现。接入 SSO 不代表给所有 SSO 用户授予后台管理权。应分别定义管理员、业务用户与系统接口的认证策略，不改写现有 auth 配置来让管理员入口接受任意 SSO 用户。

### 首选路径：SSO 签发面向权限中心的访问令牌

若现有登录中心支持 OAuth2/OIDC，可将权限中心登记为受保护 API。浏览器通过 SSO 获取其 audience 包含权限中心、且权限范围受限的 access token，再直接发送 HTTPS 业务请求。作为公开客户端的浏览器采用授权码加 PKCE 流程，不能依靠保存在浏览器中的 client secret 证明自身为可信服务。[OAuth 2.0 Security BCP](https://www.rfc-editor.org/rfc/rfc9700.html)

权限中心是此访问令牌的资源服务器：验证签名或令牌状态、issuer、audience、有效期和约定的 scope。若采用 JWT access token，应按身份源明确的令牌规范验证用途与声明；ID token 用于登录客户端确认认证结果，不能直接当成业务 API 的 access token。用户已登录某个业务系统，也不代表该系统获得的任意 Token 都能调用权限中心。[JWT Access Token Profile](https://www.rfc-editor.org/rfc/rfc9068.html)、[OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html#IDToken)

如果 access token 是 JWT，优先评估可信发现/JWKS 与受控密钥更新；固定 RSA 公钥也需要明确更新方式。发现地址和 JWKS 地址必须由服务端信任配置确定，不能取请求中的任意地址。如果是 opaque access token，需由后端调用受认证保护的 introspection，检查 active、受众、主体和范围，并传播超时/故障；不能因为 Token 不是 JWT 就当成现有系统固定 Token。[OAuth Token Introspection](https://www.rfc-editor.org/rfc/rfc7662.html)

若现有 SSO 只有登录 Cookie、SAML 或只为其他 API 签发 Token，需与 SSO/业务后端设计受控的身份桥接或令牌交换，签发仅面向权限中心且范围受限的短期用户凭据。浏览器仍可在获取该凭据后直接访问权限中心；不能把给其他受众的 Token 直接改为接受，也不能用浏览器提交的 userId 换取任意身份。

### 必须确定的作用域规则

- 业务主体使用可信身份源与稳定用户标识，例如 `(issuer, sub)`；具体如何映射到各系统业务主体，需与现有 SSO 的主体规则核对。不同客户端的 sub 未必相同，也不能拿用户名、邮箱直接当成稳定唯一标识。
- 系统归属从身份源明确的可信声明、已登记 OAuth 客户端映射或受控委托中确定，并结合用户的允许范围检查。请求 systemId 只能作为目标参数；公开 clientId、Origin 白名单或 PKCE 都不能单独授予业务权限。
- 浏览器默认按已验证当前用户查询自身权限；查询他人、列举敏感授权关系、批量枚举资源、授权/撤权必须另有明确操作权限。不能把服务端接口原样暴露给业务用户。
- scope 表示允许调用的 API 能力，具体资源权限仍由权限中心业务规则计算。SSO 登录成功不能替代资源授权；浏览器计算结果用于页面体验，业务后端仍负责实际执行允许或拒绝。
- SSO 退出、用户停用和已签发 Token 的失效并非天然同步。需确定可接受的失效时限、是否 introspection 或撤销通知；JWKS 缓存属于验证密钥管理，不代表允许缓存权限计算结果。

### 需要补充的配置类别

以下是设计类别，尚不是可直接填写的现有配置键：

| 所在方 | 配置类别 |
|---|---|
| 现有 SSO | 权限中心 API 受众/资源登记、业务 Web 公开客户端、精确回调地址、允许申请的 scope、令牌有效期与用户状态策略 |
| 权限中心业务用户认证 | 独立于本地管理员的可信 issuer、允许 audience、算法/令牌用途、JWKS/发现或 introspection 配置、有限网络超时、主体与系统范围映射 |
| 权限中心或边界代理 | HTTPS、精确 CORS 来源白名单、OPTIONS、允许 Authorization/Content-Type/x-request-id、暴露响应请求 ID、按用户和系统的限流 |
| 浏览器 SDK | 权限中心 baseURL、timeout、access token 获取回调；登录/更新流程与既有 SSO SDK 集成。配置公开 clientId 时不将其当成秘密或授权依据 |
| 服务端 SDK | HTTP/gRPC endpoint、系统 Token、TLS 信任与 timeout；与浏览器 SDK 使用独立的凭据接口 |

当直接使用 Bearer 用户令牌时，无需为了认证额外引入跨站 Cookie。若身份方案选择 Cookie，则还需设计 SameSite、CSRF 与带凭据 CORS；不能混用两种模型的配置假设。业务 Web 跨域是此目标下应准备的能力，同源部署可以不启用跨域放行。

## 必须先补齐的协议差异

1. **gRPC 尚未接入系统认证。** `NewServer` 仅接收配置，未装配管理应用服务；拦截器仅处理 JWT，不能验证现有固定系统 Token。应复用 `AuthenticateSystem`，共同检查完整归属链，不能复制另一套 Token 逻辑。
2. **gRPC 的 JWT 主体策略与 HTTP 不一致。** HTTP 使用 `NewLocalVerifier` 固定取 sub；gRPC 使用 `NewVerifier`，优先读取旧 staffuserid。新增管理员业务 RPC 前应统一为本地身份策略，并在应用层复查有效用户和操作范围。验签成功不能单独作为管理授权。
3. **缺少业务 RPC 和明确的按方法认证策略。** 当前只注册标准 health 和 Healthy；CheckPermission 只有 proto/生成代码。应明确公开健康、管理员接口、业务用户接口和系统接口各自接受的身份，禁止尝试多种身份直到成功。不同协议最终调用同一应用用例。
4. **需要保持错误与时限语义。** HTTP 保留现有标准外壳和真实状态；gRPC 使用原生状态码。明确无效凭据、范围拒绝、依赖不可用、超时/取消，SDK 不得把服务故障转成允许。当前 gRPC 认证配置错误映射 Internal，业务接入时需统一故障分类；还需确定服务端最大执行时限，不能只依赖调用方主动设置 deadline。

目前 gRPC 只开放健康检查，因此以上差异是新增业务 RPC 前的缺口，不能描述为已经开放业务接口的实际越权事件。若将来提供长连接流式业务 RPC，还需定义建立连接之后的停用、撤销和过期生效策略；现有 Stream 拦截器只在建立调用时认证。

## 配置是否足够

| 配置/责任方 | 当前现状 | 建议 |
|---|---|---|
| 后端 JWT 私钥、公钥、issuer、audience、TTL | 已支持，单签发方/单 audience/单 RSA 密钥；默认 8 小时 | 足够自有后台登录；服务 SDK 不需要这些签发密钥。audience 表示令牌的预期接收方，不必按 HTTP/gRPC 或每种 SDK 语言拆分；当前名称带 web 不会导致协议不兼容 |
| 系统 Token | 后台签发并由数据库管理，不是全局共享配置 | 每个接入系统使用自己的 Token，服务 SDK 从自己的秘密配置读取；不在前端构建变量、URL 或日志中出现 |
| 业务用户 SSO | 已确认外部存在 SSO，当前仓库无对应认证与配置 | 增加独立信任策略和主体/系统范围映射；具体配置取决于已核对的协议和 access token 能力，不覆盖现有本地管理员配置 |
| 外部 HTTPS / gRPC TLS | 应用目前监听明文 HTTP/gRPC；没有入站 TLS/mTLS 配置 | 可由 Ingress/网关终止 TLS，并明确网关至服务的网络保护或再加密；若应用直面外部，则需新增应用 TLS。无需同时强制两种部署模式，但必须验证选定路径 |
| SpiceDB TLS、CA、Token、timeout | 已有，作用于权限中心连接 SpiceDB | 属于后端出站依赖，不能替代外部 SDK 连接权限中心的 TLS/认证；SpiceDB Token 不分发给接入方 |
| CORS | 当前没有；自有前端通过同源代理 | 已确认业务浏览器直连，需准备跨域来源白名单与预检能力；实际同源部署时可关闭跨域放行 |
| 请求执行时限 | HTTP 有执行时限，SpiceDB 有调用时限；gRPC 未配置服务端统一最大执行时限 | 为系统 RPC 确定有界时限；SDK 允许配置 timeout 并传播取消 |
| 代理与限流 | trusted_proxies 和单实例登录限流已有 | 对外部署需核对代理可信范围；按系统/接口定义业务限流与配额，生产多副本限流可放网关，避免仅用进程内登录计数保护业务 API |
| SDK 配置 | 尚无独立、可分发 SDK | 服务端配置 endpoint、系统 Token、TLS 与 timeout；浏览器配置 baseURL、timeout、用户 access token 获取回调，不能要求系统密钥 |

Bearer 凭据需要受保护的传输；TLS 和应用层 Token 是互补关系。gRPC 官方支持将传输凭据与调用凭据组合使用，不必为了支持 gRPC 另造一套系统密钥格式。[gRPC Authentication](https://grpc.io/docs/guides/auth/)

未实现 refresh token、跨设备会话撤销和 SSO 是已记录边界。若当前使用者是自有后台管理员，加上有限 JWT 有效期与实时用户状态检查，可继续一期方案；若产品需要统一登录或立即撤销单个会话，则应单独设计。不能仅增加一个 issuer 配置就宣称接入任意外部身份源。

## 建议的下一步范围

先核对已有 SSO 协议与 Token 契约，确定业务用户与系统的可信映射；在不实现 SpiceDB 权限计算的前提下设计独立业务用户认证与当前身份接口。并行规划 CORS/TLS、浏览器 SDK 凭据提供接口，以及 HTTP/gRPC 共用的系统认证、系统身份探测 RPC、错误、请求 ID、取消与时限。管理 HTTP 不必因为提供系统 gRPC 而全部再复制为 RPC。

SDK 应有明确的服务端使用说明、凭据隔离、连接复用/关闭、敏感字段脱敏、版本与兼容约定。只读调用可按策略有限重试；签发密钥、授权变更等写操作不得默认自动重试，须先确定幂等语义。OpenAPI、proto 与生成代码是基础契约，不等同于已完成 SDK。

认证回归应覆盖 HTTP/gRPC 同一身份、三类凭据互相误用、凭据吊销、账号/系统/租户停用、跨系统参数、依赖不可用、超时/取消和敏感数据不泄漏。浏览器路径还需覆盖错误受众/身份源、ID token 被误当 access token、密钥更新/未知 kid、introspection 故障（选用时）、伪造用户或系统参数、scope 不足与 CORS 预检。浏览器直连与已有 SSO 已确认；具体 SSO 协议、SDK 首批语言、流式 RPC 和部署入口仍待核对。

之后再接入核心业务：权限查询、关系写入、授权/撤权的操作权限和一致性。这些需要核心模型设计，当前不以健康检查或身份探测代替完成。

## 代码依据与本轮验证

- [HTTP 路由](../internal/interfaces/http/router.go) 与 [JWT 中间件](../internal/interfaces/http/middleware/auth.go)。
- [JWT 校验器](../pkg/auth/verifier.go)、[系统认证用例](../internal/application/management/system.go) 与 [管理范围规则](../internal/application/management/service.go)。
- [gRPC 装配](../internal/interfaces/grpc/server.go) 与 [gRPC 拦截器](../internal/interfaces/grpc/interceptor.go)。
- [服务启动与监听](../internal/bootstrap/run.go)、[安全配置模板](../config/config.example.yaml) 与 [配置说明](../README.md)。

本轮只修改分析文档；`go test -count=1 ./pkg/auth ./pkg/credential ./internal/interfaces/grpc ./internal/application/management` 通过。未进行真实外部 TLS、浏览器跨域、业务 gRPC 或 SDK 端到端验证，不能据此宣称已经具备完整对外接入能力。
