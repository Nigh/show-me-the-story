package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeHTTPServerShutsDownOnContextCancellation(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(ctx, server, func() error { return server.Serve(listener) })
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveHTTPServer() = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not shut down after cancellation")
	}
}

func TestServeHTTPServerReturnsStartupFailure(t *testing.T) {
	t.Parallel()
	want := errors.New("listen failed")
	server := &http.Server{}
	if err := serveHTTPServer(context.Background(), server, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("serveHTTPServer() = %v, want %v", err, want)
	}
}
