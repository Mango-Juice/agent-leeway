# Your AI coding agents, overall: Good

2026-01-30 – 2026-03-01

| | Rating (good · fair · poor) | Details |
|:--|:--|:--|
| Did what you asked | not checked yet | The in-depth check has not been run for 22 Codex sessions in this period. To run it: `agent-leeway drift --since 30d` (it shows what it would send and asks first) |
| Money wasted by habits | **Good** | $0.00 of $0.97 (0.0%) · Already Good |

_Rated on cost only: the in-depth check has not been run yet._

---

## What is going well

- **100%** of what you spent had no waste we could find

## Where it went off, and where the money went

### Where the agent went off

The in-depth check has not been run for 22 Codex sessions in this period. The in-depth check reads each session and finds where the agent did something other than what you asked, and why. To run it: `agent-leeway drift --since 30d` (it shows what it would send and asks first)


Not checked in depth yet: 22 Codex sessions in this period. To see what would be checked (nothing is sent): agent-leeway drift --dry-run --tool codex --since 30d

## Look things up

The rest of the report, in full.

### Other things worth fixing

<a id="f-setup-idle-codex"></a>
#### Turn off the skills you do not use

**3 of the 6 Codex skills in the list were not used in the last 29 days, 28% of the list.** Every session starts with each listed skill's name and description, used or not. [Codex docs](https://learn.chatgpt.com/docs/build-skills)

_How sure: Measured_

**How to fix it:** Turn off the skills you have not used, in Codex's config file. → `[[skills.config]]`, `enabled = false`

- atlas: none of this plugin's 2 skills was used in the last 29 days.
- Other skills not used: deploy.

Not counted: 1 session whose list was hidden or cut short.

### Your setup

What loads into every session, checked against each tool's own docs: per project, the latest session in this period; each row shows the project furthest over.

| | | |
|:--|:--|:--|
| Codex · Unused skills | 3 of 6 not used in the last 29 days | To review: 28% of the list; list seen in 21 sessions · [Codex docs](https://learn.chatgpt.com/docs/build-skills) |

**Codex · Unused skills**: Every session starts with each listed skill's name and description, used or not.

- atlas: none of this plugin's 2 skills was used in the last 29 days.
- Other skills not used: deploy.

Not counted: 1 session whose list was hidden or cut short.

**Do:** Turn off the skills you have not used, in Codex's config file. → `[[skills.config]]`, `enabled = false`

- Official docs: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “ChatGPT and Codex start with each skill's name and description, then load the full 'SKILL.md' instructions when they decide to use that skill.”
- Official docs: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “Use '[[skills.config]]' entries in '~/.codex/config.toml' to disable a skill without deleting it:”
- Official docs: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “Use exactly one selector per entry. An absolute 'path' matches one 'SKILL.md' file. A 'name' matches every skill with that name. When more than one entry matches a skill, the last matching entry wins.”
- Official docs: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “Restart Codex after changing the configuration.”

### How we checked

- **What we read**: Claude Code and Codex logs on this computer, 2026-01-30 to 2026-03-01. 22 sessions.
- **How sessions were checked**: Each tool's own model read each session against a fixed rubric (?). The two judges are calibrated differently, so their results are kept apart.
- **Where the dollars come from**: API list prices, checked 2026-11-22. A subscription is not billed this way.
- **Privacy**: This page was made on this computer. Nothing was sent anywhere to build it.
- Not checked in depth yet: 22 Codex sessions in this period. To see what would be checked (nothing is sent): agent-leeway drift --dry-run --tool codex --since 30d
- Unused skills: a tool's skills become a card in Other things worth fixing when half or more of the skills in its list went unused. This cut-off is our rule of thumb.
- A Codex skill counts as used when it was picked, or when a tool call names its folder.

[Full method and known limits](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)
