package messages

// Korean text of the drift and report commands (pre-run text, progress, results, folder notes, privacy).
// SetLang("ko") switches the package tables to these; keys missing here stay English, and
// TestKoreanCoversDriftAndCli fails unless the only English ones are those in KeepEnglish.

// KeepEnglish are Drift keys that are not shown as interface text: "truncated" is part of the judge input for long
// sessions, and "instr_current_warning" is stored in each result. They stay English in every language.
var KeepEnglish = map[string]bool{"truncated": true, "instr_current_warning": true}

var driftKO = map[string]string{
	"no_cli":                "경고: {tool} CLI를 찾지 못했습니다. {tool} 세션은 판정할 수 없습니다.",
	"wrong_provider":        "{tool} 세션은 {tool} 모델로만 판정합니다(지정된 값: {tier}).",
	"lower_tier":            "경고({tier}): {msg}",
	"none":                  "이 기간에 요청이 있는 직접 대화 세션이 없습니다.",
	"head":                  "이탈 점검 · 세션 {n}개({scope}, 최근 것부터{limit}) · 규칙집 {ver}",
	"scope_direct":          "직접 대화",
	"scope_all":             "직접 대화와 자동화",
	"limit":                 ", 최대 {n}개",
	"resumed":               "이미 판정한 세션 {n}개는 건너뜁니다(이어서 하기).",
	"est_unpriced":          "  {tier}: 세션 {n}개, 가격 모름(가격표에 이 모델이 없음)",
	"est_line":              "  {tier}: 세션 {n}개, 예상 ${usd:.2f}{ref}",
	"preflight_failed":      "판정 CLI 확인에 실패해 아무것도 보내지 않았습니다: {msg}",
	"no_cli_any":            "선택한 세션 {n}개를 판정할 CLI가 없습니다. `claude`(Claude Code)나 `codex`(Codex)를 설치하고 로그인한 뒤 다시 실행하세요.",
	"no_cli_skipped":        "  {tool} 세션 {n}개는 뺍니다. {tool} CLI를 찾지 못했습니다.",
	"judge_cmd_set":         "경고: LEEWAY_JUDGE_CMD가 설정되어 있습니다({cmd}). 세션은 제공사 CLI가 아니라 이 로컬 프로그램으로 갑니다.",
	"budget_unpriced":       "{tier}은(는) 가격표에 가격이 없어 예산 상한(${b:.2f})을 지킬 수 없습니다. 가격표에 있는 모델을 고르거나, prices.json에 행을 추가하거나, 상한 없이 하려면 --budget 0을 쓰세요.",
	"est_upper":             "  입력 기준 최대 예상치입니다. 제공사의 프롬프트 캐시는 셈하지 않았습니다. 같은 세션을 곧 다시 판정하면 캐시를 다시 써서 더 쌉니다.",
	"plan_line":             "이 기간 세션 {n}개 중 최근 {m}개를 판정합니다(세션마다 판정 호출 한 번). 구독이면 요금제 사용량에서 빠지고, API 키면 정가로 약 ${usd:.2f}입니다.",
	"cut_max":               "세션 {total}개 중 최근 {k}개만 판정합니다(--max-sessions {k}, 같으면 세션 id 순). 더 하려면 --max-sessions N, {total}개 전부 하려면 --max-sessions 0.",
	"cut_budget":            "--budget ${b:.2f}(API 정가 기준)에 따라 판정할 세션 {todo}개 중 최근 것부터 약 {m}개 뒤에 멈춥니다(같으면 세션 id 순). 늘리려면 --budget USD, 금액 상한을 없애려면 빼세요.",
	"includes_instructions": "발췌마다 로그에 있는 지시 파일(CLAUDE.md, AGENTS.md)이 들어갑니다. 지시 파일이 없는 Claude Code 세션에는 프로젝트 폴더의 현재 CLAUDE.md, CLAUDE.local.md, AGENTS.md와 전역 CLAUDE.md를 붙입니다(--no-current-instructions로 끌 수 있음).",
	"start_smaller":         "처음 실행치고는 큽니다. 작게 시작하려면: --max-sessions 10, --budget 2 또는 --project <이름>.",
	"list_line":             "  {date} {tool:11} {project}: 약 {ktok:,.0f}k 토큰({kb:,.0f} KB)",
	"list_more":             "  ... 그리고 {n}개 더",
	"inputs_written":        "세션 {n}개의 판정 입력을 그대로 {dir}에 썼습니다(세션마다 파일 하나, system-prompt.md 포함). 아무것도 보내지 않았습니다.",
	"none_judged":           "아직 판정한 세션이 없습니다.",
	"est_ref":               " (세션당 최소 ${ref:.3f}: 검증 실행 평균)",
	"est_total":             "  예상 합계 ${usd:.2f}(API 정가 기준). CLI에 로그인한 계정으로 청구됩니다(구독이면 요금제 사용량, API 키면 돈){budget}",
	"est_budget":            ", 예산 상한 ${b:.2f}",
	"nothing":               "새로 판정할 세션이 없습니다.",
	"state_on":              "켬",
	"state_off":             "끔",
	"fable_refused":         "Fable 계열 모델은 판정에 쓰지 않습니다. 다른 모델을 고르세요.",
	"group_only_conflict":   "--group-only와 --no-grouping은 함께 쓸 수 없습니다",
	"group_nothing":         "묶기, {tool}: 묶을 것이 없습니다(문제가 있는 세션이 두 개 미만). 호출하지 않습니다.",
	"group_unchanged":       "묶기, {tool}: 지난번 묶은 뒤로 판정한 문제가 그대로입니다. 호출하지 않습니다.",
	"group_budget":          "묶기, {tool}: 예산 ${b:.2f}을 넘게 되어 건너뜁니다.",
	"group_limit":           "묶기, {tool}: 사용량 한도로 멈췄습니다. 리포트는 문제를 하나씩 보여 줍니다.",
	"group_not_run":         "묶기, {tool}: 실행하지 않았습니다({msg}).",
	"group_failed":          "묶기, {tool}: 실패했습니다({msg}). 리포트는 문제를 하나씩 보여 줍니다.",
	"group_done":            "묶기, {tool}: 여러 세션에 걸친 주제 {n}개.",
	"group_done_dropped":    "묶기, {tool}: 여러 세션에 걸친 주제 {n}개, 검사에서 뺀 것 {d}개.",
	"stale_prices":          "참고: 가격표가 {days}일 지났습니다({read}에 확인). `agent-leeway prices`를 제공사 페이지와 비교하거나 agent-leeway를 업데이트하세요.",
	"ended_promo":           "참고: 가격표에 있는 {model}의 할인 가격이 {date}에 끝났습니다. `agent-leeway prices`를 확인하거나 agent-leeway를 업데이트하세요.",
	"grouping_state":        "판정 뒤 뜻으로 묶기: {state}. 도구별로 끄거나 켜려면 --grouping claude=off(또는 settings.json \"grouping\").",
	"grouping_none_on":      "판정한 모든 도구에서 묶기가 꺼져 있어 할 일이 없습니다. 켜려면 --grouping claude=on 또는 --grouping codex=on.",
	"by_tool":               "이 기간 세션: Claude Code {claude}개 · Codex {codex}개",
	"left_out":              "이 기간의 {tool} 세션 {n}개는 이번에 판정하지 않습니다(--tool all이면 포함).",
	"grouping_plan":         "판정 뒤 묶기 호출 {n}번({tools}), 합계 약 ${usd:.2f}: 제공사마다 자기 세션에서 판정한 문제(판정자가 이미 본 인용과 요약)만 받아 뜻으로 묶습니다. --no-grouping이면 건너뜁니다.",
	"dry_run":               "--dry-run: 예상만 보여 주고 아무것도 판정하지 않았습니다.",
	"need_yes":              "확인이 필요합니다. 대화형 터미널에서 실행하거나 --yes를 붙이세요.",
	"ask":                   "진행할까요? [y/N] ",
	"cancelled":             "취소했습니다.",
	"judged":                "{n}개 판정, 실제 비용 ${usd:.2f}{reason}",
	"stopped":               " · 멈춤: {reason}",
	"resume_hint":           "같은 명령을 다시 실행하면 이어서 합니다. 이미 판정한 세션은 건너뜁니다.",
	"stop_budget":           "예산 상한 ${b:.2f}에 도달",
	"stop_limit":            "제공사가 사용량 한도 응답을 보내 바로 멈췄습니다({msg})",
	"stop_cancel":           "취소됨(실행 중이던 호출은 기록하지 않음, 다시 실행하면 이어서 함)",
	"stop_config":           "판정 CLI를 안전하게 실행할 수 없습니다: {msg}",
	"stop_fail":             "판정 호출이 연달아 두 번 실패했습니다(원인을 확인한 뒤 다시 실행하면 이어서 함)",
	"progress":              "  {i}/{n} {tool} {project}: {verdict} (${usd:.3f})",
	"failed":                "  실패: {tier} ({err})",
	"result":                "이탈 점검: 세션 {n}개 · {verdicts} · 버그 신고 {defects}건 · 판정 모델 {models}{extra}",
	"result_partial":        " · {n}개는 너무 길어 가운데를 빼고 판정",
	"result_custom":         " · 직접 고친 규칙집",
	"see_report":            "자세한 내용은 `report` 명령이 쓰는 Markdown/HTML 파일에 있습니다.",
}

