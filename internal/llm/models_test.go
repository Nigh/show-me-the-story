package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"showmethestory/internal/config"
)

func TestFetchModels(t *testing.T) {
	for _, tc := range []struct {
		name, base, body string
		strict           bool
		status           int
		wantErr          bool
	}{
		{name: "bare base", body: `{"data":[{"id":"z","context_window":8192},{"id":"a","context_length":4096},{"id":"z"},{"id":""}]}`, status: 200},
		{name: "strict base", base: "/custom", strict: true, body: `{"data":[]}`, status: 200},
		{name: "chat URL", base: "/v1/chat/completions", body: `{"data":[]}`, status: 200},
		{name: "unsupported", body: `secret upstream detail`, status: 404, wantErr: true},
		{name: "malformed", body: `{"data":`, status: 200, wantErr: true},
		{name: "wrong schema", body: `{}`, status: 200, wantErr: true},
		{name: "oversized", body: strings.Repeat(" ", (4<<20)+1), status: 200, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expected := "/v1/models"
				if tc.strict {
					expected = "/custom/models"
				}
				if r.Method != "GET" || r.URL.Path != expected || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("wrong request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			models, err := FetchModels(context.Background(), &config.APIConfig{BaseURL: srv.URL + tc.base, URLStrict: tc.strict, APIKey: "test-key"})
			if (err != nil) != tc.wantErr {
				t.Fatal(models, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret upstream") {
				t.Fatal("provider body leaked")
			}
			if tc.name == "bare base" && (len(models) != 2 || models[0].ID != "a" || models[0].ContextWindow != 4096 || models[1].ContextWindow != 8192) {
				t.Fatal(models)
			}
			if !tc.wantErr && models == nil {
				t.Fatal("empty list must be an array")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchModels(ctx, &config.APIConfig{BaseURL: "http://127.0.0.1:1"}); err == nil {
		t.Fatal("cancellation ignored")
	}
	if _, err := FetchModels(context.Background(), nil); err == nil {
		t.Fatal("missing config accepted")
	}
}
