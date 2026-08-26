package webnetwork

import (
	"context"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestNetworkSnapshotSelectsRequestedInterface(t *testing.T) {
	backend := &fakeBackend{}
	snapshot, err := New(backend).NetworkSnapshot(context.Background(), "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.Total != 2 || snapshot.Summary.Up != 2 || snapshot.Selected == nil || snapshot.Selected.ID != "eth0" || len(snapshot.Selected.IPv4) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if len(backend.requests) != 2 || backend.requests[1].Arguments[0] != "eth0" {
		t.Fatalf("unexpected requests: %#v", backend.requests)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, request)
	if request.Command == "list" {
		return protocol.Reply{Response: api.Success(map[string]any{"interfaces": []any{
			map[string]any{"id": "lo", "name": "lo", "type": "loopback", "state": "up"},
			map[string]any{"id": "eth0", "name": "eth0", "type": "ethernet", "state": "up"},
		}})}, nil
	}
	mac, mtu := "00:11:22:33:44:55", 1500
	return protocol.Reply{Response: api.Success(map[string]any{"id": "eth0", "name": "eth0", "type": "ethernet", "state": "up", "mac": mac, "mtu": mtu, "ipv4": []any{map[string]any{"address": "192.0.2.2", "prefix": 24}}, "ipv6": []any{}})}, nil
}
