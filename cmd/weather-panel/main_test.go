package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer lets the test read the service logs while the service is still writing them.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestRunServesUntilCanceled(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	env := map[string]string{
		"PORT":         port,
		"DATABASE_URL": "postgres://weather:weather@127.0.0.1:1/weather",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logs syncBuffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(name string) string { return env[name] }, &logs) }()

	url := "http://127.0.0.1:" + port + "/healthz"
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("service did not start: %v\nlogs:\n%s", err, logs.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not return after cancellation")
	}
	if !strings.Contains(logs.String(), `"msg":"stopped"`) {
		t.Errorf("logs do not record a clean stop:\n%s", logs.String())
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	err := run(context.Background(), func(string) string { return "" }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("run() error = %v, want it to name DATABASE_URL", err)
	}
}
