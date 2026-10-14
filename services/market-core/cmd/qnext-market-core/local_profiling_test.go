package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalProfilingDisabledByDefault(t *testing.T) {
	t.Setenv("QNEXT_LOCAL_PROFILING", "")
	t.Setenv("QNEXT_LOCAL_PROFILING_TOKEN", "")
	if err := startLocalProfiling(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLocalProfilingRejectsShortToken(t *testing.T) {
	t.Setenv("QNEXT_LOCAL_PROFILING", "1")
	t.Setenv("QNEXT_LOCAL_PROFILING_TOKEN", "short")
	if err := startLocalProfiling(context.Background()); err == nil {
		t.Fatal("opt-in with weak token must fail before listener is opened")
	}
}

func TestLocalProfilingHandlerRequiresToken(t *testing.T) {
	const token = "one-long-random-token-for-tests-123456789"
	handler := localProfilingHandler(token)
	for _, tc := range []struct {
		token  string
		method string
		status int
	}{
		{token: "", method: http.MethodGet, status: http.StatusUnauthorized},
		{token: "incorrect", method: http.MethodGet, status: http.StatusUnauthorized},
		{token: token, method: http.MethodPost, status: http.StatusMethodNotAllowed},
		{token: token, method: http.MethodGet, status: http.StatusOK},
	} {
		request := httptest.NewRequest(tc.method, "/debug/pprof/", nil)
		request.Header.Set(profilingTokenHeader, tc.token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != tc.status {
			t.Fatalf("token=%q method=%s got %d want %d", tc.token, tc.method, recorder.Code, tc.status)
		}
	}
}
