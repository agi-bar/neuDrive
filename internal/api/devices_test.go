package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agi-bar/agenthub/internal/services"
)

func TestRespondDeviceCallError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "bad request", err: services.ErrDeviceInvalidRequest, want: http.StatusBadRequest},
		{name: "not found", err: services.ErrDeviceNotFound, want: http.StatusNotFound},
		{name: "unsupported", err: services.ErrDeviceUnsupported, want: http.StatusNotImplemented},
		{name: "upstream", err: services.ErrDeviceUpstreamFailed, want: http.StatusBadGateway},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			respondDeviceCallError(rec, tc.err)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
