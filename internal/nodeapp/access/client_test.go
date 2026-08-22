package access

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAccessClientDecodesOfflineEventWithoutExpiresAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"` + request.ID + `","result":{"access_instance_id":"access-01","session_epoch":"11111111-1111-4111-8111-111111111111","latest_sequence":1,"events":[{"event_id":"access-01:1","sequence":1,"access_instance_id":"access-01","session_epoch":"11111111-1111-4111-8111-111111111111","type":"registration_changed","occurred_at":"2026-08-18T15:00:00Z","device_access_id":"34020000001320000001","payload":{"state":"offline","reason":"unregister","remote_address":"192.0.2.1:5060","last_seen":"2026-08-18T15:00:00Z"}}]}}`))
	}))
	defer server.Close()

	result, err := NewAccessClient(server.URL, time.Second).PollEvents(context.Background(), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || !result.Events[0].Payload.ExpiresAt.IsZero() {
		t.Fatalf("offline event did not decode as an absent optional expiry: %#v", result.Events)
	}
}

func TestAccessRPCClientApply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.JSONRPC != "2.0" || request.Method != "access.v1.applyDeviceProfile" || request.ID == "" {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"status": "applied", "version": 3}})
	}))
	defer server.Close()
	client := NewAccessClient(server.URL, time.Second)
	result, err := client.ApplyDeviceProfile(context.Background(), AccessProfile{DeviceAccessID: "34020000001320000001", Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || result.Version != 3 {
		t.Fatalf("result = %+v", result)
	}
}
