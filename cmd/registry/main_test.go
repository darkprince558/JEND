package main

import (
	"encoding/json"
	"testing"
)

func TestRegistryItemJSON(t *testing.T) {
	// Simple test to ensure struct tags are correct
	item := RegistryItem{
		Code: "test-code",
		IP:   "127.0.0.1",
		Port: 8080,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Failed to marshal item: %v", err)
	}

	expected := `{"code":"test-code","ip":"127.0.0.1","port":8080,"expires_at":0}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}
}
