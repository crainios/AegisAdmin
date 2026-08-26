package tor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeRunner map[string]struct {
	o  string
	ok bool
}

func (f fakeRunner) Run(_ context.Context, n string, a ...string) (string, bool) {
	r := f[n+" "+join(a)]
	return r.o, r.ok
}
func join(a []string) string {
	r := ""
	for i, v := range a {
		if i > 0 {
			r += " "
		}
		r += v
	}
	return r
}
func TestHiddenServicesFixture(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join("/var/lib/tor", "aegisadmin-test")
	config := filepath.Join(root, "torrc")
	if err := os.WriteFile(config, []byte("HiddenServiceDir "+dir+"\nHiddenServicePort 80 127.0.0.1:8080\n"), 0600); err != nil {
		t.Fatal(err)
	}
	items, ok := hidden(config, "/var/lib/tor")
	if !ok || len(items) != 1 || items[0].ID != "test" || len(items[0].Ports) != 1 {
		t.Fatalf("items: %#v", items)
	}
}
func TestHandlerValidation(t *testing.T) {
	h := New(&Backend{})
	for _, x := range []struct {
		c    string
		a    []string
		code string
	}{{"", nil, "MISSING_COMMAND"}, {"unknown", nil, "COMMAND_NOT_FOUND"}, {"info", []string{"x"}, "INVALID_ARGUMENT_COUNT"}} {
		r := h.Handle(context.Background(), x.c, x.a)
		if r.Response.Error == nil || r.Response.Error.Code != x.code {
			t.Fatalf("reply: %#v", r)
		}
	}
}

func TestLoadsPortableProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tor-profile")
	content := "binary=/usr/bin/tor\nconfig=/usr/local/etc/tor/torrc\ndefaults=\nunit=tor.service\ndata_directory=/var/lib/tor-custom\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &Backend{binary: binary, config: config, defaults: defaults, unit: unit, service: service, dataDirectory: "/var/lib/tor"}
	b.loadProfile(path)
	if b.binary != "/usr/bin/tor" || b.config != "/usr/local/etc/tor/torrc" || b.defaults != "" || b.unit != "tor.service" || b.service != "tor" || b.dataDirectory != "/var/lib/tor-custom" {
		t.Fatalf("profile: %#v", b)
	}
}
