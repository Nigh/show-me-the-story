package httpapi

import (
	"net/http/httptest"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"strings"
	"testing"
)

func TestBatchRequestValidationDoesNotMutateProject(t *testing.T) {
	h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	h.state.CorePrompt = "permanent rules"
	h.state.LongTermDirection = "existing direction"
	for _, body := range []string{
		`{"chapter_count":12,"requirements":"legacy field is not a batch synopsis"}`,
		`{"chapter_count":12,"outline_synopsis":"  "}`,
		`{"chapter_count":37,"outline_synopsis":"plot"}`,
		`{"chapter_count":12,"outline_synopsis":"plot","mode":"replace_last","batch_id":1}`,
	} {
		req := httptest.NewRequest("POST", "/api/outline/generate-continuation", strings.NewReader(body))
		req.Header.Set("X-UI-Locale", "en")
		res := httptest.NewRecorder()
		h.PostOutlineGenerateContinuation(res, req)
		if res.Code != 400 {
			t.Fatalf("got %d: %s", res.Code, res.Body.String())
		}
		if h.isTaskRunning() {
			t.Fatal("rejected request leaked task lock")
		}
		if h.state.CorePrompt != "permanent rules" || h.state.LongTermDirection != "existing direction" {
			t.Fatal("rejected request changed story inputs")
		}
	}
}
