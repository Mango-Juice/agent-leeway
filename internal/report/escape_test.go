package report

import (
	"strings"
	"testing"
)

// Judge-written text cannot add links, images, HTML or new lines to the Markdown report (docs/20 L9).
func TestJudgeTextIsNeutralizedInMarkdown(t *testing.T) {
	got := mq("ok ![x](https://host/?q=secret) <img src=x>\n## Heading `code`")
	for _, bad := range []string{"![", "](", "<img", "\n", "`"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q survives: %s", bad, got)
		}
	}
}
