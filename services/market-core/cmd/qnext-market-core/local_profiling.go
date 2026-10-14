package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strings"
	"time"
)

const profilingAddress = "127.0.0.1:18081"
const profilingTokenHeader = "X-QNext-Profiling-Token"

// The profiling listener is never registered on the public market API mux.
// Both a long secret and an explicit opt-in are required. It only binds to
// loopback; do not reverse-proxy this port into public /qnext routes.
func startLocalProfiling(ctx context.Context) error {
	if strings.TrimSpace(os.Getenv("QNEXT_LOCAL_PROFILING")) != "1" {
		return nil
	}
	token := strings.TrimSpace(os.Getenv("QNEXT_LOCAL_PROFILING_TOKEN"))
	if len(token) < 24 {
		return errors.New("QNEXT_LOCAL_PROFILING_TOKEN must contain at least 24 characters when profiling is enabled")
	}
	listener, err := net.Listen("tcp", profilingAddress)
	if err != nil {
		return err
	}
	handler := localProfilingHandler(token)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	// Keep sampling overhead fully disabled unless the operator opts in.
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(10_000_000)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		runtime.SetMutexProfileFraction(0)
		runtime.SetBlockProfileRate(0)
	}()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("local profiling server stopped: %v", err)
		}
	}()
	log.Printf("QNext local profiling enabled on %s (loopback only)", profilingAddress)
	return nil
}

func localProfilingHandler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get(profilingTokenHeader)
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
