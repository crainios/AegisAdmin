package adminaccess

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := []settings{{true, 8443, "*", "all"}, {true, 9443, "127.0.0.1\n10.0.0.1", "192.168.1.0/24"}, {false, 8443, "::1,2001:db8::10", "2001:db8::/32"}}
	for _, item := range valid {
		if err := validate(item); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	invalid := []settings{{true, 80, "*", "all"}, {true, 8443, "example.com", "all"}, {true, 8443, "*", "network"}, {true, 8443, "*\n127.0.0.1", "all"}, {true, 8443, "127.0.0.1,127.0.0.1", "all"}}
	for _, item := range invalid {
		if validate(item) == nil {
			t.Fatalf("expected rejection: %#v", item)
		}
	}
}

func TestRenderUsesManagedValues(t *testing.T) {
	result := render([]string{"192.0.2.10:9443", "[2001:db8::10]:9443"}, "/var/www/Aegisadmin", "Require ip 10.0.0.0/8")
	for _, wanted := range []string{"Listen 192.0.2.10:9443\nListen [2001:db8::10]:9443", "<VirtualHost 192.0.2.10:9443 [2001:db8::10]:9443>", "Require ip 10.0.0.0/8", "ProxyPass https://127.0.0.1:9080/"} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("missing %q", wanted)
		}
	}
}

func TestDirectListenAddresses(t *testing.T) {
	listeners := directListenAddresses([]string{"192.0.2.10", "2001:db8::10"}, 8443, true)
	if strings.Join(listeners, ",") != "192.0.2.10:8443,[2001:db8::10]:8443" {
		t.Fatalf("listeners=%#v", listeners)
	}
	disabled := directListenAddresses([]string{"*"}, 8443, false)
	if len(disabled) != 1 || disabled[0] != "127.0.0.1:9080" {
		t.Fatalf("disabled listeners=%#v", disabled)
	}
}
