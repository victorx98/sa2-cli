---
name: agent-cli-usage
description: Use the SuperAcademy sa2 CLI to check the signed-in identity, sign in by email or Feishu, or clear the local session for a chosen SA2 environment. Use for SA2 account and session tasks that call for login, whoami, or logout.
---

# 使用 sa2 CLI

## 选择入口和环境

- 使用已安装的 `sa2`。使用源码时先按仓库 README 构建并使用生成的二进制。
- 使用任务授权的 SA2 API origin。优先级为 `--api-url`、`SA2_API_URL` 环境变量、用户配置目录中的 `sa2-cli/config.json`。缺少配置文件时，默认地址为 `https://sa2.mentorx.org`，并在用户配置目录中创建配置文件。HTTP 仅用于环回地址，其他地址使用 HTTPS。省略认证信息、路径、查询和片段。
- 本地开发使用 `sa2 --dev <命令>` 直连 `http://localhost:8094`，登录及后续命令都使用这个全局选项。它与 `--api-url` 互斥，本地 HTTP 使用邮箱登录。飞书扫码需要支持 `/auth/cli` 的 HTTPS 站点。
- 使用当前 OS 账户可访问的系统凭据库。Linux 无桌面宿主需要预配持久加密、已解锁的 Secret Service。

## 执行任务

- 用 `sa2 whoami` 查看当前身份的 `id`、`email` 和 `roles`。每次调用都会刷新并轮换令牌。只在任务确实需要身份检查时执行。
- 邮箱登录使用 `sa2 login --email "$SA2_ACCOUNT_EMAIL"`。有终端时由用户通过隐藏输入完成。自动化加上 `--password-stdin`，由宿主的秘密管理器把密码直接送到子进程的 stdin 并关闭该流。密码只经隐藏输入或 stdin 提供。
- 飞书登录使用 `sa2 login --feishu`。由用户在运行 CLI 的电脑上打开终端显示的链接，再用飞书 App 扫描 `sa2-ui` 页面上的二维码。完成授权后在终端查看结果。引导用户在自己的终端读取链接，避免把临时链接复制到聊天或日志。
- 用户要求结束 CLI 会话时执行 `sa2 logout`。它只删除所选环境的本地凭据，重复运行仍成功。

## 读取结果

业务 stdout 是单行 JSON。成功读取 `data`，失败依据 `error.code` 选择动作。交互提示来自 stderr，帮助和版本为文本。

| 退出码 | 处理 |
| --- | --- |
| `0` | 登录和身份查询返回身份信息，退出返回 `{"loggedOut":true}`。 |
| `1` | `CONFIG_ERROR` 时检查配置文件的 JSON 格式和访问权限。 `CREDENTIAL_STORE_UNAVAILABLE` 时检查宿主凭据库及当前运行身份。 `SESSION_BUSY` 时等当前命令结束后再尝试。 `API_UNAVAILABLE` 时检查 API 连通性。 `SESSION_STORAGE_ERROR`、`INVALID_API_RESPONSE` 或 `INTERNAL_ERROR` 时报告命令名、错误码及退出码。 |
| `1` | `AUTH_TIMEOUT` 时重新运行飞书登录命令。 `CALLBACK_UNAVAILABLE` 时检查本机环回接口能否监听。 |
| `2` | `INVALID_ARGUMENT` 时核对 API origin、参数及密码输入模式。 |
| `3` | `LOGIN_REQUIRED` 表示会话缺失或无法确认续期结果。任务已授权且凭据可用时重新登录。 `AUTH_REJECTED` 时核对账号权限及登录方式。 |
| `130` | `CANCELLED`，结束当前操作。 |

## 业务错误（退出码 1）

业务失败也用退出码 1，但错误码来自服务端或命令本身，不是认证问题：

| 错误码 | 含义 | 动作 |
| --- | --- | --- |
| `PLAN_BLOCKED` | 预览被业务阻断，`error.blockers` 列出阻断码 | 按对应业务 skill 处理。该响应的 `requestId` 属于本次预览，不能代作已有操作的回执 ID |
| `OUTCOME_UNKNOWN` | 提交结果不明，带原 `requestId` | 用该 requestId 查 `sa2 sessions operation --request-id …`；不要换键重发 |
| `WAIT_TIMEOUT` | 等待同步超时，带 `sessionId` 和最后状态 | 继续只读跟踪并报告标识，交人工 |
| `SYNC_FAILED` / `SYNC_NEEDS_REVIEW` | 平台写入已受理，外部会议同步失败或待人工 | 保留 `sessionId` 与 `requestId` 交人工；不要另建课次 |
| `BOOKING_API_NOT_DEPLOYED` | 目标环境未部署预约接口 | 报告该环境缺少的能力，保留已准备的信息 |
| `ORIGIN_MISMATCH` / `BOOKING_NOT_CONFIGURED` | 预约 API 的 origin 或签名环境配置不正确 | 报告环境和错误码，交环境维护者核对 |
| `FORBIDDEN` | 当前身份无权执行该操作 | **不要**重新登录、换账号或换环境重试；报告所缺权限 |
| `NOT_FOUND` | 对象不存在或不在授权范围 | 不要换身份扩大查询 |
| `CONFLICT` | 服务端未细分的业务冲突（常见于权益或财务锁类异常） | 读回课次与预览核对，报告冲突项 |
| `PLAN_EXPIRED` | 计划超过有效期 | 重新预览并重新取得授权 |
| `STALE_SESSION` | 课次版本已变 | 先读回最新课次再重新计划 |
| `IDEMPOTENCY_MISMATCH` | 同一 requestId 的载荷已变 | 不要换键重发；先确认是否另起一笔 |
| `PLAN_EFFECTS_CHANGED` | 预览与提交之间业务影响已变 | 重新预览并重新取得授权 |
| `INVALID_SEARCH` | 人物搜索词为空或超过 100 字符 | 缩短关键词后重试 |
| `INVALID_BOOKING_INPUT` | 服务端拒绝了请求内容（常见于标题超过 255 字符、取消原因超过 100 字符） | 核对对应业务 skill 的字段要求，修正后重新预览 |
| `INVALID_PLAN` | 计划令牌被篡改或损坏，服务端无法校验 | 重新预览生成新计划，不要手工编辑计划文件 |
| `OPERATION_NOT_FOUND` | 该 requestId 尚无可见回执 | 可能原事务还没提交，按 requestId 继续只读查询；不要据此新建课次 |
| `API_ERROR` | 服务端返回了无法识别的错误 | 报告命令名、错误码与退出码，交人工 |

其余业务码（如 `TIME_CONFLICT`、`INSUFFICIENT_BALANCE`、`SESSION_BILLING_LOCKED`、
`MENTOR_PRICE_MISSING`、`ENTITLEMENT_AMBIGUOUS` 等）可能直接出现在 `error.code`。按对应业务 skill 处置，结果不明的提交始终先查原回执。

退出码 2 的 `INVALID_ARGUMENT` 表示命令参数或本地校验不通过，不会发到服务端：ID 格式、时区
白名单、时长范围、日期顺序与 7 天窗口、起始时间的偏移与 `--timezone` 不匹配都归此类。
退出码 2 的 `INVALID_PLAN_FILE` 表示计划文件不可用或与本账号、origin、动作不匹配；
`PLAN_FILE_EXISTS` 表示 `--out` 目标已存在，换新文件名重新预览。两者都只在登录成功后才会出现——
未登录时业务命令先返回退出码 3 的 `LOGIN_REQUIRED`。

汇报时只使用公开身份字段或错误码。凭据由系统凭据库和 CLI 管理。
