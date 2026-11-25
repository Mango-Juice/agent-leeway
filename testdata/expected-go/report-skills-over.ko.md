# AI 코딩 에이전트 활용 종합: 좋음

2026-01-30 – 2026-03-01

| | 등급 (좋음 · 보통 · 나쁨) | 내용 |
|:--|:--|:--|
| 요청대로 했는지 | 점검 전 | 이 기간의 Claude Code 세션 22개는 아직 정밀 점검하지 않았습니다. 실행하려면: `agent-leeway drift --since 30d` (보낼 내용을 먼저 보여 주고 묻습니다) |
| 습관으로 샌 비용 | **좋음** | $0.65 중 $0.00 (0.0%) · 이미 좋음 |

_정밀 점검 전이라 비용만으로 매긴 등급입니다._

### 먼저 고칠 것: 직접 부르는 스킬은 목록에서 빼세요

목록이 꽉 차서 Claude Code가 스킬 10개 중 3개의 설명을 뺐습니다. 그 스킬은 Claude가 알아서 고르지 못할 수 있습니다.

[출처](https://code.claude.com/docs/en/skills.md) · **→ [사례 보기](#f-setup-skills-claude)**

---

<a id="f-setup-skills-claude"></a>
## 고칠 것 1 / 1: 직접 부르는 스킬은 목록에서 빼세요

**목록이 꽉 차서 Claude Code가 스킬 10개 중 3개의 설명을 뺐습니다. 그 스킬은 Claude가 알아서 고르지 못할 수 있습니다.**

빼도 /이름으로 직접 부르면 계속 쓸 수 있습니다. 이 목록에는 컨텍스트 창의 1% 정도만 쓸 수 있고, 덜 쓰는 스킬부터 설명이 빠집니다. [출처](https://code.claude.com/docs/en/skills.md)

_확실성: 측정값_

**고치는 방법:** 직접 부를 때만 쓰는 스킬은 SKILL.md에 `disable-model-invocation: true`를 넣으세요. 그 스킬의 설명이 목록에서 빠집니다. 더 안 쓰는 스킬은 끄세요. `/skill-doctor`가 한 번도 안 쓴 스킬을 보여 줍니다.

설명이 빠진 스킬, 덜 쓴 순서: gif-maker, atlas:tiles, release-notes.

최근 29일 동안 쓰지 않은 스킬: 10개 중 5개, 목록 글자 수의 45%. atlas: 최근 29일 동안 이 플러그인의 스킬 3개를 하나도 쓰지 않았습니다.

찾은 것: 스킬 10개 중 3개, 2026-02-27에 보낸 목록 (trail-planner)

_다른 방법: `skillListingBudgetFraction`로 예산을 늘려도 설명이 돌아오지만, 그만큼 모든 요청에 실리는 목록이 커집니다. [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md)_

## 잘 되고 있는 것

- **100%** 사용액에서는 낭비를 찾지 못했습니다

## 어디서 엇나갔고, 돈은 어디로 샜나

### 에이전트가 엇나간 곳

이 기간의 Claude Code 세션 22개는 아직 정밀 점검하지 않았습니다. 정밀 점검은 세션마다 대화를 읽고, 에이전트가 요청과 다르게 한 곳과 그 이유를 찾습니다. 실행하려면: `agent-leeway drift --since 30d` (보낼 내용을 먼저 보여 주고 묻습니다)


아직 정밀 점검하지 않음: 이 기간의 Claude Code 세션 22개. 무엇을 점검할지 보려면(아무것도 보내지 않음): agent-leeway drift --dry-run --tool claude --since 30d

## 찾아보기

리포트의 나머지 내용을 모두 담았습니다.

### 내 설정

모든 세션에 먼저 실리는 설정을 각 도구 문서의 기준과 비교했습니다. 프로젝트마다 이 기간의 마지막 세션을 보고, 각 줄에는 기준을 가장 많이 넘은 프로젝트를 보여 줍니다.

| | | |
|:--|:--|:--|
| Claude Code · 스킬 목록 | 스킬 10개, 목록이 넘쳐 3개의 설명이 빠짐 | **기준 넘음**: 예산은 컨텍스트 창의 1% 정도 · [Claude Code 문서](https://code.claude.com/docs/en/skills.md) |
| Claude Code · 안 쓴 스킬 | 최근 29일 동안 10개 중 5개를 쓰지 않음 | 살펴볼 것: 목록 글자 수의 45%, 목록이 기록된 세션 22개 · [Claude Code 문서](https://code.claude.com/docs/en/skills.md) |

**Claude Code · 안 쓴 스킬**: 목록에 있는 스킬은 쓰든 안 쓰든 매 턴 컨텍스트에 실립니다.

- atlas: 최근 29일 동안 이 플러그인의 스킬 3개를 하나도 쓰지 않았습니다.
- ledger: 스킬 2개 중 1개를 쓰지 않음: ledger:audit.
- 그 밖에 쓰지 않은 스킬: gif-maker.

목록에 오른 세션이 20개 미만인 스킬 1개는 세지 않았습니다.

**할 일:** 쓰지 않은 스킬은 끄고, 스킬을 하나도 쓰지 않는 플러그인은 꺼 두세요. → `/skills`, `/plugin disable`

- 공식 문서: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “Every skill in the skill listing adds to your context on every turn, whether or not Claude ever uses it.”
- 공식 문서: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “The '/skills' menu writes it for you: highlight a skill and press 'Space' to cycle states, then 'Esc' to save to '.claude/settings.local.json'.”
- 공식 문서: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “Plugin skills are not affected by 'skillOverrides'. Manage those through '/plugin' instead.”
- 공식 문서: [Claude Code docs: Extend Claude with skills](https://code.claude.com/docs/en/skills.md) · 2026-11-25 · “Bundled skill: set 'disableBundledSkills' to 'true' to turn off bundled skills, or set one skill to '"off"' in 'skillOverrides' to hide it.”
- 공식 문서: [Claude Code docs: Install and manage plugins](https://code.claude.com/docs/en/plugins/install.md) · 2026-11-25 · “In a Claude Code session, run '/plugin' and press Tab to reach it, or run '/plugin enable', '/plugin disable', or '/plugin uninstall' to open the panel and make that change there.”

### 점검 방법

- **무엇을 읽었나**: 이 컴퓨터에 있는 Claude Code·Codex 로그, 2026-01-30부터 2026-03-01까지. 세션 23개.
- **어떻게 점검했나**: 각 도구의 모델이 정해진 기준(?)에 따라 세션을 하나씩 읽었습니다. 두 판정 모델은 기준이 조금 달라서 결과를 섞지 않았습니다.
- **금액은 어디서**: API 정가 기준이고, 가격표는 2026-11-22에 확인했습니다. 구독 요금은 이렇게 청구되지 않습니다.
- **개인정보**: 이 페이지는 이 컴퓨터에서 만들었고, 만드는 동안 아무것도 밖으로 보내지 않았습니다.
- 아직 정밀 점검하지 않음: 이 기간의 Claude Code 세션 22개. 무엇을 점검할지 보려면(아무것도 보내지 않음): agent-leeway drift --dry-run --tool claude --since 30d
- 안 쓴 스킬: 목록에 있는 스킬의 절반 이상을 쓰지 않은 도구는 '그 밖에 고칠 것'에 카드로 올립니다. 이 기준값은 우리가 정한 경험칙입니다.

[방법과 알려진 한계 전체 보기](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)
