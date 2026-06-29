package progress

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRateLimitSource(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "externalapi coingecko",
			line: "[25/06/2026 16:18:34 CEST] WARNING rotkehlchen.externalapis.coingecko Greenlet-1: Got rate limited by coingecko querying https",
			want: "coingecko",
		},
		{
			name: "externalapi blockscout",
			line: "[25/06/2026 16:18:34 CEST] DEBUG rotkehlchen.externalapis.blockscout Greenlet-1: Blockscout API request https://api.blockscout.com/10/api got rate limited",
			want: "blockscout",
		},
		{
			name: "externalapi hyperliquid debug",
			line: "[25/06/2026 16:18:34 CEST] DEBUG rotkehlchen.externalapis.hyperliquid Greenlet-1: Hyperliquid API request got rate limited. Sleeping for 5 seconds.",
			want: "hyperliquid",
		},
		{
			name: "non-externalapi module falls back",
			line: "[25/06/2026 16:18:34 CEST] WARNING rotkehlchen.inquirer Greenlet-1: oracle failed due to: Got rate limited by coingecko",
			want: "a remote source",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rateLimitSource(tc.line); got != tc.want {
				t.Fatalf("rateLimitSource = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCauseFromLogChunk(t *testing.T) {
	t.Run("no rate-limit lines", func(t *testing.T) {
		chunk := []byte("[..] DEBUG web3 Greenlet-1: ordinary line\n[..] INFO another line\n")
		if got := causeFromLogChunk(chunk); got != "" {
			t.Fatalf("expected empty cause, got %q", got)
		}
	})

	t.Run("returns last marker source", func(t *testing.T) {
		chunk := []byte(
			"[..] WARNING rotkehlchen.externalapis.coingecko Greenlet-1: Got rate limited by coingecko\n" +
				"[..] DEBUG web3 Greenlet-1: unrelated\n" +
				"[..] DEBUG rotkehlchen.externalapis.blockscout Greenlet-1: request got rate limited\n",
		)
		if got := causeFromLogChunk(chunk); got != "blockscout" {
			t.Fatalf("expected last source blockscout, got %q", got)
		}
	})
}

// wantDecode is the decode annotation produced by the standard test message.
const wantDecode = "decoding ethereum 250/1000"

func TestSnapshotDecodeProgress(t *testing.T) {
	tr := NewTracker("")

	// progress_updates / undecoded_transactions -> decode annotation
	tr.handleMessage([]byte(`{"type":"progress_updates","data":{"chain":"ethereum","subtype":"undecoded_transactions","total":1000,"processed":250}}`))
	if got := tr.Snapshot(); got != wantDecode {
		t.Fatalf("Snapshot = %q, want %q", got, wantDecode)
	}
}

func TestSnapshotTransactionStatusFallback(t *testing.T) {
	tr := NewTracker("")

	// transaction_status before any decode progress -> humanized status
	tr.handleMessage([]byte(`{"type":"transaction_status","data":{"chain":"optimism","subtype":"evm","status":"querying_transactions_started"}}`))
	if got, want := tr.Snapshot(), "optimism: querying transactions started"; got != want {
		t.Fatalf("Snapshot = %q, want %q", got, want)
	}
}

func TestSnapshotIgnoresUnrelatedMessages(t *testing.T) {
	tr := NewTracker("")

	// other progress subtype must not produce a decode annotation
	tr.handleMessage([]byte(`{"type":"progress_updates","data":{"chain":"ethereum","subtype":"protocol_cache_updates","total":5,"processed":2}}`))
	tr.handleMessage([]byte(`{"type":"some_other_type","data":{}}`))
	if got := tr.Snapshot(); got != "" {
		t.Fatalf("Snapshot = %q, want empty", got)
	}
}

func TestSnapshotCombinesWsAndLogCause(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "rotki-core.log")
	if err := os.WriteFile(logPath, []byte("startup line\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tr := NewTracker(logPath)
	tr.handleMessage([]byte(`{"type":"progress_updates","data":{"chain":"ethereum","subtype":"undecoded_transactions","total":1000,"processed":250}}`))

	// First snapshot establishes the log offset (markers before "now" ignored).
	if got := tr.Snapshot(); got != wantDecode {
		t.Fatalf("first Snapshot = %q, want %q", got, wantDecode)
	}

	// A rate-limit line appended after the offset is reported as the cause.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("[..] WARNING rotkehlchen.externalapis.coingecko Greenlet-1: Got rate limited by coingecko\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	if got, want := tr.Snapshot(), wantDecode+" — coingecko rate-limited"; got != want {
		t.Fatalf("second Snapshot = %q, want %q", got, want)
	}

	// With no new rate-limit lines, the cause clears (not stale).
	if got := tr.Snapshot(); got != wantDecode {
		t.Fatalf("third Snapshot = %q, want %q", got, wantDecode)
	}
}

func TestCloseIsSafeWithoutWebsocket(t *testing.T) {
	tr := NewTracker("")
	tr.Close()
	tr.Close() // idempotent
}
