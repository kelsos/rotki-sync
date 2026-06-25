package models

import (
	"encoding/json"
	"testing"
)

func TestTaskOutcomeErr(t *testing.T) {
	tests := []struct {
		name    string
		outcome string
		wantErr bool
	}{
		{
			name:    "successful result with 200 status",
			outcome: `{"result": {"foo": "bar"}, "message": "", "status_code": 200}`,
			wantErr: false,
		},
		{
			name:    "successful boolean result no status",
			outcome: `{"result": true, "message": ""}`,
			wantErr: false,
		},
		{
			name:    "non-2xx status is a failure",
			outcome: `{"result": null, "message": "boom", "status_code": 500}`,
			wantErr: true,
		},
		{
			name:    "404 status is a failure",
			outcome: `{"result": null, "message": "not found", "status_code": 404}`,
			wantErr: true,
		},
		{
			name:    "result false with message is a failure",
			outcome: `{"result": false, "message": "something went wrong"}`,
			wantErr: true,
		},
		{
			name:    "result false without message is not flagged",
			outcome: `{"result": false, "message": ""}`,
			wantErr: false,
		},
		{
			name:    "missing status_code treated as not reported",
			outcome: `{"result": {"ok": 1}, "message": ""}`,
			wantErr: false,
		},
		{
			name:    "201 created is success",
			outcome: `{"result": {"id": 1}, "message": "", "status_code": 201}`,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var outcome TaskOutcome
			if err := json.Unmarshal([]byte(tc.outcome), &outcome); err != nil {
				t.Fatalf("failed to unmarshal outcome: %v", err)
			}
			err := outcome.Err()
			if (err != nil) != tc.wantErr {
				t.Errorf("Err() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
