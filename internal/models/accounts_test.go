package models

import (
	"encoding/json"
	"testing"
)

func TestAccountListUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantAddrs []string
	}{
		{
			name:      "array form (evm and most chains)",
			payload:   `[{"address":"0xabc","label":"a"},{"address":"0xdef"}]`,
			wantAddrs: []string{"0xabc", "0xdef"},
		},
		{
			name:      "bitcoin object with standalone only",
			payload:   `{"standalone":[{"address":"bc1qstandalone","label":"x","tags":null}],"xpubs":[]}`,
			wantAddrs: []string{"bc1qstandalone"},
		},
		{
			name: "bitcoin object with standalone and xpub-derived addresses",
			payload: `{"standalone":[{"address":"bc1qstandalone"}],` +
				`"xpubs":[{"xpub":"xpub1","derivation_path":null,` +
				`"addresses":[{"address":"bc1qderived1"},{"address":"bc1qderived2"}]}]}`,
			wantAddrs: []string{"bc1qstandalone", "bc1qderived1", "bc1qderived2"},
		},
		{
			name:      "bitcoin xpub with no derived addresses (null)",
			payload:   `{"standalone":[],"xpubs":[{"xpub":"xpub1","addresses":null}]}`,
			wantAddrs: nil,
		},
		{
			name:      "null payload",
			payload:   `null`,
			wantAddrs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var list AccountList
			if err := json.Unmarshal([]byte(tt.payload), &list); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if len(list) != len(tt.wantAddrs) {
				t.Fatalf("got %d accounts, want %d (%v)", len(list), len(tt.wantAddrs), list)
			}
			for i, addr := range tt.wantAddrs {
				if list[i].Address != addr {
					t.Errorf("account %d: got %q, want %q", i, list[i].Address, addr)
				}
			}
		})
	}
}

func TestAccountListUnmarshalRejectsScalar(t *testing.T) {
	var list AccountList
	if err := json.Unmarshal([]byte(`42`), &list); err == nil {
		t.Fatal("expected error for non-array, non-object payload")
	}
}

func TestTransactionDecodeResultIsScalarCount(t *testing.T) {
	// The unified per-chain decode returns decoded_tx_number as a plain integer,
	// not the legacy per-chain map the removed evm decode endpoint returned.
	var res TransactionDecodeResult
	if err := json.Unmarshal([]byte(`{"decoded_tx_number": 7}`), &res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if res.DecodedTxNumber != 7 {
		t.Errorf("got %d, want 7", res.DecodedTxNumber)
	}
}
