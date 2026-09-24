package upstox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizerFetchesOneTimeWebSocketURI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected Accept header %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected Authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"authorized_redirect_uri":"wss://feed.example.test/path?code=one-time"}}`))
	}))
	defer server.Close()

	authorizer := Authorizer{HTTP: server.Client(), Endpoint: server.URL}
	uri, err := authorizer.AuthorizedURI(context.Background(), "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if uri != "wss://feed.example.test/path?code=one-time" {
		t.Fatalf("unexpected URI %q", uri)
	}
}

func TestAuthorizerRejectsNonWebSocketURI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"authorized_redirect_uri":"https://example.test/not-ws"}}`))
	}))
	defer server.Close()

	authorizer := Authorizer{HTTP: server.Client(), Endpoint: server.URL}
	if _, err := authorizer.AuthorizedURI(context.Background(), "secret-token"); err == nil {
		t.Fatal("expected invalid websocket URI to fail")
	}
}
