# 飞书 CLI 设备授权

CLI 使用 `sa2 login --feishu` 启动飞书原生设备授权，并通过 SA2 API 轮询。验证链接及短码写入 stderr，设备会话存入现有系统凭据库。保留邮箱登录、续期和本地退出。

验收包括模拟 API 子进程测试、`go test -race ./...`、`go vet ./...` 和 OpenAPI 子集检查。在无浏览器环境中使用预发布 API 和测试飞书账号完成真实扫码及续期后，再发布新 CLI。
