package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPendingHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/connector/pending" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("missing/wrong auth header: %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[{"business_code":"SPM","filename":"SPM_facturas.txt","content":"R#..\r\n","batch_token":"abc"}]}`)
	}))
	defer srv.Close()

	batches, err := New(srv.URL, "tok").Pending(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 1 || batches[0].BusinessCode != "SPM" || batches[0].BatchToken != "abc" {
		t.Fatalf("unexpected batches: %+v", batches)
	}
}

func TestPendingNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "bad").Pending(context.Background()); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestConfirmPostsTokenAndReturnsCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/connector/confirm" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["batch_token"] != "tok-123" {
			t.Errorf("unexpected batch_token: %q", body["batch_token"])
		}
		_, _ = io.WriteString(w, `{"delivered":3}`)
	}))
	defer srv.Close()

	n, err := New(srv.URL, "tok").Confirm(context.Background(), "tok-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected delivered 3, got %d", n)
	}
}

func TestBaseURLTrailingSlashTrimmed(t *testing.T) {
	c := New("https://x/", "t")
	if strings.HasSuffix(c.baseURL, "/") {
		t.Fatalf("trailing slash not trimmed: %q", c.baseURL)
	}
}
