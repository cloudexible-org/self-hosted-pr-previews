package main

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// frame encodes one frame of Docker's multiplexed log stream.
func frame(stream byte, payload string) []byte {
	b := make([]byte, 8+len(payload))
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:], uint32(len(payload)))
	copy(b[8:], payload)
	return b
}

func TestDemux(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(frame(1, "2026-09-11T17:30:00.123456789Z ready on :3000\n"))
	stream.Write(frame(2, "2026-09-11T17:30:01Z \x1b[31mboom\x1b[0m\n"))
	stream.Write(frame(1, "2026-09-11T17:30:02Z one\n2026-09-11T17:30:02Z two\n"))

	var got []logLine
	if err := demux(&stream, func(l logLine) error { got = append(got, l); return nil }); err != nil {
		t.Fatal(err)
	}
	want := []logLine{
		{Stream: "stdout", Time: time.Date(2026, 9, 11, 17, 30, 0, 123456789, time.UTC), Text: "ready on :3000"},
		{Stream: "stderr", Time: time.Date(2026, 9, 11, 17, 30, 1, 0, time.UTC), Text: "boom"},
		{Stream: "stdout", Time: time.Date(2026, 9, 11, 17, 30, 2, 0, time.UTC), Text: "one"},
		{Stream: "stdout", Time: time.Date(2026, 9, 11, 17, 30, 2, 0, time.UTC), Text: "two"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Stream != want[i].Stream || !got[i].Time.Equal(want[i].Time) || got[i].Text != want[i].Text {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDemuxRejectsHugeFrame(t *testing.T) {
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:], maxFrame+1)
	if err := demux(bytes.NewReader(header), func(logLine) error { return nil }); err == nil {
		t.Fatal("want an error for an oversized frame")
	}
}

func TestParseLineWithoutTimestamp(t *testing.T) {
	l := parseLine("stdout", "no timestamp here\r")
	if !l.Time.IsZero() || l.Text != "no timestamp here" {
		t.Errorf("got %+v", l)
	}
}

func TestParseTail(t *testing.T) {
	for in, want := range map[string]string{"": "1000", "200": "200", "all": "all", "0": "1000", "-5": "1000", "abc": "1000", "999999": "1000"} {
		if got := parseTail(in); got != want {
			t.Errorf("parseTail(%q) = %q, want %q", in, got, want)
		}
	}
}
