---
name: agenty
description: Use when operating agenty itself, including sessions, tools, skills, project context, and local or daemon mode workflows.
license: Apache-2.0
metadata:
  id: 019decb1-850b-7efb-8261-966c19224492
  builtin: "true"
  version: "1.0.0"
---

# agenty

Use this skill when the user asks about agenty behavior, project context, available skills, tool usage, or differences between local mode and daemon mode.

## Operating Principles

- Prefer existing agenty tools and services before introducing external workflow assumptions.
- Treat session cwd as the boundary for project-local context.
- Load project skills from `<cwd>/.agents/skills` and `<cwd>/.claude/skills` when they are relevant.
- Use `find_skill` to discover additional skills when the available skill list is incomplete or too large.
- Use `read_file` to load a selected `SKILL.md` before applying that skill.

## Skill Selection

When multiple skills may apply, choose the most specific skill first. Project skills override global and built-in guidance when they directly match the user's repository or task.
