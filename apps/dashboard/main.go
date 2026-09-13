// Command dashboard lists the preview environments running on this host and
// streams their logs. It only reads: the Docker API it talks to is a proxy
// that refuses anything but GET requests for containers.
package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates static
var assets embed.FS

func main() {
	s := newServer(getenv("DOCKER_HOST", "http://docker-proxy:2375"), os.Getenv("INFRA_PROJECT"))
	addr := getenv("LISTEN", ":8080")
	log.Printf("listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type server struct {
	docker *dockerAPI
	// infraProject is the compose project the host's own services belong to.
	// Their logs are shown alongside the previews'.
	infraProject string
	pages        *template.Template
}

func newServer(dockerHost, infraProject string) *server {
	return &server{
		docker:       newDockerAPI(dockerHost),
		infraProject: infraProject,
		pages:        template.Must(template.ParseFS(assets, "templates/*.html")),
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /logs/{name}", s.logPage)
	mux.HandleFunc("GET /logs/{name}/stream", s.logStream)
	mux.Handle("GET /static/", http.FileServerFS(assets))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "ok\n")
	})
	return withHeaders(mux)
}

// withHeaders limits the pages to their own scripts and styles.
func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

type overview struct {
	Projects []project
	Services []container
	Err      string
	Updated  time.Time
}

func (s *server) overview(ctx context.Context) overview {
	o := overview{Updated: time.Now()}
	cs, err := s.docker.containers(ctx)
	if err != nil {
		o.Err = err.Error()
		return o
	}
	var previews []preview
	for _, c := range cs {
		if p, ok := previewOf(c); ok {
			previews = append(previews, p)
		} else if s.isService(c) {
			o.Services = append(o.Services, c)
		}
	}
	o.Projects = byProject(previews)
	slices.SortFunc(o.Services, func(a, b container) int { return strings.Compare(a.Name(), b.Name()) })
	return o
}

func (s *server) isService(c container) bool {
	return s.infraProject != "" && c.Labels["com.docker.compose.project"] == s.infraProject
}

// find looks up a container by name if the dashboard shows its log: a
// preview's, or one of the host's own services'. Other containers stay out of
// reach even though the Docker API would serve them.
func (s *server) find(ctx context.Context, name string) (container, *preview, bool, error) {
	cs, err := s.docker.containers(ctx)
	if err != nil {
		return container{}, nil, false, err
	}
	for _, c := range cs {
		if c.Name() != name {
			continue
		}
		if p, ok := previewOf(c); ok {
			return c, &p, true, nil
		}
		return c, nil, s.isService(c), nil
	}
	return container{}, nil, false, nil
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "couldn't render the page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	buf.WriteTo(w)
}

// index is the overview page; with ?partial it is just the list, which the
// page fetches to refresh itself.
func (s *server) index(w http.ResponseWriter, r *http.Request) {
	page := "index"
	if r.URL.Query().Has("partial") {
		page = "list"
	}
	s.render(w, page, s.overview(r.Context()))
}

var tails = []string{"200", "1000", "5000", "all"}

const defaultTail = "1000"

// parseTail accepts a line count or "all"; anything else gets the default.
func parseTail(v string) string {
	if v == "all" {
		return v
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100_000 {
		return strconv.Itoa(n)
	}
	return defaultTail
}

type logView struct {
	Container container
	Preview   *preview
	Tails     []string
	Tail      string
}

func (s *server) logPage(w http.ResponseWriter, r *http.Request) {
	c, p, ok, err := s.find(r.Context(), r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.render(w, "logs", logView{Container: c, Preview: p, Tails: tails, Tail: defaultTail})
}

// logStream sends a container's log as server-sent events: `line` for each
// line, then one `end` saying why the stream stopped.
func (s *server) logStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, _, ok, err := s.find(ctx, r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	// Send the headers now: a quiet container may not log anything for a
	// while, and the browser shows the stream as connecting until they arrive.
	flusher.Flush()

	events := &eventWriter{w: bufio.NewWriter(w), flush: flusher.Flush}
	stop := events.pump(ctx)
	defer stop()
	events.send("end", s.streamLogs(ctx, c, parseTail(r.URL.Query().Get("tail")), events))
}

// streamLogs sends the container's last lines and then follows its log,
// returning why it stopped. The socket proxy drops a connection that stays
// quiet for too long, so while the container is still running the stream is
// reopened from the last line sent.
func (s *server) streamLogs(ctx context.Context, c container, tail string, events *eventWriter) string {
	q := logQuery{Tail: tail, Follow: true}
	var last time.Time
	resumed := false
	for {
		in, err := s.docker.inspect(ctx, c.ID)
		if ctx.Err() != nil {
			return ""
		}
		if err != nil {
			if resumed {
				return "The container is gone. A new deploy may have replaced it; reload to follow the new one."
			}
			return "Couldn't read the container: " + err.Error()
		}
		if resumed && !in.State.Running {
			return "The container stopped."
		}
		err = s.docker.logs(ctx, c.ID, in.Config.Tty, q, func(l logLine) error {
			if resumed && !l.Time.IsZero() && !l.Time.After(last) {
				return nil // already sent before the stream was reopened
			}
			if !l.Time.IsZero() {
				last = l.Time
			}
			events.send("line", l)
			return ctx.Err()
		})
		if ctx.Err() != nil {
			return ""
		}
		if err != nil {
			return "The log stream failed: " + err.Error()
		}
		if !in.State.Running {
			return "The container isn't running, so this is the end of its log."
		}
		resumed = true
		q = logQuery{Tail: "all", Since: last, Follow: true}
		if last.IsZero() {
			q.Tail = "0"
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(time.Second):
		}
	}
}

// eventWriter writes server-sent events. Log lines can arrive far faster than
// it is worth flushing, so pump flushes them in batches.
type eventWriter struct {
	mu        sync.Mutex
	w         *bufio.Writer
	flush     func()
	dirty     bool
	lastWrite time.Time
}

func (e *eventWriter) send(event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, b)
	e.dirty = true
}

// heartbeat keeps a quiet stream from looking dead to the proxies in between.
const heartbeat = 20 * time.Second

// pump flushes pending events every 100ms and sends a heartbeat when the
// stream has been quiet. The returned stop flushes what is left and must run
// before the handler returns.
func (e *eventWriter) pump(ctx context.Context) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	e.lastWrite = time.Now()
	go func() {
		defer close(finished)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case now := <-tick.C:
				e.mu.Lock()
				if !e.dirty && now.Sub(e.lastWrite) >= heartbeat {
					io.WriteString(e.w, ": ping\n\n")
					e.dirty = true
				}
				e.flushLocked(now)
				e.mu.Unlock()
			}
		}
	}()
	return func() {
		close(done)
		<-finished
		e.mu.Lock()
		e.flushLocked(time.Now())
		e.mu.Unlock()
	}
}

func (e *eventWriter) flushLocked(now time.Time) {
	if !e.dirty {
		return
	}
	e.w.Flush()
	e.flush()
	e.dirty = false
	e.lastWrite = now
}
