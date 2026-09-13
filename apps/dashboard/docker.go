package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// dockerAPI reads from the Docker Engine API over plain HTTP. In production
// that is a socket proxy which lets only GET requests for containers through.
type dockerAPI struct {
	base   string
	client *http.Client
}

func newDockerAPI(host string) *dockerAPI {
	base := strings.TrimSuffix(strings.Replace(host, "tcp://", "http://", 1), "/")
	// No client timeout: log streams stay open. Each call bounds itself by ctx.
	return &dockerAPI{base: base, client: &http.Client{}}
}

type container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Labels  map[string]string `json:"Labels"`
}

// Name is the container's name without Docker's leading slash.
func (c container) Name() string {
	if len(c.Names) == 0 {
		return c.ID
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

func (d *dockerAPI) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := d.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("docker GET %s: %s %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (d *dockerAPI) getJSON(ctx context.Context, path string, query url.Values, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := d.get(ctx, path, query)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// containers lists every container on the host, running or not.
func (d *dockerAPI) containers(ctx context.Context) ([]container, error) {
	var cs []container
	err := d.getJSON(ctx, "/containers/json", url.Values{"all": {"1"}}, &cs)
	return cs, err
}

// inspection holds the two facts the log stream needs. Docker's full response
// also carries the container's environment, secrets included, which is why
// it is decoded into this narrow struct and goes no further.
type inspection struct {
	Config struct{ Tty bool }
	State  struct{ Running bool }
}

func (d *dockerAPI) inspect(ctx context.Context, id string) (inspection, error) {
	var in inspection
	err := d.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", nil, &in)
	return in, err
}

type logLine struct {
	Stream string    `json:"s"`
	Time   time.Time `json:"t,omitzero"`
	Text   string    `json:"m"`
}

type logQuery struct {
	Tail   string // a line count, or "all"
	Since  time.Time
	Follow bool
}

// logs sends a container's log to emit one line at a time. With Follow it
// runs until the container stops, the connection drops, or ctx ends.
func (d *dockerAPI) logs(ctx context.Context, id string, tty bool, q logQuery, emit func(logLine) error) error {
	query := url.Values{"stdout": {"1"}, "stderr": {"1"}, "timestamps": {"1"}, "tail": {q.Tail}}
	if !q.Since.IsZero() {
		query.Set("since", fmt.Sprintf("%d.%09d", q.Since.Unix(), q.Since.Nanosecond()))
	}
	if q.Follow {
		query.Set("follow", "1")
	}
	resp, err := d.get(ctx, "/containers/"+url.PathEscape(id)+"/logs", query)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if tty {
		return scanLines(resp.Body, "stdout", emit)
	}
	return demux(resp.Body, emit)
}

// maxFrame bounds one frame of Docker's log stream. Docker splits log entries
// at 16 KiB, so a frame near this size means the stream is corrupt.
const maxFrame = 1 << 20

// demux decodes the multiplexed stream Docker sends for a container without a
// TTY: each frame is an 8-byte header (stream id, three zero bytes, and a
// big-endian payload length) followed by the payload.
func demux(r io.Reader, emit func(logLine) error) error {
	var header [8]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		size := binary.BigEndian.Uint32(header[4:])
		if size > maxFrame {
			return fmt.Errorf("log frame of %d bytes", size)
		}
		stream := "stdout"
		if header[0] == 2 {
			stream = "stderr"
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return err
		}
		for _, raw := range strings.Split(strings.TrimSuffix(string(payload), "\n"), "\n") {
			if err := emit(parseLine(stream, raw)); err != nil {
				return err
			}
		}
	}
}

// scanLines reads a TTY container's log, which Docker sends as one raw stream.
func scanLines(r io.Reader, stream string, emit func(logLine) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxFrame)
	for sc.Scan() {
		if err := emit(parseLine(stream, sc.Text())); err != nil {
			return err
		}
	}
	return sc.Err()
}

// ansi matches terminal escape sequences (colours, mostly), which would show
// up in the browser as noise.
var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// parseLine splits off the RFC 3339 timestamp Docker puts before each line.
func parseLine(stream, raw string) logLine {
	raw = strings.TrimSuffix(raw, "\r")
	if stamp, rest, ok := strings.Cut(raw, " "); ok {
		if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			return logLine{Stream: stream, Time: t, Text: ansi.ReplaceAllString(rest, "")}
		}
	}
	return logLine{Stream: stream, Text: ansi.ReplaceAllString(raw, "")}
}
