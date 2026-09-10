package story

import (
	"fmt"
	"showmethestory/internal/i18n"
	"strings"
)

func validateEnding(state *Progress, req OutlineBatchRequest, lang string) error {
	switch req.EndingIntent {
	case "", "serial", "final", "sequel":
	default:
		return fmt.Errorf("%s", i18n.T(lang, "ending_invalid"))
	}
	switch req.EndingStyle {
	case "", "closed", "open", "custom":
	default:
		return fmt.Errorf("%s", i18n.T(lang, "ending_invalid"))
	}
	if req.EndingStyle == "custom" && strings.TrimSpace(req.EndingRequirements) == "" {
		return fmt.Errorf("%s", i18n.T(lang, "ending_invalid"))
	}
	if req.Mode != "replace_last" && !req.ConfirmContinue {
		for _, b := range state.OutlineBatches {
			if b.PlannedFinal {
				return fmt.Errorf("%s", i18n.T(lang, "ending_continue_confirm"))
			}
		}
	}
	return nil
}

func chapterEnding(state *Progress, num int, lang string) string {
	for _, b := range state.OutlineBatches {
		if num >= b.StartCh && num <= b.EndCh {
			return endingPrompt(b, num, lang)
		}
	}
	return ""
}
