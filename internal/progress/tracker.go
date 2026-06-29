// Package progress provides a best-effort view of what a long-running rotki-core
// async task is currently doing, used to enrich the heartbeat that the async
// task manager logs while waiting.
//
// It draws on two independent sources, because neither alone is sufficient:
//
//   - a websocket to rotki-core answers *what* is happening (decode progress and
//     transaction-query status), e.g. "decoding ethereum 250/1000".
//   - a tail of rotki-core.log answers *why* a task is slow (rate-limit markers),
//     which the websocket does not carry, e.g. "coingecko rate-limited".
//
// Both sources are strictly best-effort: if the websocket never connects or the
// log cannot be read, Snapshot returns less detail (or an empty string). It
// never errors and never blocks the heartbeat.
package progress

import (
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// maxLogScanBytes bounds how much of the log tail is read per Snapshot so a
// huge, fast-growing debug log cannot make the heartbeat expensive.
const maxLogScanBytes = 512 * 1024

// Tracker aggregates websocket progress and log-tail rate-limit causes. The
// zero value is not usable; construct it with NewTracker. All exported methods
// are safe for concurrent use.
type Tracker struct {
	mu sync.Mutex

	// websocket-sourced decode progress (subtype undecoded_transactions)
	decodeChain     string
	decodeProcessed int
	decodeTotal     int
	haveDecode      bool

	// websocket-sourced transaction-query status (fallback when no decode
	// progress has arrived yet)
	statusChain string
	statusStep  string

	// log-tail state
	logPath    string
	logOffset  int64
	logStarted bool
	logErr     bool

	// websocket lifecycle
	stop     chan struct{}
	stopOnce sync.Once
	wsOnce   sync.Once
}

// NewTracker returns a Tracker that reads rate-limit causes from logPath (the
// rotki-core log). The websocket is not started until StartWebsocket is called.
func NewTracker(logPath string) *Tracker {
	return &Tracker{
		logPath: logPath,
		stop:    make(chan struct{}),
	}
}

// Snapshot returns a short human annotation describing the current task state,
// or "" when nothing is known. It combines the websocket "what" with the
// log-tail "why", e.g. "decoding ethereum 250/1000 — coingecko rate-limited".
func (t *Tracker) Snapshot() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	cause := t.scanLogForCauseLocked()

	var what string
	switch {
	case t.haveDecode:
		what = "decoding " + t.decodeChain + " " + strconv.Itoa(t.decodeProcessed) + "/" + strconv.Itoa(t.decodeTotal)
	case t.statusStep != "":
		step := strings.ReplaceAll(t.statusStep, "_", " ")
		if t.statusChain != "" {
			what = t.statusChain + ": " + step
		} else {
			what = step
		}
	}

	switch {
	case what != "" && cause != "":
		return what + " — " + cause + " rate-limited"
	case what != "":
		return what
	case cause != "":
		return cause + " rate-limited"
	default:
		return ""
	}
}

// scanLogForCauseLocked reads the log bytes appended since the last call and
// returns the source of the most recent rate-limit marker found in that window
// (or "" if none). Scoping to the new window means a stale rate-limit from an
// earlier step is not reported as the current cause. The caller must hold t.mu.
func (t *Tracker) scanLogForCauseLocked() string {
	if t.logPath == "" || t.logErr {
		return ""
	}

	info, err := os.Stat(t.logPath)
	if err != nil {
		// The log may not exist yet (core still starting); not fatal.
		return ""
	}
	size := info.Size()

	if !t.logStarted {
		// First observation: only consider markers emitted from here forward.
		t.logStarted = true
		t.logOffset = size
		return ""
	}

	readFrom := t.logOffset
	if size < readFrom {
		// File was rotated or truncated; restart from a bounded tail.
		readFrom = 0
	}
	if size-readFrom > maxLogScanBytes {
		readFrom = size - maxLogScanBytes
	}
	if size <= readFrom {
		return "" // nothing new
	}

	f, err := os.Open(t.logPath) // #nosec G304 - path is the configured core log
	if err != nil {
		t.logErr = true
		logger.Debug("Progress tracker: cannot open log %s: %v", t.logPath, err)
		return ""
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(readFrom, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	t.logOffset = readFrom + int64(len(data))

	return causeFromLogChunk(data)
}

// causeFromLogChunk returns the rate-limited source named in the last
// rate-limit line of chunk, or "" if there is none.
func causeFromLogChunk(chunk []byte) string {
	cause := ""
	for _, line := range strings.Split(string(chunk), "\n") {
		if strings.Contains(strings.ToLower(line), "rate limit") {
			cause = rateLimitSource(line)
		}
	}
	return cause
}

// rateLimitSource extracts the external-API name from a rotki-core log line such
// as "[..] DEBUG rotkehlchen.externalapis.coingecko Greenlet-1: Got rate
// limited ...". When the source cannot be identified it falls back to a generic
// label so the cause is still surfaced.
func rateLimitSource(line string) string {
	const marker = "rotkehlchen.externalapis."
	i := strings.Index(line, marker)
	if i == -1 {
		return "a remote source"
	}
	rest := line[i+len(marker):]
	if end := strings.IndexAny(rest, " .\t"); end != -1 {
		return rest[:end]
	}
	return "a remote source"
}

// Close stops the websocket goroutine (if started). It is safe to call more than
// once and safe to call even if StartWebsocket was never called.
func (t *Tracker) Close() {
	t.stopOnce.Do(func() { close(t.stop) })
}
