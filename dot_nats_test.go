package xtemplate

import (
	"net/http"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// newInProcessNats starts an in-process (non-listening) NATS server and returns
// a client connection to it. Both are torn down when the test finishes.
func newInProcessNats(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := server.NewServer(&server.Options{DontListen: true})
	if err != nil {
		t.Fatalf("failed to create in-process nats server: %v", err)
	}
	srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("in-process nats server did not become ready")
	}
	nc, err := nats.Connect("", nats.InProcessServer(srv))
	if err != nil {
		srv.Shutdown()
		t.Fatalf("failed to connect to in-process nats server: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		srv.Shutdown()
	})
	return nc
}

// TestDotNats_Request drives the request-reply path: a Go-side responder answers
// on a subject and a template issues a Request to it via the Nats dot field.
func TestDotNats_Request(t *testing.T) {
	nc := newInProcessNats(t)
	if _, err := nc.Subscribe("echo", func(m *nats.Msg) {
		_ = m.Respond([]byte("pong"))
	}); err != nil {
		t.Fatalf("failed to subscribe responder: %v", err)
	}

	inst := buildInstance(t,
		map[string]string{
			"req.html": `{{$m := .Nats.Request "echo" "ping"}}{{printf "%s" $m.Data}}`,
		},
		WithProvider(&DotNatsConfig{Name: "Nats", Conn: nc}),
	)

	w := doRequest(inst, http.MethodGet, "/req")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "pong" {
		t.Errorf("body = %q, want %q", got, "pong")
	}
}

// TestDotNats_Publish verifies a template can publish a message that a Go-side
// subscriber receives.
func TestDotNats_Publish(t *testing.T) {
	nc := newInProcessNats(t)
	got := make(chan string, 1)
	if _, err := nc.Subscribe("events", func(m *nats.Msg) {
		got <- string(m.Data)
	}); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	inst := buildInstance(t,
		map[string]string{
			"pub.html": `{{$_ := .Nats.Publish "events" "hello"}}ok`,
		},
		WithProvider(&DotNatsConfig{Name: "Nats", Conn: nc}),
	)

	w := doRequest(inst, http.MethodGet, "/pub")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	select {
	case msg := <-got:
		if msg != "hello" {
			t.Errorf("received %q, want %q", msg, "hello")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for published message")
	}
}
