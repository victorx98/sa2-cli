---
name: sa2-monthly-milestones
description: Update SA2 student monthly report milestones from a counselor spreadsheet using an authenticated SA2 API session. Use only when the user asks to apply milestone rows to monthly reports; a spreadsheet attachment alone does not authorize updates.
---

# SA2 monthly report milestones

Use the authenticated SA2 API base URL and login state supplied by the user. Never paste tokens into files or output. A write requires the user's explicit request to update reports in this conversation. Follow the repository's database write rules, including a before snapshot, field-limited update, and after verification. Do not write to the database directly.

## Prepare the workbook

Use the complete milestone worksheet as the data sheet. If the workbook follows the earlier draft structure, **完整草稿v6** is the data sheet; **本次三项修订** and **核查说明** explain edits and must not be applied as extra rows.

Column **A must contain the student's UUID** and column **B must contain the report period in `YYYY-MM` form** on every data row. Read the milestone title, English status, notes, row type and evidence by their header names rather than fixed column letters; adding A and B may shift the other columns. The milestone data comes from the title, status and notes columns. Evidence is for review, not report text. Do not turn a check date into an action in the report month.

Skip rows with an empty A cell. Validate every nonempty A value as a UUID and its B value as a real calendar month. Group rows by the exact `(studentId, year-month)` pair; never fill down an omitted ID or period from the preceding row. If a row with an ID has an invalid ID, period, milestone title or required header, report it and leave that row unchanged. Confirm the requested students and periods against the workbook before writing. The earlier sample workbook used student names in A and row numbers in B; it must be revised to this new layout before use.

Map these spreadsheet titles to built-in keys:

| Title                            | Key                                |
| -------------------------------- | ---------------------------------- |
| 首次顾问战略沟通会               | `first_counselor_strategy_meeting` |
| 岗位差距分析会议                 | `first_mentor_session`             |
| 家长沟通会                       | `parent_meeting`                   |
| 求职阶段部署会议                 | `job_search_prep_meeting`          |
| 简历修改与定稿                   | `resume_revision_finalization`     |
| 求职信与第三人称介绍撰写与定稿   | `cover_letter_finalization`        |
| 自我介绍视频录制                 | `self_intro_video_recording`       |
| LinkedIn页面搭建与Networking学习 | `linkedin_profile_setup`           |
| BQ 逐字稿撰写                    | `bq_script_review`                 |
| 专业技能提升                     | `professional_skills_enhancement`  |
| 面试技能提升                     | `interview_skills_enhancement`     |
| 人脉拓展                         | `networking`                       |
| 岗位投递                         | `job_application`                  |

Other titles, including 面试进展 and class or internship rows, are custom milestones. Reuse an existing custom key when its label exactly matches the title; otherwise create a `custom_` plus UUID key and set `label` to the title. The workbook merges simulated interviews into 面试技能提升. Omit an untouched default `mock_interview` row; if that row contains counselor edits, preserve its content and flag the merge for review rather than losing it.

Map `On Track` → `on_track`, `Needs Attention` → `needs_attention`, `Not Started` → `not_started`, `At Risk` → `at_risk`, `Done` → `done`, and `Critical` → `critical`. `—`, blank, and `待确认` are not confirmed statuses: preserve the existing row rather than asserting a new status. For rows marked 本月无内容, preserve the existing row unless an explicit status and substantive note clearly call for an update. Keep the notes column's factual content and dates; escape HTML and use paragraphs or line breaks because the report notes field is rich text. Do not paste evidence links into notes unless the user asks.

## API workflow

For each validated `(studentId, year-month)` group, GET `/api/reports/students/{studentId}/monthly-service-reports?year={year}&month={month}` with the supplied Bearer login state. Require a non-null report `id`; the update endpoint does not create reports. Save the returned `monthlyMilestonesSnapshot` and other stored fields as the before snapshot. Work from this snapshot: update only confirmed workbook rows, keep other keys and their order, and append new custom rows in workbook order. Do not send duplicate keys or unknown status values.

PUT `/api/reports/monthly-service-reports/milestones/batch` once with JSON `{ "reports": [{ "studentId": "uuid", "period": "YYYY-MM", "monthlyMilestonesSnapshot": [...] }, ...] }`. Include every validated group exactly once. The endpoint checks that all target reports exist before updating, then updates each group's milestone field in request order and returns an array of updated reports in that order. A write error after updates start can leave an earlier group changed; do not retry the whole batch blindly. On 400/403/404 or any unexpected error, read each target report and compare with its before snapshot to identify what changed. Do not generate missing reports or retry with a different identity. After success, GET each report and compare the full milestone list to the intended list and all other stored fields to the before snapshot. Stop on any unexpected difference.

Summarize each student ID and report period, changed milestone titles, skipped uncertain rows, and verification result without exposing the login token or unrelated personal data. If the batch fails, report which groups changed and which remain.
