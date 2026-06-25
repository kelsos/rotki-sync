package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kelsos/rotki-sync/internal/config"
)

func TestIsEndpointMissing(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "404 status",
			err:  &HTTPError{StatusCode: http.StatusNotFound, Body: "anything"},
			want: true,
		},
		{
			name: "url not found body without 404 code",
			err:  &HTTPError{StatusCode: 400, Body: "The requested URL was not found on the server."},
			want: true,
		},
		{
			name: "wrapped 404",
			err:  fmt.Errorf("failed to initiate async request: %w", &HTTPError{StatusCode: http.StatusNotFound, Body: "x"}),
			want: true,
		},
		{
			name: "500 is not a missing endpoint",
			err:  &HTTPError{StatusCode: 500, Body: "internal error"},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsEndpointMissing(tc.err); got != tc.want {
				t.Errorf("IsEndpointMissing() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEndpointExists(t *testing.T) {
	// A backend that registers everything except /api/1/blockchains/gone.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/1/blockchains/gone" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Allow", "OPTIONS, POST")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewAPIClient(&config.Config{BaseURL: server.URL})

	exists, err := c.EndpointExists("/blockchains/transactions")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected registered endpoint to exist")
	}

	exists, err = c.EndpointExists("/blockchains/gone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected removed endpoint to be reported missing")
	}
}
