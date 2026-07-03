package komari

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServerTimeUsesHTTPDateHeader(t *testing.T) {
	serverTime := time.Date(2026, 7, 4, 12, 29, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s, want HEAD", r.Method)
		}
		w.Header().Set("Date", serverTime.Format(http.TimeFormat))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClientWithOptions(server.URL, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ServerTime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(serverTime) {
		t.Fatalf("server time = %s, want %s", got, serverTime)
	}
}
