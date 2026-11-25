# Your AI coding agents, overall: Good

2026-01-30 – 2026-03-01

| | Rating (good · fair · poor) | Details |
|:--|:--|:--|
| Did what you asked | not checked yet | The in-depth check has not been run for 22 Claude Code sessions in this period. To run it: `agent-leeway drift --since 30d` (it shows what it would send and asks first) |
| Money wasted by habits | **Good** | $0.00 of $0.65 (0.0%) · Already Good |

_Rated on cost only: the in-depth check has not been run yet._

---

## What is going well

- **100%** of what you spent had no waste we could find

## Where it went off, and where the money went

### Where the agent went off

The in-depth check has not been run for 22 Claude Code sessions in this period. The in-depth check reads each session and finds where the agent did something other than what you asked, and why. To run it: `agent-leeway drift --since 30d` (it shows what it would send and asks first)


Not checked in depth yet: 22 Claude Code sessions in this period. To see what would be checked (nothing is sent): agent-leeway drift --dry-run --tool claude --since 30d

## Look things up

The rest of the report, in full.

### Other things worth fixing

<a id="f-setup-idle-claude"></a>
#### Turn off the skills you do not use

**5 of the 10 Claude Code skills in the list were not used in the last 29 days, 50% of the list.** Every skill in the list adds to the context on every turn, used or not. [Claude Code docs](https://code.claude.com/docs/en/skills.md)

_How sure: Measured_

**How to fix it:** Turn off the skills you have not used, and turn off a plugin whose skills you never use. → `/skills`, `/plugin disable`

- atlas: none of this plugin's 3 skills was used in the last 29 days.
- ledger: 1 of 2 skills not used: ledger:audit.
- Other skills not used: gif-maker.

Not counted: 1 skill listed in fewer than 20 sessions.

### Your setup

What loads into every session, checked against each tool's own docs: per project, the latest session in this period; each row shows the project furthest over.

| | | |
|:--|:--|:--|
| Claude Code · Skill list | 10 skills, every description kept | Within: budget about 1% of the context window · [Claude Code docs](https://code.claude.com/docs/en/skills.md) |
| Claude Code · Unused skills | 5 of 10 not used in the last 29 days | To review: 50% of the list; list seen in 22 sessions · [Claude Code docs](https://code.claude.com/docs/en/skills.md) |

**Claude Code · Unused skills**: Every skill in the list adds to the context on every turn, used or not.

- atlas: none of this plugin's 3 skills was used in the last 29 days.
- ledger: 1 of 2 skills not used: ledger:audit.
- Other skills not used: gif-maker.

Not counted: 1 skill listed in fewer than 20 sessions.

**Do:** Turn off the skills you have not used, and turn off a plugin whose skills you never use. → `/skills`, `/plugin disable`

- Official docs: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “Every skill in the skill listing adds to your context on every turn, whether or not Claude ever uses it.”
- Official docs: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “The '/skills' menu writes it for you: highlight a skill and press 'Space' to cycle states, then 'Esc' to save to '.claude/settings.local.json'.”
- Official docs: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “Plugin skills are not affected by 'skillOverrides'. Manage those through '/plugin' instead.”
- Official docs: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “Bundled skill: set 'disableBundledSkills' to 'true' to turn off bundled skills, or set one skill to '"off"' in 'skillOverrides' to hide it.”
- Official docs: [Claude Code docs: Install and manage plugins](https://code.claude.com/docs/en/plugins/install.md) · 2026-11-25 · “In a Claude Code session, run '/plugin' and press Tab to reach it, or run '/plugin enable', '/plugin disable', or '/plugin uninstall' to open the panel and make that change there.”

### How we checked

- **What we read**: Claude Code and Codex logs on this computer, 2026-01-30 to 2026-03-01. 23 sessions.
- **How sessions were checked**: Each tool's own model read each session against a fixed rubric (?). The two judges are calibrated differently, so their results are kept apart.
- **Where the dollars come from**: API list prices, checked 2026-11-22. A subscription is not billed this way.
- **Privacy**: This page was made on this computer. Nothing was sent anywhere to build it.
- Not checked in depth yet: 22 Claude Code sessions in this period. To see what would be checked (nothing is sent): agent-leeway drift --dry-run --tool claude --since 30d
- Unused skills: a tool's skills become a card in Other things worth fixing when half or more of the skills in its list went unused. This cut-off is our rule of thumb.

[Full method and known limits](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)
