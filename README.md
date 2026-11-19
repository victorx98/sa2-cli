# sa2-cli

`sa2` 为 Agent 提供 SuperAcademy 的命令行入口。认证、授权和令牌轮换由 `sa2-api` 处理。

## 安装与运行

从发行页下载对应 macOS、Windows 或 Linux 的归档，核对 `checksums.txt` 后解压。也可以用 Go 1.27.4 从源码构建。在 macOS 上使用原生构建环境和 `CGO_ENABLED=1`，链接系统 Security framework。

发行归档提供 Linux 和 macOS 的 amd64、arm64 版本，以及 Windows amd64 版本。

```sh
go build -trimpath -o sa2 ./cmd/sa2
./sa2 --help
```

Windows 源码构建使用 `go build -trimpath -o sa2.exe ./cmd/sa2`。以下示例把域名换成获准使用的 SA2 API HTTPS origin。

```sh
sa2 login --api-url https://sa2.example.com --email "<邮箱>"
sa2 whoami --api-url https://sa2.example.com
sa2 logout --api-url https://sa2.example.com
```

- `--api-url` 优先于 `SA2_API_URL`。使用纯 HTTPS origin，省略账号凭据、路径、查询和片段。
- 登录使用隐藏终端输入。自动化使用 `--password-stdin`，由宿主的秘密管理器写入 stdin 并关闭该流。需要飞书员工认证的账号沿用 Web 流程。
- 每次 `whoami` 都尝试续期，成功时轮换刷新令牌并返回服务端身份。续期结果不明时要求重新登录。
- `logout` 仅清除指定 origin 的本地凭据，重复执行仍成功。
- 业务命令的 stdout 是单行 JSON。成功读取 `data`，失败读取 `error.code` 和 `error.message`。退出码为 `0` 成功、`1` 运行失败、`2` 参数错误、`3` 认证被拒或需登录、`130` 取消。交互提示来自 stderr。

## 系统凭据库

| 系统 | 运行条件 |
| --- | --- |
| macOS | 当前用户的默认 Keychain 已解锁，且允许当前二进制访问其凭据项。 |
| Windows | 当前运行身份可访问 Credential Manager。通用凭据以本机范围持久化。 |
| Linux | 当前用户可访问已解锁的、持久加密的默认 Secret Service 集合。CLI 通过当前用户的 Unix session bus 连接。 |

刷新令牌只保存在系统凭据库。密码和访问令牌只由 CLI 在进程内存中处理。应用状态目录只保存空锁文件。凭据库未配置、锁定或拒绝访问时，命令返回 `CREDENTIAL_STORE_UNAVAILABLE`。Linux SSH、容器和 CI 宿主需要预配独立 OS 账户、该账户的 Unix session bus 及持久加密的默认集合。通过 `DBUS_SESSION_BUS_ADDRESS` 指定非标准的 `unix:path=` 地址。

Go 版使用独立凭据命名空间。切换前在旧版对使用过的各 origin 执行 `logout`，再在 Go 版重新登录。回滚时用对应版本重新登录。

## Agent Skill

将 [agent-cli-usage](./skills/agent-cli-usage/SKILL.md) 所在目录按 Agent 宿主支持的方式注册，再为宿主提供已安装的 `sa2` 和对应的 OS 凭据库。Skill 说明输入模式、结果解析和失败恢复。

## 开发与发布

从本仓库执行以下检查。集成测试启动本地 HTTPS 模拟 API。

```sh
go test ./...
go vet ./...
go run ./tools/contract -check
```

Linux 原生凭据测试使用隔离的 D-Bus 和 GNOME Keyring。安装这两个测试依赖后，运行 `bash scripts/test-linux-keyring.sh`。CI 在 macOS 和 Windows 上也运行各自的原生凭据测试。

API 来源和更新步骤见 [OpenAPI 契约](./contracts/README.md)。主分支与发行候选检查需能读取固定的 API 提交，私有上游可配置只读 `SA2_API_READ_TOKEN`。维护者打发行 tag 后，CI 创建包含归档、校验和与 Skill 的草稿发行。正式发布前在预发布检查目标宿主的认证链路和峰值续期负载。

实施与验收条件见 [Go 安全重构方案](./docs/plans/active/2026-11-19-go-migration.md)。
