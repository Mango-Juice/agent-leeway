# AI 코딩 에이전트 활용 종합: 좋음

2026-01-30 – 2026-03-01

| | 등급 (좋음 · 보통 · 나쁨) | 내용 |
|:--|:--|:--|
| 요청대로 했는지 | 점검 전 | 이 기간의 Codex 세션 22개는 아직 정밀 점검하지 않았습니다. 실행하려면: `agent-leeway drift --since 30d` (보낼 내용을 먼저 보여 주고 묻습니다) |
| 습관으로 샌 비용 | **좋음** | $0.97 중 $0.00 (0.0%) · 이미 좋음 |

_정밀 점검 전이라 비용만으로 매긴 등급입니다._

---

## 잘 되고 있는 것

- **100%** 사용액에서는 낭비를 찾지 못했습니다

## 어디서 엇나갔고, 돈은 어디로 샜나

### 에이전트가 엇나간 곳

이 기간의 Codex 세션 22개는 아직 정밀 점검하지 않았습니다. 정밀 점검은 세션마다 대화를 읽고, 에이전트가 요청과 다르게 한 곳과 그 이유를 찾습니다. 실행하려면: `agent-leeway drift --since 30d` (보낼 내용을 먼저 보여 주고 묻습니다)


아직 정밀 점검하지 않음: 이 기간의 Codex 세션 22개. 무엇을 점검할지 보려면(아무것도 보내지 않음): agent-leeway drift --dry-run --tool codex --since 30d

## 찾아보기

리포트의 나머지 내용을 모두 담았습니다.

### 그 밖에 고칠 것

<a id="f-setup-idle-codex"></a>
#### 안 쓰는 스킬은 끄세요

**최근 29일 동안 Codex 목록의 스킬 6개 중 3개를 쓰지 않았습니다. 목록 글자 수의 28%입니다.** 세션마다 목록에 있는 스킬의 이름과 설명이 먼저 실립니다. 쓰든 안 쓰든 마찬가지입니다. [Codex docs](https://learn.chatgpt.com/docs/build-skills)

_확실성: 측정값_

**고치는 방법:** 쓰지 않은 스킬은 Codex 설정 파일에서 끄세요. → `[[skills.config]]`, `enabled = false`

- atlas: 최근 29일 동안 이 플러그인의 스킬 2개를 하나도 쓰지 않았습니다.
- 그 밖에 쓰지 않은 스킬: deploy.

목록이 숨겨졌거나 잘린 세션 1개는 세지 않았습니다.

### 내 설정

모든 세션에 먼저 실리는 설정을 각 도구 문서의 기준과 비교했습니다. 프로젝트마다 이 기간의 마지막 세션을 보고, 각 줄에는 기준을 가장 많이 넘은 프로젝트를 보여 줍니다.

| | | |
|:--|:--|:--|
| Codex · 안 쓴 스킬 | 최근 29일 동안 6개 중 3개를 쓰지 않음 | 살펴볼 것: 목록 글자 수의 28%, 목록이 기록된 세션 21개 · [Codex 문서](https://learn.chatgpt.com/docs/build-skills) |

**Codex · 안 쓴 스킬**: 세션마다 목록에 있는 스킬의 이름과 설명이 먼저 실립니다. 쓰든 안 쓰든 마찬가지입니다.

- atlas: 최근 29일 동안 이 플러그인의 스킬 2개를 하나도 쓰지 않았습니다.
- 그 밖에 쓰지 않은 스킬: deploy.

목록이 숨겨졌거나 잘린 세션 1개는 세지 않았습니다.

**할 일:** 쓰지 않은 스킬은 Codex 설정 파일에서 끄세요. → `[[skills.config]]`, `enabled = false`

- 공식 문서: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “ChatGPT and Codex start with each skill's name and description, then load the full 'SKILL.md' instructions when they decide to use that skill.”
- 공식 문서: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “Use '[[skills.config]]' entries in '~/.codex/config.toml' to disable a skill without deleting it:”
- 공식 문서: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “Use exactly one selector per entry. An absolute 'path' matches one 'SKILL.md' file. A 'name' matches every skill with that name. When more than one entry matches a skill, the last matching entry wins.”
- 공식 문서: [Codex docs: Build skills](https://learn.chatgpt.com/docs/build-skills) · 2026-11-25 · “Restart Codex after changing the configuration.”

### 점검 방법

- **무엇을 읽었나**: 이 컴퓨터에 있는 Claude Code·Codex 로그, 2026-01-30부터 2026-03-01까지. 세션 22개.
- **어떻게 점검했나**: 각 도구의 모델이 정해진 기준(?)에 따라 세션을 하나씩 읽었습니다. 두 판정 모델은 기준이 조금 달라서 결과를 섞지 않았습니다.
- **금액은 어디서**: API 정가 기준이고, 가격표는 2026-11-22에 확인했습니다. 구독 요금은 이렇게 청구되지 않습니다.
- **개인정보**: 이 페이지는 이 컴퓨터에서 만들었고, 만드는 동안 아무것도 밖으로 보내지 않았습니다.
- 아직 정밀 점검하지 않음: 이 기간의 Codex 세션 22개. 무엇을 점검할지 보려면(아무것도 보내지 않음): agent-leeway drift --dry-run --tool codex --since 30d
- 안 쓴 스킬: 목록에 있는 스킬의 절반 이상을 쓰지 않은 도구는 '그 밖에 고칠 것'에 카드로 올립니다. 이 기준값은 우리가 정한 경험칙입니다.
- Codex 스킬은 직접 골랐거나, 도구 호출에 그 스킬 폴더가 나오면 쓴 것으로 봅니다.

[방법과 알려진 한계 전체 보기](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)
