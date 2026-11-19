---
name: agent-cli-usage
description: Use the SuperAcademy sa2 CLI to check the signed-in identity, sign in by email or Feishu, or clear the local session for a chosen SA2 environment. Use for SA2 account and session tasks that call for login, whoami, or logout.
---

# 使用 sa2 CLI

## 选择入口和环境

- 使用已安装的 `sa2`。使用源码时先按仓库 README 构建并使用生成的二进制。
- 使用任务授权的 SA2 API origin。优先级为 `--api-url`、`SA2_API_URL` 环境变量、用户配置目录中的 `sa2-cli/config.json`。配置文件首次运行时创建，默认地址为 `https://api.mentorx.org`。HTTP 仅用于环回地址，其他地址使用 HTTPS。省略认证信息、路径、查询和片段。
- 使用当前 OS 账户可访问的系统凭据库。Linux 无桌面宿主需要预配持久加密、已解锁的 Secret Service。

## 执行任务

- 用 `sa2 whoami` 查看当前身份的 `id`、`email` 和 `roles`。每次调用都会刷新并轮换令牌。只在任务确实需要身份检查时执行。
- 邮箱登录使用 `sa2 login --email "$SA2_ACCOUNT_EMAIL"`。有终端时由用户通过隐藏输入完成。自动化加上 `--password-stdin`，由宿主的秘密管理器把密码直接送到子进程的 stdin 并关闭该流。密码只经隐藏输入或 stdin 提供。
- 飞书登录使用 `sa2 login --feishu`。由用户在任意电脑或手机打开终端显示的飞书验证链接，核对短码并完成授权。引导用户在自己的终端读取链接，避免把临时链接复制到聊天或日志。
- 用户要求结束 CLI 会话时执行 `sa2 logout`。它只删除所选环境的本地凭据，重复运行仍成功。

## 读取结果

业务 stdout 是单行 JSON。成功读取 `data`，失败依据 `error.code` 选择动作。交互提示来自 stderr，帮助和版本为文本。

| 退出码 | 处理 |
| --- | --- |
| `0` | 登录和身份查询返回身份信息，退出返回 `{"loggedOut":true}`。 |
| `1` | `CONFIG_ERROR` 时检查配置文件的 JSON 格式和访问权限。 `CREDENTIAL_STORE_UNAVAILABLE` 时检查宿主凭据库及当前运行身份。 `SESSION_BUSY` 时等当前命令结束后再尝试。 `API_UNAVAILABLE` 时检查 API 连通性。 `SESSION_STORAGE_ERROR`、`INVALID_API_RESPONSE` 或 `INTERNAL_ERROR` 时报告命令名、错误码及退出码。 |
| `1` | `AUTH_TIMEOUT` 时重新运行飞书登录命令。 `RATE_LIMITED` 时等待后再尝试。 |
| `2` | `INVALID_ARGUMENT` 时核对 API origin、参数及密码输入模式。 |
| `3` | `LOGIN_REQUIRED` 表示会话缺失或无法确认续期结果。任务已授权且凭据可用时重新登录。 `AUTH_REJECTED` 时核对账号权限及登录方式。 |
| `130` | `CANCELLED`，结束当前操作。 |

汇报时只使用公开身份字段或错误码。凭据由系统凭据库和 CLI 管理。
