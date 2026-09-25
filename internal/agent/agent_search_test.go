package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/story"
)

func TestSearchProjectFindsStoredEntries(t *testing.T) {
	ctx := &AgentContext{
		Config: &config.Config{Language: "en"},
		State:  &story.Progress{Foreshadows: []story.Foreshadow{{Name: "Hidden compass"}}},
		Settings: &story.ProjectSettings{
			Characters:    []story.Character{{Name: "Mira", Notes: "Keeps a silver key"}},
			Organizations: []story.Organization{{Name: "Glass Council"}},
			Worldview:     []story.WorldviewEntry{{Name: "North Gate", Tags: "silver arch"}},
		},
	}
	for _, query := range []string{"silver key", "Glass Council", "Hidden compass", "silver arch"} {
		args, _ := json.Marshal(map[string]string{"query": query})
		result, _, _ := executeTool(&ToolCall{Name: "search_project", Arguments: args}, getBuiltinTools(), ctx)
		if strings.Contains(result, "No results") || !strings.Contains(strings.ToLower(result), strings.ToLower(query)) {
			t.Errorf("query %q failed: %q", query, result)
		}
	}
}
