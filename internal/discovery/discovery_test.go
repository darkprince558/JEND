package discovery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupCloud(t *testing.T) {
	// 1. Mock Server
	mockItem := RegistryItem{
		Code: "test-code-123",
		IP:   "192.168.1.100",
		Port: 9000,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lookup/test-code-123" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockItem) //nolint:errcheck
			return
		}
		if r.URL.Path == "/lookup/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	// 2. Client with Mock Endpoint
	client := NewRegistryClient()
	client.Endpoint = server.URL

	// 3. Test Success
	item, err := client.Lookup("test-code-123")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if item.IP != mockItem.IP {
		t.Errorf("Expected IP %s, got %s", mockItem.IP, item.IP)
	}

	// 4. Test Not Found
	_, err = client.Lookup("missing")
	if err == nil {
		t.Error("Expected error for missing code, got nil")
	}
}

func TestRegisterCloud(t *testing.T) {
	// 1. Mock Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/register" && r.Method == "POST" {
			var item RegistryItem
			if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if item.Code == "bad-code" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("database error"))
				return
			}
			w.WriteHeader(http.StatusOK) // or Created
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// 2. Client
	client := NewRegistryClient()
	client.Endpoint = server.URL

	// 3. Test Success
	err := client.Register("good-code", "1.2.3.4", 8080, nil)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	// 4. Test Failure
	err = client.Register("bad-code", "1.2.3.4", 8080, nil)
	if err == nil {
		t.Error("Expected error for bad code, got nil")
	}
}
