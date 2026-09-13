package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDocker serves the few Docker API endpoints the dashboard uses. Every
// container reports itself stopped, so log streams end on their own.
func fakeDocker(t *testing.T, logs []byte) *httptest.Server {
	t.Helper()
	cs := []container{
		{ID: "a1", Names: []string{"/preview-pr-253"}, State: "running", Status: "Up 2 hours", Labels: map[string]string{
			"traefik.enable":                   "true",
			"traefik.http.routers.pr-253.rule": "Host(`pr-253.preview.myapp.com`)",
		}},
		{ID: "b2", Names: []string{"/preview-host-traefik-1"}, State: "running", Status: "Up 4 hours", Labels: map[string]string{
			"com.docker.compose.project": "preview-host",
			"com.docker.compose.service": "traefik",
		}},
		{ID: "c3", Names: []string{"/unrelated"}, State: "running", Labels: map[string]string{}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(cs)
	})
	mux.HandleFunc("GET /containers/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"Config":{"Tty":false,"Env":["SECRET=hunter2"]},"State":{"Running":false}}`)
	})
	mux.HandleFunc("GET /containers/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "a1" {
			t.Errorf("logs requested for container %q", r.PathValue("id"))
		}
		if q := r.URL.Query(); q.Get("tail") != "10" || q.Get("follow") != "1" || q.Get("timestamps") != "1" {
			t.Errorf("logs query = %v", q)
		}
		w.Write(logs)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestIndex(t *testing.T) {
	h := newServer(fakeDocker(t, nil).URL, "preview-host").routes()
	code, body := get(t, h, "/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	for _, want := range []string{"myapp", "#253", "https://pr-253.preview.myapp.com", "/logs/preview-pr-253", "traefik", "/logs/preview-host-traefik-1"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview lacks %q", want)
		}
	}
	if strings.Contains(body, "unrelated") {
		t.Error("overview shows a container that is neither a preview nor a host service")
	}
}

func TestLogPageOnlyForKnownContainers(t *testing.T) {
	h := newServer(fakeDocker(t, nil).URL, "preview-host").routes()
	for path, want := range map[string]int{
		"/logs/preview-pr-253":         http.StatusOK,
		"/logs/preview-host-traefik-1": http.StatusOK,
		"/logs/unrelated":              http.StatusNotFound,
		"/logs/unrelated/stream":       http.StatusNotFound,
		"/logs/missing":                http.StatusNotFound,
	} {
		if code, _ := get(t, h, path); code != want {
			t.Errorf("GET %s = %d, want %d", path, code, want)
		}
	}
}

func TestLogStream(t *testing.T) {
	logs := append(frame(1, "2026-09-11T17:30:00Z hello\n"), frame(2, "2026-09-11T17:30:01Z oops\n")...)
	srv := httptest.NewServer(newServer(fakeDocker(t, logs).URL, "preview-host").routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/logs/preview-pr-253/stream?tail=10")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	for _, want := range []string{
		`event: line` + "\n" + `data: {"s":"stdout","t":"2026-09-11T17:30:00Z","m":"hello"}`,
		`data: {"s":"stderr","t":"2026-09-11T17:30:01Z","m":"oops"}`,
		`event: end` + "\n" + `data: "The container isn't running, so this is the end of its log."`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stream lacks %q; got:\n%s", want, body)
		}
	}
	if strings.Contains(body, "hunter2") {
		t.Error("stream leaked the container's environment")
	}
}
