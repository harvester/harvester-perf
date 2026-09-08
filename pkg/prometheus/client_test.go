package prometheus

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
	mockHost := "https://mock"
	tests := []struct {
		name      string
		serverURL string // input to New()
		wantPath  string // expected path hit on server
	}{
		{
			name:      "default URL",
			serverURL: "",
			wantPath:  mockHost + prometheusSVCProxy,
		},
		{
			name:      "custom URL",
			serverURL: "http://localhost:9090",
			wantPath:  "http://localhost:9090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			inputURL := tt.serverURL
			restConfig := &rest.Config{Host: mockHost}

			_, err := New(inputURL, restConfig)
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
		})
	}
}
