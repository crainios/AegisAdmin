package webfail2ban

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"context"
	"testing"
)

func TestSnapshotAndActions(t *testing.T) {
	b := &fakeBackend{}
	s, e := New(b).Fail2banSnapshot(context.Background(), "sshd")
	if e != nil {
		t.Fatal(e)
	}
	if !s.Installed || s.Info.Version != "1.1" || s.Selected != "sshd" || s.Jail == nil || len(s.Jail.BannedIPs) != 1 {
		t.Fatalf("snapshot=%#v", s)
	}
	if e = New(b).Action(context.Background(), "unban", "sshd", "192.0.2.4"); e != nil {
		t.Fatal(e)
	}
	if b.requests[4].Arguments[1] != "192.0.2.4" {
		t.Fatalf("requests=%#v", b.requests)
	}
}

func TestSnapshotReportsFail2banNotInstalled(t *testing.T) {
	snapshot, err := New(notInstalledBackend{}).Fail2banSnapshot(context.Background(), "")
	if err != nil || snapshot.Installed || len(snapshot.Status.Jails) != 0 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
}

type notInstalledBackend struct{}

func (notInstalledBackend) Execute(protocol.Request) (protocol.Reply, error) {
	return protocol.Reply{ExitCode: 5, Response: api.Failure("FAIL2BAN_NOT_INSTALLED", "Fail2ban n’est pas installé sur ce serveur.")}, nil
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(r protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, r)
	d := map[string]any{}
	switch r.Command {
	case "info":
		d = map[string]any{"product": "Fail2ban", "version": "1.1", "config_directory": "/etc/fail2ban"}
	case "status":
		d = map[string]any{"exists": true, "active": true, "enabled": true, "state": "running", "jails": []any{"sshd"}}
	case "configtest":
		d = map[string]any{"valid": true, "message": "Configuration valide"}
	case "jail":
		d = map[string]any{"jail": "sshd", "status": map[string]any{"currently_failed": 0, "total_failed": 3, "currently_banned": 1, "total_banned": 2, "banned_ips": []any{"192.0.2.4"}}}
	}
	return protocol.Reply{Response: api.Success(d)}, nil
}