var cliKO = map[string]string{
	"description":      "AI 코딩 에이전트 로그(Claude Code, Codex)를 가끔 점검합니다: 기계적인 비용 누수와 의도 이탈.",
	"since_format":     "--since 형식: 30d, 2w, 12h",
	"rubric_exists":    "{path}이(가) 이미 있어 아무것도 바꾸지 않았습니다. 다른 이름을 쓰거나, 바꾸려면 --force를 붙이세요(이전 파일은 백업으로 남김).",
	"rubric_backed_up": "이전 파일을 {path}에 남겼습니다.",
	"rubric_copied":    "규칙집을 {path}에 복사했습니다. 고친 뒤 --rubric {path}로 넘기거나 설정 파일에 \"rubric\"을 적으세요.",
	"rubric_file":      "규칙집 파일: {path} ({ver})",
	"privacy_details": "자세히:\n" +
		"  - 판정 호출은 캐시 폴더 아래 새 비공개 폴더에서 실행하고, 세션은 표준 입력으로만 보냅니다.\n" +
		"  - 묶기 호출(판정 뒤 제공사마다 한 번)은 같은 CLI 명령을 쓰고, 그 제공사에서 판정한 문제만 보냅니다: id, 규칙·원인 코드, 판정자가 본 그대로의 지시 인용과 내 말 인용(가리지 않음), 판정자 요약. 결과는 drift/groups.jsonl에 두며, `report`는 이것을 읽기만 하고 호출하지 않습니다.\n" +
		"  - Claude CLI가 강제하는 것: 내장 도구 없음(--tools \"\"), 사용자·프로젝트·로컬 설정과 훅 없음(--restricted), CLAUDE.md·스킬·플러그인·MCP 서버 없음(--safe-mode, --strict-mcp-config), 슬래시 명령 없음, 권한 요청 거부, 세션 저장 안 함.\n" +
		"  - Codex CLI가 강제하는 것: 셸·실행·이미지·앱·플러그인·훅·메모리·하위 에이전트 도구 끔, 웹 검색 끔, 사용자 설정·규칙·프로젝트 설정 읽지 않음, 읽기 전용 샌드박스, 승인 없음. 그래도 도구 호출이 보이면 멈추고 기록하지 않습니다.\n" +
		"  - 프롬프트로만 요청하는 것(강제 아님): 대화 기록을 데이터로만 판정하고 JSON으로만 답할 것.\n" +
		"  - 판정 CLI는 내 환경을 물려받으므로 ANTHROPIC_BASE_URL, OPENAI_BASE_URL, 프록시 같은 설정이 적용됩니다.\n" +
		"  - drift는 로그에 지시 파일이 없는 Claude Code 세션에 프로젝트 폴더의 현재 CLAUDE.md, CLAUDE.local.md, AGENTS.md(와 전역 CLAUDE.md)를 붙입니다. --no-current-instructions로 끌 수 있습니다.\n" +
		"  - 모든 명령은 로그에 나온 프로젝트 폴더에서 프로젝트 이름을 알려고 `git rev-parse`(읽기만 함)를 실행합니다.\n" +
		"  - LEEWAY_JUDGE_CMD를 설정하면 판정 CLI 대신 로컬 프로그램을 씁니다(drift가 알려 줌). LEEWAY_RELEASE_API는 `self-update --check`가 묻는 곳을 바꿉니다.\n" +
		"  - 가격은 로컬 가격표({prices})에서 가져오며 아무것도 내려받지 않습니다.",
	"privacy_paths":         "캐시: {cache}\n리포트: {reports}\n설정: {settings}\nClaude Code 로그: {claude}\nCodex 로그: {codex}",
	"folder_read":           "{tool} 로그를 읽음: {dir} (세션 파일 {n:,}개, 이 기간에 바뀐 것 {w:,}개)",
	"folder_absent":         "{dir}에 {tool} 로그가 없습니다(폴더 없음, {tool}은(는) 건너뜀).",
	"folder_missing":        "{tool} 로그 폴더 {dir}이(가) 없습니다. --claude-dir / --codex-dir, settings.json, CLAUDE_CONFIG_DIR / CODEX_HOME을 확인하세요.",
	"folder_unreadable":     "{tool} 로그 폴더 {dir}을(를) 읽을 수 없습니다: {err}",
	"folder_sub_unreadable": "  하위 폴더 {n}개를 읽지 못해 건너뛰었습니다.",
	"no_logs":               "Claude Code나 Codex 세션 파일을 찾지 못했습니다. 로그가 다른 곳에 있으면 --claude-dir / --codex-dir을 쓰세요(또는 에이전트처럼 CLAUDE_CONFIG_DIR / CODEX_HOME을 설정).",
	"none_in_window":        "이 기간에 바뀐 세션 파일이 없습니다. 가장 최근 것은 {date}입니다. 더 긴 --since를 써 보세요(예: --since 90d).",
	"claude_retention":      "참고: Claude Code는 기본으로 30일이 지난 대화 기록을 지웁니다(cleanupPeriodDays). 여기서 가장 오래된 것이 {date}이므로, 그 앞 기간(또는 비교 기간)에는 Claude Code 데이터가 없습니다.",
	"reading":               "로그 파일 {n:,}개를 읽는 중(처음에는 오래 걸리고, 다음부터는 캐시를 씀) ...",
	"skip_more":             "  ... 그리고 {n}개 더",
	"skip_unreadable_n":     "열 수 없는 로그 파일 {n}개를 건너뛰었습니다.",
	"broken_lines":          "참고: JSON 객체가 아닌 로그 줄 {n}개를 건너뛰었습니다(대개 아직 쓰이는 중인 세션).",
	"unknown_records":       "참고: 이 버전이 모르는 종류의 로그 기록 {n}개를 무시했습니다({kinds}). 새 에이전트 버전은 다르게 기록할 수 있으니 알려 주세요.",
	"usage_format_changed":  "경고: Codex 로그 {n}개가 이 버전이 읽지 못하는 새 형식으로만 토큰 사용량을 기록해, 그 사용액이 합계에서 빠졌습니다.",
}

