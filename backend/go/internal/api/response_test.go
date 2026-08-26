package api

import (
	"encoding/json"
	"testing"
)

func TestSuccessEnvelope(t *testing.T) {
	payload, err := json.Marshal(Success(map[string]any{"value": "ok"}))
	if err != nil {
		t.Fatal(err)
	}

	const expected = `{"success":true,"api":1,"data":{"value":"ok"}}`
	if string(payload) != expected {
		t.Fatalf("unexpected success envelope: %s", payload)
	}
}

func TestEmptySuccessKeepsDataObject(t *testing.T) {
	payload, err := json.Marshal(Success(nil))
	if err != nil {
		t.Fatal(err)
	}

	const expected = `{"success":true,"api":1,"data":{}}`
	if string(payload) != expected {
		t.Fatalf("unexpected empty success envelope: %s", payload)
	}
}

func TestFailureEnvelope(t *testing.T) {
	payload, err := json.Marshal(Failure("ERROR_CODE", "Message"))
	if err != nil {
		t.Fatal(err)
	}

	const expected = `{"success":false,"api":1,"error":{"code":"ERROR_CODE","message":"Message"}}`
	if string(payload) != expected {
		t.Fatalf("unexpected failure envelope: %s", payload)
	}
}
