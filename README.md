# sa2-cli

`sa2-cli` 为 Agent 提供访问 SuperAcademy 的命令行入口。`sa2-api` 负责身份验证、数据访问授权和业务规则。

## 安装与运行

第一版支持 Linux、Windows 和 macOS，需要 Node.js 22.12+。Windows 会话保护使用系统自带的 Windows PowerShell 和当前用户配置文件。

```sh
npm ci
npm run build
npm run --silent sa2 -- --help
```

以下示例使用本地 API 地址。连接部署环境时，替换为该环境的 HTTPS origin。

```sh
npm run --silent sa2 -- login --api-url http://localhost:8094 --email "<邮箱>"
npm run --silent sa2 -- whoami --api-url http://localhost:8094
npm run --silent sa2 -- logout --api-url http://localhost:8094
```

- `--api-url` 可通过环境变量 `SA2_API_URL` 提供。命令行参数优先。HTTP 仅用于回环地址。
- 登录时终端会隐藏密码。非交互环境将密码从调用方的秘密管理器通过标准输入传入，并加上 `--password-stdin`。命令参数中无需提供密码。
- 邮箱登录沿用 SA2 的现有登录策略。需要飞书员工登录的账号应继续使用 Web 登录。
- `whoami` 在访问令牌过期前自动续期。会话失效时重新登录。临时网络或 API 故障后可再次运行原命令。
- `logout` 清除指定 API 环境的本机会话。每个 API origin 保存一个账号。
- 登录相关命令在 `stdout` 输出一行 JSON。成功结果在 `data` 中，失败结果包含 `error.code` 和 `error.message`。退出码 `2` 表示参数错误，`3` 表示认证被拒或需要重新登录，`1` 表示其他执行错误。交互提示走 `stderr`。

## 本机会话

| 系统 | 位置 | 保护 |
| --- | --- | --- |
| Linux | `$XDG_STATE_HOME/sa2-cli`，默认 `~/.local/state/sa2-cli` | 当前用户可访问的目录和文件 |
| macOS | `~/Library/Application Support/sa2-cli` | 当前用户可访问的目录和文件 |
| Windows | `%LOCALAPPDATA%\sa2-cli` | DPAPI 当前用户加密 |

## 开发

`packages/core` 实现认证客户端和本机会话，`packages/cli` 提供命令入口。`packages/sdk` 保留作后续 TypeScript 集成入口。运行 `npm test` 用本地模拟 API 测试真实 CLI 进程。

`sa2-api`、`sa2-ui` 和 `sa2-cli` 各自维护在独立 Git 仓库。更新契约时，从 `sa2-api/docs/openapi.json` 同步到 `contracts/openapi.json`，记录来源版本，再运行 `npm run openapi:generate`。CLI 自动续期需要先部署支持 `POST /api/auth/refresh` 的 API。
