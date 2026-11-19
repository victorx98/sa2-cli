# sa2-cli Go 安全重构方案

范围限定在 `sa2-cli`。首版交付 Windows、macOS 和 Linux 的 Go 二进制，使用现有邮箱登录及刷新会话接口。旧版通过提交 `78367fe` 获取，正式切换时先在旧版退出，再在 Go 版重新登录。

## 平台凭据

| 平台 | 飞书 CLI 的参考 | sa2-cli 的实现 |
| --- | --- | --- |
| macOS | 令牌写入 AES-GCM 密文文件，主密钥可在 Keychain 或本地文件。 [源码](https://github.com/larksuite/cli/blob/main/internal/keychain/keychain_darwin.go) | 将刷新令牌直接存入当前用户 Keychain，禁止凭据操作弹窗。 |
| Windows | 当前用户 DPAPI 保护注册表中的密文。 [源码](https://github.com/larksuite/cli/blob/main/internal/keychain/keychain_windows.go) | 使用当前用户的 Credential Manager，采用仅本机持久化的通用凭据。 |
| Linux | 主密钥和 AES-GCM 密文在同一目录。 [源码](https://github.com/larksuite/cli/blob/main/internal/keychain/keychain_other.go) | 使用由宿主预配的持久、加密且已解锁的 Secret Service。SSH、容器和 CI 由独立 OS 账户运行。 |

凭据与普通配置分离。密码和访问令牌由进程内存处理。应用目录只保存空锁文件。凭据后端故障时返回 `CREDENTIAL_STORE_UNAVAILABLE`。相同 OS 用户下的程序之间的权限边界由宿主提供。

借鉴飞书 CLI 的结构化结果和 Agent 使用指引。OAuth 设备授权、scopes、机器人身份和 DPoP 由服务端协议决定。Go 版公开三个固定命令。 [飞书 CLI 认证和输出契约](https://github.com/larksuite/cli#authentication)

## 实施顺序

1. 在 `internal/session` 实现系统凭据及 OS 文件锁。以当前用户和规范化 API origin 建立命名空间。跨进程锁保护凭据读取、轮换、写入和删除。进程退出时由 OS 释放锁。适配层使用原生 API，底层错误由命令层映射为固定文案。
2. 在 `internal/api` 实现 `POST /api/auth/login` 和 `POST /api/auth/refresh`。请求只发往显式配置的 HTTPS origin，保留证书验证。禁用重定向和自动重试。用 `tools/contract` 从固定的 API 提交生成两个操作及其引用的 OpenAPI 子集。
3. 在 `internal/auth` 和 `internal/cli` 接通三个命令。 `login` 先检查凭据库，再读取密码，保存成功后输出身份。 `whoami` 在锁内确认无令牌的 `pending` 写入完成后发起一次续期，保存新令牌后返回响应中的身份。结果不明确时要求重新登录，后续进程遇到 `pending` 直接返回。 `logout` 幂等删除 origin 对应的本地凭据。业务 stdout 输出单行 JSON， stderr 用于交互提示。 `cmd/sa2` 提供可执行入口。
4. 更新 README 和 Agent Skill，说明每次 `whoami` 会续期、裸无桌面宿主的凭据服务前提和失败处置。将 npm workspaces 和旧实现从 Go 分支移除。发布流程以 tag 构建二进制、Skill 和校验和，并以原生测试结果为门槛。

## 验收

- 在 `sa2-cli` 执行 `go test ./...`、`go vet ./...`、支持平台上的 `go test -race ./...` 和 `go run ./tools/contract -check`。用固定上游的 `docs/openapi.json` 运行 `go run ./tools/contract -check -source <文件>`。
- 在 macOS、Windows、Linux 的真实凭据库上验证读写、删除和失败映射。Linux 在隔离的无桌面会话中使用真实 Secret Service，验证缺失和锁定时停止操作。用 Windows 目标账户验证存储，包括通用凭据容量限制。
- 使用真实 Go 子进程和模拟 HTTPS API 验证密码不回显、输出无密码和令牌、每个 `whoami` 一次续期、并发进程按顺序使用更新后的令牌、跨 origin 隔离，及进程中断和存储失败后的恢复。验证凭据库失败时不会发送认证请求。
- 按实际 Agent 峰值在预发布环境核验续期开销。三平台原生验收及预发布检查通过后切换默认安装入口。回滚通过恢复既有程序版本并重新登录完成。

## 当前验证记录

- Linux amd64 的 `go test ./...`、`go test -race ./...`、`go vet ./...`、隔离 GNOME Keyring 测试和伪终端取消测试已通过。
- Windows amd64、Windows arm64 的发布源码和测试二进制已交叉编译。Linux arm64 的发布源码已交叉编译。
- 固定上游 blob、OpenAPI 子集、命令帮助和 tag 版本注入均已验证。CI 与发行工作流通过 `actionlint`。
- Windows、macOS 真实凭据库验收及预发布负载检查待执行，正式发布使用这些验收结果。
