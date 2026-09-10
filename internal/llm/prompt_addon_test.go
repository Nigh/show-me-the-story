package llm

import (
	"context"
	"strings"
	"testing"
)

func TestApplyPromptAddonAppendsToSystem(t *testing.T) {
	ctx := WithPromptAddon(context.Background(), "skill rules")
	got := applyPromptAddon(ctx, []Message{{Role: "system", Content: "base"}, {Role: "user", Content: "work"}})
	if !strings.Contains(got[0].Content, "base") || !strings.Contains(got[0].Content, "skill rules") {
		t.Fatalf("addon missing: %+v", got)
	}
}
