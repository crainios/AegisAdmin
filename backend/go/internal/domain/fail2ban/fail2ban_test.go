package fail2ban

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fake map[string]struct {
	o  string
	ok bool
}

func (f fake) Run(_ context.Context, n string, a ...string) (string, bool) {
	k := n
	for _, v := range a {
		k += " " + v
	}
	r := f[k]
	return r.o, r.ok
}
func TestParsers(t *testing.T) {
	b := &Backend{runner: fake{client + " status": {o: "Status\n|- Jail list: sshd, apache, sshd", ok: true}, client + " status sshd": {o: "Currently failed: 1\nTotal failed: 5\nCurrently banned: 2\nTotal banned: 3\nBanned IP list: 2001:db8::1 192.0.2.4", ok: true}}, client: client}
	j, ok := b.jails(context.Background())
	if !ok || len(j) != 2 || j[0] != "apache" {
		t.Fatalf("jails %#v", j)
	}
	s, ok := b.jailStatus(context.Background(), "sshd")
	if !ok || s["total_failed"] != int64(5) || len(s["banned_ips"].([]string)) != 2 {
		t.Fatalf("status %#v", s)
	}
}
func TestHandlerErrors(t *testing.T) {
	h := New(&Backend{})
	for _, x := range []struct {
		c    string
		a    []string
		code string
	}{{"", nil, "MISSING_COMMAND"}, {"unknown", nil, "COMMAND_NOT_FOUND"}, {"info", []string{"x"}, "INVALID_ARGUMENT_COUNT"}} {
		r := h.Handle(context.Background(), x.c, x.a)
		if r.Response.Error == nil || r.Response.Error.Code != x.code {
			t.Fatalf("%#v", r)
		}
	}
}

func TestLoadsPortableProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fail2ban")
	content := "client=/usr/local/bin/fail2ban-client\nunit=fail2ban-custom.service\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &Backend{client: client, unit: unit, service: service}
	b.loadProfile(path)
	if b.client != "/usr/local/bin/fail2ban-client" || b.unit != "fail2ban-custom.service" || b.service != "fail2ban-custom" {
		t.Fatalf("profile: %#v", b)
	}
}
