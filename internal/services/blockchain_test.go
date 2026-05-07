package services

import (
	"testing"
	"time"

	"github.com/kelsos/rotki-sync/internal/models"
)

func TestShouldSkipTokenDetection(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	maxAge := 5 * 24 * time.Hour

	tests := []struct {
		name     string
		info     models.TokenDetectAddressInfo
		wantSkip bool
		wantAge  time.Duration
	}{
		{
			name:     "no cached entry",
			info:     models.TokenDetectAddressInfo{},
			wantSkip: false,
			wantAge:  0,
		},
		{
			name:     "negative timestamp treated as missing",
			info:     models.TokenDetectAddressInfo{LastUpdateTimestamp: -1},
			wantSkip: false,
			wantAge:  0,
		},
		{
			name:     "fresh detection one hour ago is skipped",
			info:     models.TokenDetectAddressInfo{LastUpdateTimestamp: now.Add(-time.Hour).Unix()},
			wantSkip: true,
			wantAge:  time.Hour,
		},
		{
			name:     "just under threshold is skipped",
			info:     models.TokenDetectAddressInfo{LastUpdateTimestamp: now.Add(-maxAge + time.Second).Unix()},
			wantSkip: true,
		},
		{
			name:     "exactly at threshold is not skipped",
			info:     models.TokenDetectAddressInfo{LastUpdateTimestamp: now.Add(-maxAge).Unix()},
			wantSkip: false,
		},
		{
			name:     "older than threshold is not skipped",
			info:     models.TokenDetectAddressInfo{LastUpdateTimestamp: now.Add(-10 * 24 * time.Hour).Unix()},
			wantSkip: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSkip, gotAge := shouldSkipTokenDetection(tc.info, now, maxAge)
			if gotSkip != tc.wantSkip {
				t.Errorf("skip = %v, want %v", gotSkip, tc.wantSkip)
			}
			if tc.wantAge != 0 && gotAge != tc.wantAge {
				t.Errorf("age = %v, want %v", gotAge, tc.wantAge)
			}
		})
	}
}
