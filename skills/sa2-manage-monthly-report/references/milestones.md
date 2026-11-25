# 里程碑名称与更新语义

内置 key 来自 sa2-api 的 monthly-service-report.types.ts；中文名称与 sa2-ui 的月报翻译保持一致。报告中有自定义 label 时以实际 label 为准，但写入仍使用原 key。

| 内置中文名称 | key |
| --- | --- |
| 首次顾问战略沟通会 | first_counselor_strategy_meeting |
| 岗位差距分析会议 | first_mentor_session |
| 家长沟通会 | parent_meeting |
| 求职阶段部署会议 | job_search_prep_meeting |
| 简历修改与定稿 | resume_revision_finalization |
| 求职信与第三人称介绍撰写与定稿 | cover_letter_finalization |
| 自我介绍视频录制 | self_intro_video_recording |
| LinkedIn页面搭建与Networking学习 | linkedin_profile_setup |
| BQ 逐字稿撰写 | bq_script_review |
| 专业技能提升 | professional_skills_enhancement |
| 面试技能提升 | interview_skills_enhancement |
| 人脉拓展 | networking |
| 岗位投递 | job_application |
| 模拟面试 | mock_interview |

状态为 `not_started` 未开始、`done` 已完成、`on_track` 正常推进、`needs_attention` 需要关注、`at_risk` 存在风险、`critical` 严重风险。以当前 CLI schema 的枚举为准。

“首次顾问战略沟通会，状态 —，备注：本月无相关内容，可保持系统默认”有歧义：应先问状态是否保留，以及最后一句是否为要保存的备注原文；未得到明确答复前只读。若用户明确说“保留状态，把备注原样设为这句话”，只传 notes，不传 status。

PATCH 请求必须包含 `reportPeriod` 和 `expectedUpdatedAt`，至少提供 status 或 notes；null、未知字段、未知状态不被接受。省略字段保留原值；notes 为空字符串明确清空。此接口不修改名称、排序或其他行，不支持创建和删除。

新接口使用条件更新保护本次提交。现有网页整段 PUT 仍沿用原行为；不能声称所有旧入口均已具备并发保护。提交结果不确定时先读取当前内容，不自行重放。
