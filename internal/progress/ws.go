package progress

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/kelsos/rotki-sync/internal/logger"
)

// rotki-core mounts its websocket at /ws on the REST port (not under /api/1).
const wsPath = "/ws"

// wsEnvelope is the outer shape of every rotki-core websocket message:
// {"type": "<message_type>", "data": {...}}.
type wsEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// progressData is the payload of a progress_updates message. Only the
// undecoded_transactions subtype is consumed here.
type progressData struct {
	Chain     string `json:"chain"`
	Subtype   string `json:"subtype"`
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
}

// txStatusData is the payload of a transaction_status message (EVM uses a single
// "address", bitcoin uses "addresses"; neither is needed here).
type txStatusData struct {
	Chain  string `json:"chain"`
	Status string `json:"status"`
}

// StartWebsocket connects to the rotki-core websocket on the given port and
// keeps the latest decode/transaction status updated in the background. It is
// idempotent and must be called after the API is ready. A connection failure is
// non-fatal: the tracker simply degrades to log-only (or empty) snapshots.
func (t *Tracker) StartWebsocket(port int) {
	t.wsOnce.Do(func() {
		go t.runWebsocket(fmt.Sprintf("ws://127.0.0.1:%d%s", port, wsPath))
	})
}

// runWebsocket dials the websocket and, on disconnect, reconnects with a capped
// backoff until Close is called.
func (t *Tracker) runWebsocket(url string) {
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	backoff := time.Second

	for {
		select {
		case <-t.stop:
			return
		default:
		}

		conn, _, err := dialer.Dial(url, nil)
		if err != nil {
			logger.Debug("Progress websocket dial failed (%s): %v", url, err)
			select {
			case <-t.stop:
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		logger.Debug("Progress websocket connected to %s", url)
		backoff = time.Second
		t.readLoop(conn)
	}
}

// readLoop consumes messages until the connection errors or Close is called.
func (t *Tracker) readLoop(conn *websocket.Conn) {
	// Close the connection when stop fires so a blocked ReadMessage unblocks.
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-t.stop:
			_ = conn.Close()
		case <-closed:
		}
	}()

	defer func() { _ = conn.Close() }()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			logger.Debug("Progress websocket read ended: %v", err)
			return
		}
		t.handleMessage(msg)
	}
}

// handleMessage parses one websocket frame and updates the tracker state. Frames
// of other types, or malformed frames, are ignored.
func (t *Tracker) handleMessage(raw []byte) {
	var env wsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}

	switch env.Type {
	case "progress_updates":
		var d progressData
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return
		}
		if d.Subtype != "undecoded_transactions" {
			return
		}
		t.mu.Lock()
		t.decodeChain = d.Chain
		t.decodeProcessed = d.Processed
		t.decodeTotal = d.Total
		t.haveDecode = true
		t.mu.Unlock()

	case "transaction_status":
		var d txStatusData
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return
		}
		t.mu.Lock()
		t.statusChain = d.Chain
		t.statusStep = d.Status
		t.mu.Unlock()
	}
}
