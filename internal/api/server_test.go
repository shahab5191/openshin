package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shahab5191/openshin/internal/db/testdb"
	"github.com/shahab5191/openshin/internal/memory"
	"github.com/shahab5191/openshin/internal/store"
)

func newTestServer(t *testing.T, ctx ContextAssembler) *httptest.Server {
	t.Helper()
	s := NewServer(store.NewConversations(testdb.Open(t)), ctx)
	ts := httptest.NewServer(s.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func TestIngestAndReadRange(t *testing.T) {
	ts := newTestServer(t, nil)

	body := `{"session_id":"s1","actor":"user","role":"user","content":"hello"}`
	resp, err := http.Post(ts.URL+"/v1/ingest", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/v1/threads/s1/messages")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	got := readBody(t, resp)
	if !strings.Contains(got, `"content":"hello"`) {
		t.Fatalf("response missing content: %s", got)
	}
}

func TestIngestRejectsEmptyContent(t *testing.T) {
	ts := newTestServer(t, nil)

	body := `{"session_id":"s1","actor":"user","role":"user","content":""}`
	resp, err := http.Post(ts.URL+"/v1/ingest", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestContextEndpoint(t *testing.T) {
	fake := &fakeContext{c: memory.Context{RollingSummary: "summary", Messages: nil}}
	ts := newTestServer(t, fake)

	resp, err := http.Get(ts.URL + "/v1/threads/t1/context")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(readBody(t, resp), `"rolling_summary":"summary"`) {
		t.Fatalf("missing summary: %s", readBody(t, resp))
	}
}

type fakeContext struct {
	c memory.Context
}

func (f *fakeContext) Assemble(ctx context.Context, threadID string) (memory.Context, error) {
	return f.c, nil
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}
