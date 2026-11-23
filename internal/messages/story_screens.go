package messages

// Screenshots left in the conversation, a cause of "money wasted by habits" (docs/31 B2), merged into Story and
// ReportText. Taking screenshots is fine; the cost is keeping them in a long conversation.
var storyScreens = map[string]map[string]string{
	"en": {
		"pl.screenshots.action":    "Once the visual check is done, compact or start a new session",
		"pl.screenshots.short":     "Compact after a visual check",
		"pl.screenshots.hook":      "{n} screenshots left in the conversation cost about {usd}.",
		"pl.screenshots.src":       "costs",
		"pl.screenshots.hook.one":  "1 screenshot left in the conversation cost about {usd}.",
		"pl.screenshots.why":       "Taking screenshots is fine; each one left in the chat is billed again on every later request.",
		"pl.screenshots.why.codex": "Taking screenshots is fine; each one left in the chat is billed again on every later request until /compact or /new.",
		"prob.screenshots":         "Leaving screenshots in the conversation",
		"about_usd":                "about {usd}",
		"shots_est":                "estimated from each image's size; counts screenshots and other images",
		"one_sess_all":             "All of it in one session.",
		"cmp_why_cost_causes":      "No cost comparison: this version counts one more cause than the earlier report.",
	},
	"ko": {
		"pl.screenshots.action":    "화면 확인이 끝나면 압축하거나 새 세션으로 옮기세요",
		"pl.screenshots.short":     "화면 확인 뒤 압축하기",
		"pl.screenshots.hook":      "대화에 남겨 둔 스크린샷 {n}장 때문에 약 {usd}가 나갔습니다.",
		"pl.screenshots.src":       "costs",
		"pl.screenshots.hook.one":  "대화에 남겨 둔 스크린샷 1장 때문에 약 {usd}가 나갔습니다.",
		"pl.screenshots.why":       "스크린샷을 찍는 것은 괜찮습니다. 대화에 남은 스크린샷은 이후 요청마다 다시 청구됩니다.",
		"pl.screenshots.why.codex": "스크린샷을 찍는 것은 괜찮습니다. 대화에 남은 스크린샷은 /compact나 /new 전까지 이후 요청마다 다시 청구됩니다.",
		"prob.screenshots":         "스크린샷을 대화에 남겨 두기",
		"about_usd":                "약 {usd}",
		"shots_est":                "이미지 크기로 추정했으며, 스크린샷 말고 다른 이미지도 셉니다",
		"one_sess_all":             "모두 한 세션에서 나갔습니다.",
		"cmp_why_cost_causes":      "이 버전은 이전 리포트보다 원인을 하나 더 세므로 비용은 비교하지 않습니다.",
	},
}

var reportScreens = map[string]map[string]string{
	"en": {"H_screenshots": "Leaving screenshots in the conversation"},
	"ko": {"H_screenshots": "스크린샷을 대화에 남겨 두기"},
}

func init() {
	for lang, m := range storyScreens {
		for k, v := range m {
			Story[lang][k] = v
		}
	}
	for lang, m := range reportScreens {
		for k, v := range m {
			ReportText[lang][k] = v
		}
	}
}
