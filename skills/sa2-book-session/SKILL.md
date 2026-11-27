---
name: sa2-book-session
description: 使用 SuperAcademy sa2 CLI 为学生准备或创建一对一导师课次，核对合同权益、双方时段和异步会议结果。适用于常规辅导、GAP 分析和 AI 职业辅导的新预约。
---

# 新预约一对一课次

先看本机 `sa2 sessions --help` 及所需子命令的帮助。若版本只有 login、whoami、logout，交付已知的预约摘要并列出缺项。业务操作仅走宿主提供的对应命令。登录沿用同一 origin 的 `agent-cli-usage` 流程；`whoami` 会续期，不作为无副作用探针。

新版网页和 CLI 的一对一新约、改期和课前取消共用预约 API。CLI 仍须先生成计划、取得明确授权，再按原计划提交和查回执。遇到阻断时按预约 API 的结果处置，网页上可见的按钮或候选项不代表一定可提交。

## 准备

1. 分清“查能否约”“帮我准备”和“替我提交”。班课、沟通记录、模拟面试及已完成课补录各有独立流程。已有一对一课次的调整使用 `sa2-manage-session`。
2. 用 `sa2 sessions find --kind student|mentor --text …` 查可访问的候选（kind 选其一）。名称冲突时给出最少可识别信息，让用户选定。准备具体预约时，用 `sa2 sessions options --student-id … --mentor-id …` 获取合同，再加 `--contract-id …` 查看合同可用权益和导师适用的课型。保留返回的课型 UUID；同一课型代码可出现在多个服务类别。受限或查无结果时停止，不自行扩大查询身份。
3. 只问“下周哪些时间能约”时，确认双方身份、所指日期范围、时区及所需时长即可用 `sa2 sessions slots --student-id … --mentor-id … --from … --to … --timezone … --duration …`。只返回候选时段，无需合同或标题，不建立预约。一个空闲时段只代表平台当时未查到冲突。
4. 确定预约日期、开始时间、IANA 时区、时长、标题、双方、业务合同、实际预占的 Internal/External 权益和备注可见性。遇到夏令时重复小时要求明确偏移，不存在的本地时间请用户重选。多合同、同名导师和跨权益扣课都不能靠猜测。
5. 使用 `sa2 sessions plan create --student-id … --mentor-id … --contract-id … --type-id … --entitlement … --start … --timezone … --duration … --title … --notes-visibility shared|mentor-only --out …`（两种可见性选其一）。生成的本机私有计划文件保留 requestId。展示预览中的学生、导师、课型、合同、预占数量、权益实际来源、备注可见性、带偏移的起止及北京时间；如会建飞书会议、触发通知或初始化 GAP 记录，也一并说明。仅导师可见的备注与会议通知的收件人是两个设置。只要求预览就停在这一步。

## 预览阻断的处置

`sa2 sessions plan` 被阻断时，stdout 的 `error.code` 是 `PLAN_BLOCKED`，`error.blockers` 是阻断码数组。
按下表分类处置，**不要一律重刷预览**：

| 类别 | 阻断码 | 处置 |
| --- | --- | --- |
| 用户可改 | `TIME_CONFLICT` `INSUFFICIENT_BALANCE` `START_IN_PAST` `ENTITLEMENT_NOT_FOUND` `NO_SCHEDULE_CHANGE` `SESSION_TYPE_MISMATCH` | 说明受影响项，提出候选时段、合同、权益或课型，重新预览并**重新取得该笔授权** |
| 需他人或配置修复 | `MENTOR_PRICE_MISSING` `MENTOR_INACTIVE` `ENTITLEMENT_AMBIGUOUS` | 停止并报告可核对的信息，交人工处理 |

`ORIGIN_MISMATCH`、`BOOKING_NOT_CONFIGURED` 和部分权益异常会直接返回 `error.code`。先读该字段，再按业务码决定下一步。配置问题交环境维护者处理，沿用获授权的 origin。

