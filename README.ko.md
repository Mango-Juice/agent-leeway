# Leeway

[English](README.md)

**AI 코딩 에이전트를 잘 쓰고 있는지 점검해 주는 도구입니다.**

Leeway(`agent-leeway`)는 내 컴퓨터에 쌓인 Claude Code와 Codex 세션 로그를 읽고 두 가지를 알려 줍니다.

- **돈이 샌 곳**: 쉬는 동안 캐시가 만료됐거나 같은 명령이 계속 실패한 경우처럼, 습관 때문에 낭비된 비용
- **에이전트가 엇나간 곳**: 내 요청과 다르게 움직여서 바로잡아야 했던 순간과 그 원인

그리고 무엇부터 고치면 되는지 리포트 한 장으로 보여 줍니다. macOS, Linux, Windows에서 실행 파일 하나로 동작합니다.

![testdata/의 합성 테스트 로그로 만든 리포트 첫 화면](docs/img/report-ko.png)

리포트 전체 예시: [예시 리포트](https://mango-juice.github.io/agent-leeway/ko/). 지어낸 세션으로 만들었습니다.

## 설치

**Claude Code나 Codex에 이렇게 말하면 됩니다.**

```text
Mango-Juice/agent-leeway 설치해줘
```

직접 설치하려면 시스템에 맞는 명령을 실행하세요. 프로그램을 설치하고, 에이전트에게 말로 리포트를 부탁할 수 있도록 Claude Code와 Codex에 스킬까지 추가합니다.

macOS, Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex
```

이제 에이전트에게 리포트를 부탁하면 됩니다.

Homebrew, Scoop, `npx skills`, 직접 내려받기, 체크섬 확인, Windows 참고 사항, 삭제 방법은 [docs/install.md](docs/install.md)에 있습니다(영어).

<details><summary>설치를 맡은 에이전트를 위한 안내</summary>

사용자의 시스템에 맞는 명령을 보여 주고, 실행해도 되는지 먼저 물어보세요. leeway라는 이름의 다른 프로젝트도 있으니 Mango-Juice/agent-leeway만 설치하세요.

</details>

## 사용법

**Claude Code나 Codex에 이렇게 말하세요.**

```text
Leeway 리포트 만들어줘
```

비용 점검은 따로 묻지 않고 바로 진행합니다. 정밀 점검은 세션을 판정 모델(내 `claude`나 `codex` CLI)에 보내기 전에 어떤 세션을 보낼지 보여 주고 먼저 묻습니다. Claude Code에서 부르면 Claude Code 세션만, Codex에서 부르면 Codex 세션만 판정합니다. 둘 다 보고 싶으면 그렇게 말하면 됩니다.

터미널에서 직접 실행해도 됩니다. 이때는 Claude Code와 Codex 세션을 모두 봅니다.

```sh
agent-leeway leak --lang ko              # 비용 점검만
agent-leeway drift --dry-run --lang ko   # 정밀 점검할 세션 목록만 보기 (아무것도 보내지 않음)
agent-leeway drift --lang ko             # 정밀 점검 실행 (시작 전에 물어봄)
agent-leeway report --lang ko            # 리포트: 비용 점검 + 지금까지 정밀 점검한 결과
```

기본으로 최근 30일을 보고, 그중 아직 점검하지 않은 가장 최근 세션 50개까지 정밀 점검합니다. 명령과 옵션 전체는 [docs/commands.md](docs/commands.md)에 있습니다.

## 리포트에 나오는 것

`report`는 HTML 파일과 같은 내용의 Markdown 파일을 `~/.cache/agent-leeway/reports`(Windows는 `%LOCALAPPDATA%\agent-leeway\reports`)에 만들고, 터미널에 짧은 요약을 출력합니다. 리포트에는 이런 내용이 들어갑니다.

- 비용 점검과 정밀 점검의 총평, 그리고 같은 길이의 직전 기간과 비교
- 가장 효과가 큰 개선점 3가지와 각각의 근거
- 잘하고 있는 것
- 에이전트가 요청에서 벗어난 곳과 비용이 샌 곳(원인이 된 사용 습관별로 묶음)
- 세션별 상세 내역

정밀 점검은 내가 에이전트를 바로잡아야 했던 순간마다 원인을 셋 중 하나로 나눕니다.

- 에이전트가 이미 받은 지시를 어겼거나 스스로 실수했다
- 내 지시가 없거나 모호했다. 이때는 `CLAUDE.md`나 `AGENTS.md`에 추가할 문장을 제안합니다
- 로그만으로는 판단할 수 없다

동작 방식은 [docs/how-it-works.md](docs/how-it-works.md)에 있습니다.

## 밖으로 나가는 것과 비용

- 비용 점검과 `report`는 아무것도 보내지 않고, 비용도 들지 않습니다.
- 정밀 점검은 내가 승인한 세션의 일부 내용을 Anthropic이나 OpenAI로 보냅니다. 내 `claude`나 `codex` CLI와 그 로그인을 그대로 씁니다.
- 세션마다 한 번 호출하고, 결과를 주제별로 정리할 때 Claude Code와 Codex에 한 번씩 더 호출합니다.
- 구독 요금제를 쓰면 사용량 한도에서 차감됩니다. API 키를 쓰면 세션당 약 $0.15~$0.35입니다(기본값 50개 세션이면 약 $8~$18).

`agent-leeway privacy`를 실행하면 읽고 쓰는 폴더와 밖으로 나가는 데이터를 모두 보여 줍니다. 자세한 내용은 [docs/privacy.md](docs/privacy.md)에 있습니다.

## 더 읽을거리

아래 문서는 모두 영어입니다.

- [docs/install.md](docs/install.md): 다른 설치 방법, Windows, 업데이트와 삭제
- [docs/commands.md](docs/commands.md): 명령, 옵션, 설정, 평가 기준(rubric)
- [docs/how-it-works.md](docs/how-it-works.md): 비용 점검, 정밀 점검, 주제별 묶기
- [docs/privacy.md](docs/privacy.md): 무엇을 읽고 무엇을 보내는지, 판정 모델이 할 수 있는 일
- [docs/validation.md](docs/validation.md): 정밀 점검 결과가 공개 연구의 기준 라벨과 얼마나 맞는지, 그리고 한계
- [docs/rubric.md](docs/rubric.md): 판정 모델이 따르는 평가 기준. 근거는 [docs/rubric-evidence.md](docs/rubric-evidence.md), [docs/references.md](docs/references.md)
- [docs/development.md](docs/development.md): 빌드와 테스트
- [CHANGELOG.md](CHANGELOG.md), [SECURITY.md](SECURITY.md), [CONTRIBUTING.md](CONTRIBUTING.md)

MIT 라이선스입니다([LICENSE](LICENSE), [NOTICE.md](NOTICE.md)). Leeway는 Anthropic, OpenAI와 관계없는 비공식 프로젝트입니다.
