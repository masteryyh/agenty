# HANDOFF.md Maintenance

`HANDOFF.md` is the short-lived handoff bridge for future agents working in this repository. It is the only routine exception to the project rule that forbids summary documents or planning documents.

## Update Trigger

After every completed conversation turn, if any project file was created, modified, deleted, renamed, generated, or materially reformatted, update `HANDOFF.md` in the same turn. This applies to source code, configuration, workflow files, design documents, user-facing usage documents, project skills, reference documents, and other repository artifacts.

If `HANDOFF.md` does not exist, create it. Do not restore old incorrect content or stale historical records.

## Language and Structure

The project skill and its reference documents should be written in English. The content of `HANDOFF.md` itself must be written in Simplified Chinese.

Keep the heading hierarchy:

```md
# 交接记录

## 2026-05-17 - 任务标题

### 任务记录
```

Every completed task must add a separate second-level heading. The heading should include the date and task name so future agents can distinguish separate work. Use a third-level heading to contain the task table; the default heading is `### 任务记录`.

## Required Table

Every task block must use a table to record the task goal, completion approach, and modified files. The table columns are fixed:

```md
| 任务目标 | 完成手段 | 修改文件 |
|---|---|---|
```

If a conversation turn contains multiple goals, split them into multiple rows by goal. Do not squeeze multiple independent goals into one row.

`任务目标` should describe the user's actual requirement and the real scope of the turn. `完成手段` should describe the concrete implementation, documentation strategy, important constraints, tradeoffs, and validation. `修改文件` should list every path touched in the turn and why it changed; deleted, renamed, and generated files must also be called out.

## Content Boundaries

Keep the content factual and operational. Do not copy chat transcripts, do not write a chronological log, and do not preserve stale historical notes. Keep only the current conclusions, constraints, and file changes that a future agent needs to continue safely.