预览结果里的 `warnings` 不是错误，但必须向用户明示并取得确认，不得静默通过：
`ENTITLEMENT_OVERRIDE`（预占权益与导师类型不一致）、`LEGACY_ENTITLEMENT_SOURCE`（回退到历史权益来源）。

`sa2 sessions options` 另有三个 CLI 侧错误码：`MENTOR_NOT_FOUND`（该 ID 没有在职导师）、
`MENTOR_TYPE_UNKNOWN`（导师类型不属于内外部组，无法给出可约课型）、`CONTRACT_NOT_FOUND`
（该合同不在此学生名下）。三者都要停下换对象，不要换身份重试。

`sa2 sessions slots` 返回的 `intervals` 是“无 booked 冲突的空窗”，不是可约时段：可能跨午夜，
也可能整段落在导师工作时间之外。须在区间内自行选取子窗口并向用户确认。
如果 `truncated=true`，缩小查询窗口再查，避免把截断的数据当成完整结果。

## 提交与核对

- 当前对话已明确授权该笔具体预约且摘要完全一致时，执行一次 `sa2 sessions create --plan … --confirm`。授权只覆盖该 origin、身份和计划内容。信息不全、歧义或关键项变化时先给出新摘要，请用户确认差异。测试或笼统的“验证”不构成真实写入授权。
- 返回 `sessionId` 和 `pending_meeting` 时，告知“预约已受理，会议创建中”。用 `sa2 sessions wait --request-id … --timeout 60s` 只读等待。确认业务为 `scheduled` 且会议 `syncStatus=succeeded` 后再报告已排课。通知状态另读 `operation.notificationStatus`。CLI 不返回会议链接，需要时请用户在网页查看。
- 网络超时或 5xx 使提交结果不明时，用原计划的 `sa2 sessions operation --request-id …` 核对。查到已提交就跟踪原课次。暂时查不到也可能仍在提交中。仅限本次预览明确返回协议 v1、`durableReplay=true` 和 `operationLookup=true`，且原授权有效、原计划仍在有效期内时，允许原文件原键重放一次。其他情况只读查询；持续不明则报告 requestId 交人工排查。会议失败或长期 pending 时保留已知 sessionId，不另建课或自行取消。
- 冲突、余额不足、候选变更或计划失效时，按「预览阻断的处置」表分类处置，不要一律重刷预览。换时段、合同、权益或导师都要取得该笔变更后的明确授权。禁止绕过财务锁或修改记录归属。遵守所在工作区对共享库、测试账号及可还原性的规定。

## 隐含契约

- 计划文件有效期 **10 分钟**。预览后若要等待用户确认，超过 10 分钟须重新 `sa2 sessions plan`，
  重新预览并重新取得授权；提交返回 `PLAN_EXPIRED` 时同理。
- 当输入代表当地钟表时间时，使用带明确偏移的 RFC3339，并核对预览的 `localStart`。重复小时须选定偏移，不存在的时间请用户重选。CLI 会校验显式偏移。以 `Z` 结尾的输入直接表示 UTC 时间点，适用于用户指定的 UTC 时间。
- 服务端只允许 Counselor、PartnerCounselor、Admin 写入；CLI 不在本地校验角色，越权由服务端以业务
  `FORBIDDEN` 拒绝。该码表示身份无权，**不是登录过期**：不要重新登录、换账号或换环境重试。
- 提交阶段服务端只返回**第一个**阻断码，不返回完整清单。需要完整阻断项时以预览阶段的
  `error.blockers` 为准。
- `sa2 sessions get` 不返回会议链接、备注和取消原因，CLI 输出里没有这些字段。需要时请用户在
  网页查看，不要编造。
- 标题去掉首尾空白后须为 1–255 字符。服务端用 `INVALID_BOOKING_INPUT` 拒绝超限输入，核对后重新预览。
