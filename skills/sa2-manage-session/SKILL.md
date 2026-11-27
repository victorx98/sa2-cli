---
name: sa2-manage-session
description: 使用 SuperAcademy sa2 CLI 查询已存在的一对一课次、核对会议进度、为获授权课次改期或课前取消。适用于已有预约的跟进和变更。
---

# 跟进和变更已有课次

先用本机 `sa2 sessions --help` 核对版本能力；缺少所需命令时报告缺口。已知 requestId 而未知 sessionId 时，优先查询 `sa2 sessions operation --request-id …`。按用户授权的 origin 操作，认证沿用 `agent-cli-usage`。不要因查不到记录就换账号、换环境或重新约一节。

新版网页与 CLI 共用改期和课前取消的预约 API。网页的课节资料编辑、已完成时长修正和课后取消有独立入口。在 CLI 中遇到这些请求时说明可用的网页流程；涉及财务锁或申诉则交相应负责人。

## 查课与区分来源

- 使用 `sa2 sessions list --student-id … --from … --to …` 缩小候选，再以 `sa2 sessions get --id …` 核对具体学生、导师、课型、时区、当前时间和状态。同日多节或时间含糊时请用户选定。属于 `class_session`、返回 `readOnly=true`、对象不可访问或已删除时，停止写入流程。只查状态无需准备变更。
- `pending_meeting` 表示已创建课次，`scheduled` 表示已排课。已完成、失败、取消各按实际状态报告。旧课可能缺少时区、课型或时长；标记未知，必要时以明确标识的显示时区展示已知时间。

## 阻断与错误码

先读 stdout 的 `error.code`。当值为 `PLAN_BLOCKED` 时，再读 `error.blockers`。对象、权限、环境配置和部分权益异常会直接返回业务码。

| 类别 | 阻断码 | 处置 |
| --- | --- | --- |
| 用户可改 | `TIME_CONFLICT` `START_IN_PAST` `NO_SCHEDULE_CHANGE` `ENTITLEMENT_NOT_FOUND` `INSUFFICIENT_BALANCE` `SESSION_TYPE_MISMATCH` | 说明受影响项，重新预览并重新取得该笔授权 |
| 需他人或配置修复 | `SESSION_BILLING_LOCKED` `MENTOR_PRICE_MISSING` `MENTOR_INACTIVE` `ENTITLEMENT_AMBIGUOUS` | 停止并交人工。已完成课的撤销涉及退款、导师应付或申诉，转专门的财务流程 |
| `SESSION_NOT_WRITABLE` | 原课已开始或状态不支持该动作 | 读回课次，报告当前状态和可用流程 |
| `OPERATION_IN_PROGRESS` | 建会或已有同步正在处理，或需要人工核对 | 有本身份和 origin 发起的原 `requestId` 时查原操作。`PLAN_BLOCKED` 上的 ID 属于失败预览，不能用来查原操作。缺原 ID 时报告当前状态并交人工核对 |

提交阶段还可能返回：

- `PLAN_EXPIRED`：计划超过有效期，重新预览并重新取得授权。
- `STALE_SESSION`：课次版本已变，先 `sa2 sessions get --id …` 读回最新值再重新计划。
- `IDEMPOTENCY_MISMATCH`：同一 requestId 的载荷已变。不要换键重发，先确认是否另起一笔。
- `PLAN_EFFECTS_CHANGED`：预览与提交之间业务影响已变，重新预览并重新取得授权。

提交阶段服务端只返回第一个阻断码，不返回完整清单；完整阻断项以预览阶段的 `error.blockers` 为准。

## 改期或取消

1. 改期适用于已排定、尚未开始且无财务锁的课次。针对原 sessionId，运行 `sa2 sessions plan reschedule --id … --start … --timezone … --duration … --out …`。展示旧→新起止（含时区）、时长、权益预占差额和会议同步影响。`plan reschedule` 会排除本课次旧时段，普通 `slots` 不会。仅改显示时区可能返回 `NO_SCHEDULE_CHANGE`。
2. 课前取消适用于未来的 `scheduled` 或 `pending_meeting`。建会中的取消可以被受理，须继续按取消操作的 `requestId` 核对。使用 `sa2 sessions plan cancel --id … --reason … --out …`，原因去掉首尾空白后须为 1–100 字符。展示目标、实际原因、预计释放预占、日历和通知影响。改期始终针对原课次，不能用先取消再创建代替。
3. 仅在当次对话中已明确授权摘要对应的具体操作时执行一次 `sa2 sessions reschedule --plan … --confirm` 或 `sa2 sessions cancel --plan … --confirm`。查询、模拟、预览均不授予写权限。原时间、目标、权益或影响发生变化时重新预览并确认。遵守工作区对共享库、测试及可恢复性的规定。
4. 分别核对 `businessStatus`、会议 `syncStatus` 和需要确认时的 `notificationStatus`。`wait` 只等待会议同步。改期后 `scheduled` 可能沿用自旧状态，应查本次 `requestId` 并回读原课次时间。会议同步 `needs_review` 时，本地新时段、日历和预占可能仍生效。等待超时或需人工时，报告 sessionId、requestId 和最后一次观测，避免重复变更。
5. 提交无响应时先用原 requestId 查结果。本次预览明确返回协议 v1、`durableReplay=true` 和 `operationLookup=true`，且原授权有效、原计划未过期、参数完全一致时，才允许原计划原键重放一次。否则只读跟踪。遇到 `STALE_SESSION` 先读取最新课次；不以新键重复执行，不删除课次，不更换归属字段，不代替用户选择其他时段。

## 隐含契约

- 计划文件有效期 **10 分钟**。超过后提交返回 `PLAN_EXPIRED`，须重新预览并重新取得授权。
- `sa2 sessions plan reschedule` 必须显式给 `--duration`。先用 `sa2 sessions get --id …` 读出当前的
  `durationMins` 再原样回填。旧课缺时长时停止并核实原约定，请用户明确确认时长后再计划，避免猜测并改变预占。
- 新协议的改期只能改开始时间、时长和时区，**改不了标题、备注、备注可见性和权益**。这些不属于
  改期范围，不要尝试用改期命令改变它们，也不要用先取消再创建代替改期。
- `sa2 sessions get` 不返回会议链接、备注和取消原因。需要时请用户在网页查看，不要编造。
- 业务 `FORBIDDEN` 是身份无权，不是登录过期；不要重新登录、换账号或换环境重试。服务端只允许
  Counselor、PartnerCounselor、Admin 写入，CLI 不在本地校验角色。
- `sa2 sessions operation` 返回的 `errorCode` 可能为 `MEETING_REJECTED`、`PROVIDER_OUTCOME_UNKNOWN`、
  `CREATE_OUTCOME_UNKNOWN` 或 `MISSING_SYNC_RESULT`，都要保留 `sessionId` 与 `requestId` 交人工。
