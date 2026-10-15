package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/sse"
)

func TestAPIModelsUsesUnsavedConfigWithoutModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/models" || r.Header.Get("Authorization") != "Bearer unsaved-key" {
			t.Error("ignored unsaved configuration")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"local-model","context_window":8192}]}`))
	}))
	defer srv.Close()
	h := NewHandlers(&config.APIConfig{Model: "saved-model"}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	for _, tc := range []struct {
		body   string
		busy   bool
		status int
	}{
		{`{"base_url":"` + srv.URL + `/custom","url_strict":true,"api_key":"unsaved-key"}`, false, 200},
		{`{}`, false, 400}, {`{`, false, 400}, {`{}`, true, 409},
	} {
		h.taskRunning = tc.busy
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/config/api/models", strings.NewReader(tc.body))
		req.Header.Set("X-UI-Locale", "en")
		h.PostAPIModels(res, req)
		if res.Code != tc.status {
			t.Fatal(res.Code, res.Body.String())
		}
		if tc.status == 200 && !strings.Contains(res.Body.String(), "local-model") {
			t.Fatal(res.Body.String())
		}
		if h.apiCfg.Model != "saved-model" {
			t.Fatal("discovery saved config")
		}
	}
}
