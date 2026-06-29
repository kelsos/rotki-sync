package process

import (
	"slices"
	"testing"
)

func TestFilterChildEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HOME=/home/kelsos",
		"ROTKI_SYNC_AGE_KEY=AGE-SECRET-KEY-xxx",
		"ALICE_PASSWORD=hunter2",
		"ROTKI_PORT=59001",
		"LANG=en_US.UTF-8",
	}
	got := filterChildEnv(in)

	want := []string{
		"PATH=/usr/bin",
		"HOME=/home/kelsos",
		"ROTKI_PORT=59001",
		"LANG=en_US.UTF-8",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("filterChildEnv mismatch\n got: %v\nwant: %v", got, want)
	}

	for _, kv := range got {
		if kv == "ROTKI_SYNC_AGE_KEY=AGE-SECRET-KEY-xxx" || kv == "ALICE_PASSWORD=hunter2" {
			t.Errorf("secret leaked into child env: %q", kv)
		}
	}
}
