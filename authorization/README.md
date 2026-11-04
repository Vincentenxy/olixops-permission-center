# SpiceDB 权限模型验证

状态：离线模型已验证，业务授权服务尚未实现。Schema 唯一源码为 [schema.zed](schema.zed)，设计见 [核心授权方案](../design/core-authorization.md)。

## 执行

安装官方 [zed v1.2.1](https://github.com/authzed/zed/releases/tag/v1.2.1)，将其所在目录加入 PATH。仓库不保存平台二进制，不读取本地配置、系统密钥或集群地址。

从任何目录执行 PowerShell 脚本（以下相对路径假定当前在仓库根目录）：

```powershell
./authorization/validate.ps1
# 未加入 PATH 时显式指定本机安装路径：
./authorization/validate.ps1 -ZedPath E:/software/zed/zed.exe
```

Linux/macOS 在仓库根目录执行：

```sh
zed validate --fail-on-warn authorization/schema.zed authorization/tests/*.yaml
```

脚本使用绝对路径定位仓库内文件；任一警告或断言失败即失败，不依赖个人电脑盘符。YAML 的 schemaFile 相对各测试文件解析，所有快照引用同一份 Schema。规则和工具行为参考 [官方验证文档](https://authzed.com/docs/spicedb/modeling/validation-testing-debugging)。

## 测试语义

同租户的系统 A/B、另一租户的系统 C 分别保存独立主体与授权；alice 是示例外部账号在各系统中的独立对象。bob 经父目录获得权限，carol 经 folder 类型范围获得权限。示例名称代替内部 UUID，仅供测试；生产映射仍由权限中心生成。

| 文件 | 断言目标 |
|---|---|
| 01-baseline | 直接/角色/父目录多级继承/类型范围及新登记资源允许；跨系统、跨租户、错误动作、陌生主体、未配置继承的资源拒绝 |
| 02-direct-revoked-role-remains | 撤销直接授权后，角色来源仍允许 |
| 03-role-member-revoked-direct-remains | 移除角色成员后，直接来源仍允许 |
| 04-all-alice-sources-revoked | 移除 alice 所有来源后，本目录和下级都拒绝，其他用户不受影响 |
| 05-scope-revoked | 移除范围授权后，现有、新登记、下级资源全部失去该来源 |
| 06-parent-revoked | 移除父目录授权后，子孙失去继承权限，范围来源不受影响 |
| 07-invalid-write-boundary | 故意写入跨系统直接授权、跨租户角色、跨系统继承和跨动作继承，证明它们会导致允许 |

**07 的 assertTrue 是风险复现，不是期望开放的产品行为。** 当前通用 Schema 只做类型约束，不保证主体、角色、access 具有相同系统归属，也不理解业务动作名。system 关系不会隐式参与 allowed。必须封闭 SpiceDB 写入口，并在应用层拒绝非法关系；这些将由后续应用层拒绝测试覆盖。

Schema 同样不保证单一归属、对象存在完整性、父链无环/深度、成员停用或管理员的授权资格。不得把本测试通过视为这些防护已实现。原始 SpiceDB 凭据仅由后端持有，不能交给接入方。

## 撤权与继承边界

每个 YAML 是独立的最终状态快照，不会把 01 的数据带入 02。通过移除目标关系验证撤权后的计算结果；不模拟在线 DELETE、写入顺序或一致性时序。关系是集合：只要直接、角色、类型范围或父目录任一来源仍允许，最终就允许。

同一 access 可以有一个业务父级和一个类型 scope；两者由应用层区分来源并限制数量。首次登记、动作新增必须显式补齐继承关系。read 不包含 update，系统归属本身不授予任何动作。

## 本次结果及剩余验证

2026-11-05 使用本机 zed v1.2.1，Schema 编译及 7 组快照、37 条权限断言全部通过，并启用 --fail-on-warn。使用官方本地验证引擎，无需启动 Docker 或连接现有服务。

另临时移除继承公式进行回归探测：继承断言失败、脚本以非零状态退出；恢复公式后，从仓库外工作目录执行仍全部通过。故意破坏的公式未保留。

尚未执行：真实 SpiceDB gRPC 写入/删除、fully_consistent/跨客户端撤权、并发/故障恢复、worker 门禁、应用层非法写入拒绝、循环和深度上限拒绝、LookupResources/Bulk 契约及目标规模压测。本机 Docker 引擎未运行；未修改现有集群和数据库。此结果不代表端到端业务鉴权、隔离防护或性能已验收。
