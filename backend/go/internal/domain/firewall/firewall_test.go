package firewall

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fake map[string]struct {
	o, e string
	ok   bool
}

func (f fake) Run(_ context.Context, n string, a ...string) (string, string, bool) {
	k := n
	for _, v := range a {
		k += " " + v
	}
	r := f[k]
	return r.o, r.e, r.ok
}
func TestParsers(t *testing.T) {
	v := parseVerbose("Status: active\nLogging: on (low)\nDefault: deny (incoming), allow (outgoing), deny (routed)")
	if v["active"] != true || v["default_incoming"] != "deny" {
		t.Fatalf("%#v", v)
	}
	r := parseRules("[ 1] 22/tcp                    ALLOW IN    Anywhere\n[ 2] 53/udp (v6)              DENY OUT    Anywhere (v6)")
	if len(r) != 2 || r[0].Ports[0] != "22" || r[1].Family != "ipv6" || r[1].Direction != "out" {
		t.Fatalf("%#v", r)
	}
}
func TestValidation(t *testing.T) {
	if e := validateAdd([]string{"allow", "22,8000:8010", "tcp", "192.0.2.0/24"}); e != nil {
		t.Fatalf("%#v", e)
	}
	if e := validateAdd([]string{"allow", "0", "tcp", "any"}); e == nil || e.Response.Error.Code != "INVALID_FIREWALL_RULE" {
		t.Fatalf("%#v", e)
	}
}
func TestUsageAndFingerprint(t *testing.T) {
	line := "[ 1] 22/tcp                    ALLOW IN    Anywhere"
	b := &Backend{runner: fake{"/usr/sbin/ufw status numbered": {o: line, ok: true}, "/usr/bin/ss -H -lntup": {o: `tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=1,fd=3))`, ok: true}}, ufw: "/usr/sbin/ufw"}
	u, e := b.usage(context.Background(), 1)
	if e != nil || u["in_use"] != true || u["ssh_access_risk"] != true || u["fingerprint"] != fingerprint(line) {
		t.Fatalf("%#v %#v", u, e)
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

func TestFirewalldParsingAndInfo(t *testing.T) {
	rich := `rule family="ipv4" source address="192.0.2.0/24" port port="22" protocol="tcp" accept`
	rule, ok := parseFirewalldRule(rich)
	if !ok || rule.Action != "allow" || rule.Source != "192.0.2.0/24" || rule.Ports[0] != "22" {
		t.Fatalf("rich rule: %#v", rule)
	}
	directory := t.TempDir()
	command := filepath.Join(directory, "firewall-cmd")
	if err := os.WriteFile(command, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	b := &Backend{runner: fake{
		command + " --state":           {o: "running", ok: true},
		command + " --list-rich-rules": {o: rich, ok: true},
		command + " --list-ports":      {o: "443/tcp", ok: true},
		command + " --version":         {o: "2.3.1", ok: true},
	}, ufw: filepath.Join(directory, "missing"), firewalld: command}
	reply := New(b).Handle(context.Background(), "info", nil)
	if !reply.Response.Success || (*reply.Response.Data)["backend"] != "firewalld" || (*reply.Response.Data)["active"] != true {
		t.Fatalf("firewalld info: %#v", reply)
	}
	rules := New(b).Handle(context.Background(), "rules", nil)
	if !rules.Response.Success || (*rules.Response.Data)["count"] != 2 {
		t.Fatalf("firewalld rules: %#v", rules)
	}
}
