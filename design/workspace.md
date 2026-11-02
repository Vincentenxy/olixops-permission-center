# 跨电脑协同开发

后端仓库为 [olixops-permission-center](https://github.com/Vincentenxy/olixops-permission-center)，前端仓库为 [olixops-permission-center-web](https://github.com/Vincentenxy/olixops-permission-center-web)。建议克隆到同一父目录，打开前端仓库的 `permission-center.code-workspace`；路径都相对工作区，不依赖个人电脑盘符。

```sh
git clone https://github.com/Vincentenxy/olixops-permission-center.git
git clone https://github.com/Vincentenxy/olixops-permission-center-web.git
```

每个仓库分别维护自己的 `AGENTS.md`，开发前都要读取受影响仓库的规范。后端 [数据库文档](database.md) 是七表 SQL 的唯一来源，[管理接口文档](management-api.md) 与 [OpenAPI](../api/openapi.yaml) 描述协议。前端仓库携带 `.agents/skills/` 内的 Apple 设计 skill、许可证及使用入口，换电脑无需依赖个人 skills 安装。

前端继续采用已确认的简洁、明了的 Apple 风格。新电脑检查和安装方式统一维护在前端的 [设计 skills 安装与验证说明](https://github.com/Vincentenxy/olixops-permission-center-web/blob/main/.agents/skills/README.md)（本地为同级前端仓库的 `.agents/skills/README.md`），包含固定版本、离线安装、Windows/macOS/Linux 命令与已有版本比较；前端 `AGENTS.md` 会引导 Agent 读取。提交时保留前端仓库的 skills 完整副本、许可证和设计规范，安装不依赖本机个人路径。

启动顺序：准备 PostgreSQL 并按文档执行 SQL → 根据后端 README 生成本地 RSA 密钥并准备连接配置 → 显式运行超管初始化命令 → 启动后端 → 前端安装锁定依赖并启动。登录依赖业务 PostgreSQL 和 JWT 配置，不要求先启动 Redis 或 SpiceDB。

真实密码、RSA 私钥、系统 Token、数据库内容和个人 IDE 配置不提交。Git 保存可复现的源码、依赖锁、文档与安全模板，运行时秘密通过自己的环境变量或挂载文件提供。前后端契约变化需要在两个仓库分别验证、提交，不能只更新其中一方后宣称联调完成。
