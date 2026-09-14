package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestNew_returnsAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	api, err := New(srv.URL, &rest.Config{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if api == nil {
		t.Fatal("New() returned nil API, want non-nil")
	}
}

func TestNew_invalidURL(t *testing.T) {
	_, err := New("://bad-url", &rest.Config{})
	if err == nil {
		t.Fatal("New() with invalid URL expected an error, got nil")
	}
}

func TestNew_URL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL func(string) string
		wantPath  string
	}{
		{
			name:      "service proxy by default",
			serverURL: func(string) string { return "" },
			wantPath:  prometheusSVCProxy + "/api/v1/query",
		},
		{
			name:      "custom URL",
			serverURL: func(host string) string { return host },
			wantPath:  "/api/v1/query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
			}))
			defer srv.Close()

			api, err := New(tt.serverURL(srv.URL), &rest.Config{Host: srv.URL})
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
			if _, _, err := api.Query(context.Background(), "up", time.Now()); err != nil {
				t.Fatalf("Query() error = %v, want nil", err)
			}
			if gotPath != tt.wantPath {
				t.Errorf("request path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}
