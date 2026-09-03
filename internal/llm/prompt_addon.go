package llm

import "context"

type promptAddonKey struct{}

// WithPromptAddon attaches trusted application-formatted user skill instructions.
func WithPromptAddon(ctx context.Context, addon string) context.Context {
	return context.WithValue(ctx, promptAddonKey{}, addon)
}

func applyPromptAddon(ctx context.Context, messages []Message) []Message {
	addon, _ := ctx.Value(promptAddonKey{}).(string)
	if addon == "" {
		return messages
	}
	out := append([]Message(nil), messages...)
	for i := range out {
		if out[i].Role == "system" {
			out[i].Content += "\n\n## Active user skills\n\n" + addon
			return out
		}
	}
	return append([]Message{{Role: "system", Content: "## Active user skills\n\n" + addon}}, out...)
}