var privacyKO = "개인정보: 로그, 캐시, 리포트는 이 컴퓨터에만 있습니다. 밖으로 나가는 것은 두 가지입니다. 이탈 점검은 고른 세션마다 해당 제공사의 공식 CLI(claude -p 또는 codex exec)로 보내고, CLI는 설정된 곳(로그인한 계정, 또는 설정한 base URL이나 프록시)으로 CLI 자체 원격 측정과 함께 보냅니다. 판정 뒤에는 제공사마다 한 번, 그 제공사에서 판정한 문제(판정자가 이미 본 인용과 요약)로 묶기 호출을 합니다(--no-grouping으로 건너뜀). 그리고 `self-update --check`는 GitHub에 최신 릴리스 태그를 묻습니다. 그 밖에는 네트워크에 연결하지 않습니다(자세한 내용은 `privacy`)."

var lowerTierKO = "아래 등급 판정 모델은 대화 기록에 없는 인용을 더 자주 지어냅니다. 인용된 위치를 직접 확인하세요."

var verdictKO = map[string]string{"agent_drift": "에이전트 이탈", "prompt_gap": "지시 부족", "minor_drift": "작은 이탈", "as_intended": "의도대로", "undeterminable": "판단 불가"}

var (
	driftEN, cliEN, verdictEN map[string]string
	privacyEN, lowerTierEN    string
)

func init() {
	driftEN, cliEN, verdictEN = Drift, Cli, Verdict
	privacyEN, lowerTierEN = Privacy, LowerTierWarning
}

func merged(en, ko map[string]string) map[string]string {
	out := make(map[string]string, len(en))
	for k, v := range en {
		out[k] = v
	}
	for k, v := range ko {
		if !KeepEnglish[k] {
			out[k] = v
		}
	}
	return out
}

// SetLang switches the interface text of the drift and report commands (and the shared folder and privacy lines)
// to lang ("ko" or "en"). Text that is part of the judge input or of stored results stays English.
func SetLang(lang string) {
	if lang == "ko" {
		Drift, Cli, Verdict = merged(driftEN, driftKO), merged(cliEN, cliKO), merged(verdictEN, verdictKO)
		Privacy, LowerTierWarning = privacyKO, lowerTierKO
		return
	}
	Drift, Cli, Verdict = driftEN, cliEN, verdictEN
	Privacy, LowerTierWarning = privacyEN, lowerTierEN
}
