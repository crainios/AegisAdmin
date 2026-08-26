package webfirewall

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"context"
	"strings"
	"testing"
)

func TestSnapshotAndDeleteCheck(t *testing.T) {
	b := &fakeBackend{}
	s, e := New(b).FirewallSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if s.Info.Product != "UFW" || len(s.Rules) != 1 {
		t.Fatalf("snapshot=%#v", s)
	}
	if e = New(b).Delete(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	if b.requests[3].Command != "delete" || b.requests[3].Arguments[2] != "yes" {
		t.Fatalf("requests=%#v", b.requests)
	}
	if e = New(b).SetEnabled(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if b.requests[4].Command != "disable" {
		t.Fatalf("requests=%#v", b.requests)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(r protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, r)
	d := map[string]any{}
	switch r.Command {
	case "info":
		d = map[string]any{"backend": "ufw", "product": "UFW", "installed": true, "version": "0.36", "active": true, "ipv6": true, "default_incoming": "deny", "default_outgoing": "allow", "default_routed": "deny"}
	case "rules":
		d = map[string]any{"rules": []any{map[string]any{"id": 1, "action": "allow", "direction": "in", "protocol": "tcp", "ports": []any{"22"}, "source": "any", "destination": "any", "family": "ipv4"}}}
	case "delete-check":
		d = map[string]any{"rule_id": 1, "fingerprint": strings.Repeat("a", 64)}
	}
	return protocol.Reply{Response: api.Success(d)}, nil
}
