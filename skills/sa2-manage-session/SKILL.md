---
name: sa2-manage-session
description: 使用 SuperAcademy sa2 CLI 查询已存在的一对一课次、核对会议进度、为获授权课次改期或课前取消。适用于已有预约的跟进和变更。
---

# 跟进和变更已有课次

先用本机 `sa2 sessions --help` 核对版本能力；缺少所需命令时报告缺口。已知 requestId 而未知 sessionId 时，优先查询 `sa2 sessions operation --request-id …`。按用户授权的 origin 操作，认证沿用 `agent-cli-usage`。不要因查不到记录就换账号、换环境或重新约一节。

## 查课与区分来源

- 使用 `sa2 sessions list --student-id … --from … --to …` 缩小候选，再以 `sa2 sessions get --id …` 核对具体学生、导师、课型、时区、当前时间和状态。同日多节或时间含糊时请用户选定。属于 `class_session`、返回 `readOnly=true`、对象不可访问或已删除时，停止写入流程。只查状态无需准备变更。
- `pending_meeting` 表示已创建课次，`scheduled` 表示已排课。已完成、失败、取消各按实际状态报告。旧课可能缺少时区、课型、时长或会议链接；标记未知，必要时以明确标识的显示时区展示已知时间。

## 改期或取消

1. 首版改期适用于已排定、尚未开始且无财务锁的课次。建会或上次同步尚在进行时，先按原操作查询。针对原 sessionId，运行 `sa2 sessions plan reschedule --id … --start … --timezone … --duration … --out …`。若仅调整开始时间，沿用后端能核实的原时长。展示旧→新起止（含时区）、时长、权益预占差额和会议同步影响。双人冲突检查须排除本课次自身的旧时段。
2. 课前取消使用 `sa2 sessions plan cancel --id … --reason … --out …`，展示目标课次、具体原因、预计释放预占、日历和通知影响。已完成课的撤销涉及退款、导师应付或申诉，转专门的财务流程。确认已产生财务锁、版本已变或课程来源不符时，停止。改期始终针对原课次，不能用先取消再创建代替。
3. 仅在当次对话中已明确授权摘要对应的具体操作时执行一次 `sa2 sessions reschedule --plan … --confirm` 或 `sa2 sessions cancel --plan … --confirm`。查询、模拟、预览均不授予写权限。原时间、目标、权益或影响发生变化时重新预览并确认。遵守工作区对共享库、测试及可恢复性的规定。
4. 分别核对本地业务状态和外部会议同步。改期后 `scheduled` 可能沿用自旧状态；用本次 requestId 查同步结果，并回读原 sessionId 的最终时间。课次已取消而飞书撤会仍 pending 时，如实报告两者。等待超时或补偿失败时报告 sessionId、requestId 和最后一次观测，交人工处理。
5. 提交无响应时先用原 requestId 查结果。本次预览明确返回协议 v1、`durableReplay=true` 和 `operationLookup=true`，且原授权有效、原计划未过期、参数完全一致时，才允许原计划原键重放一次。否则只读跟踪。遇到 `STALE_SESSION` 先读取最新课次；不以新键重复执行，不删除课次，不更换归属字段，不代替用户选择其他时段。
