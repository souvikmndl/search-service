package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTransformToUserResponseMapsFields(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	user := User{
		ID:        42,
		EmailID:   "user@example.com",
		UserName:  "tester",
		Password:  "supersecret",
		CreatedAt: now,
	}

	resp := TransformToUserResponse(user)

	if resp.ID != 42 {
		t.Fatalf("ID: expected 42, got %d", resp.ID)
	}
	if resp.EmailID != "user@example.com" {
		t.Fatalf("EmailID: expected user@example.com, got %s", resp.EmailID)
	}
	if resp.UserName != "tester" {
		t.Fatalf("UserName: expected tester, got %s", resp.UserName)
	}
	if resp.CreatedAt != now.Format(time.RFC3339) {
		t.Fatalf("CreatedAt: expected %s, got %s", now.Format(time.RFC3339), resp.CreatedAt)
	}
}

func TestTransformToUserResponseOmitsPassword(t *testing.T) {
	user := User{
		ID:        1,
		EmailID:   "user@example.com",
		UserName:  "tester",
		Password:  "topsecret",
		CreatedAt: time.Now(),
	}

	resp := TransformToUserResponse(user)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["password"]; ok {
		t.Fatalf("response must not contain a password field")
	}
	if _, ok := m["Password"]; ok {
		t.Fatalf("response must not contain a Password field")
	}
}
