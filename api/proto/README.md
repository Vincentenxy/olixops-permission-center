# Protobuf 代码生成

安装 `protoc`、`protoc-gen-go` 和 `protoc-gen-go-grpc` 后，在项目根目录执行：

```powershell
protoc --proto_path=api/proto `
  --go_out=api/gen --go_opt=paths=source_relative `
  --go-grpc_out=api/gen --go-grpc_opt=paths=source_relative `
  api/proto/health/v1/health.proto `
  api/proto/permission/v1/permission.proto
```

`api/gen` 下的文件由工具生成，禁止手工修改。
