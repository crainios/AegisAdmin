package network

import (
	"context"
	"errors"
	"os"
	"testing"
)

type fakeCollector struct {
	interfaces []Interface
	err        error
}

func (f fakeCollector) Interfaces(context.Context) ([]Interface, error) { return f.interfaces, f.err }

func TestNormalizeIPJSONFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/ip-address.json")
	if err != nil {
		t.Fatal(err)
	}
	interfaces, err := normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(interfaces) != 3 || interfaces[0].ID != "br0" || interfaces[1].ID != "eth0" || interfaces[2].ID != "lo" {
		t.Fatalf("unexpected interface order: %#v", interfaces)
	}
	eth0 := interfaces[1]
	if eth0.Type != "ethernet" || eth0.State != "up" || eth0.MAC == nil || *eth0.MAC != "02:42:ac:11:00:02" || eth0.MTU == nil || *eth0.MTU != 1500 {
		t.Fatalf("unexpected eth0 normalization: %#v", eth0)
	}
	if len(eth0.IPv4) != 2 || eth0.IPv4[0].Address != "192.0.2.10" || len(eth0.IPv6) != 1 {
		t.Fatalf("unexpected addresses: %#v", eth0)
	}
	if interfaces[0].Type != "bridge" || interfaces[0].State != "down" || interfaces[2].Type != "loopback" || interfaces[2].State != "unknown" {
		t.Fatalf("unexpected type or state normalization: %#v", interfaces)
	}
}

func TestNormalizeIPTypesFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/ip-types.json")
	if err != nil {
		t.Fatal(err)
	}
	interfaces, err := normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]string{}
	for _, item := range interfaces {
		types[item.ID] = item.Type
	}
	for name, expected := range map[string]string{"bond0": "bond", "vlan10": "vlan", "wg0": "wireguard", "tun0": "tun", "tap0": "tap"} {
		if types[name] != expected {
			t.Fatalf("unexpected type for %s: %q", name, types[name])
		}
	}
}

func TestNormalizeRejectsInvalidJSON(t *testing.T) {
	if _, err := normalize([]byte(`{"not":"an array"}`)); err == nil {
		t.Fatal("an object payload must be rejected")
	}
	if _, err := normalize([]byte(`null`)); err == nil {
		t.Fatal("a null payload must be rejected")
	}
}

func TestHandlerListAndStatusContracts(t *testing.T) {
	mtu := 1500
	mac := "00:11:22:33:44:55"
	handler := New(fakeCollector{interfaces: []Interface{{ID: "eth0", Name: "eth0", Type: "ethernet", State: "up", MAC: &mac, MTU: &mtu, IPv4: []Address{}, IPv6: []Address{}}}})

	list := handler.Handle(context.Background(), "list", nil)
	if list.ExitCode != 0 || !list.Response.Success || list.Response.Data == nil {
		t.Fatalf("unexpected list reply: %#v", list)
	}
	items := (*list.Response.Data)["interfaces"].([]map[string]any)
	if len(items) != 1 || len(items[0]) != 4 || items[0]["id"] != "eth0" {
		t.Fatalf("unexpected list contract: %#v", items)
	}

	status := handler.Handle(context.Background(), "status", []string{"eth0"})
	if status.ExitCode != 0 || status.Response.Data == nil || (*status.Response.Data)["mac"] != &mac || (*status.Response.Data)["mtu"] != &mtu {
		t.Fatalf("unexpected status reply: %#v", status)
	}
}

func TestHandlerErrors(t *testing.T) {
	tests := []struct {
		handler   *Handler
		command   string
		arguments []string
		exit      int
		code      string
	}{
		{New(fakeCollector{}), "", nil, 2, "MISSING_COMMAND"},
		{New(fakeCollector{}), "unknown", nil, 4, "COMMAND_NOT_FOUND"},
		{New(fakeCollector{}), "list", []string{"extra"}, 2, "INVALID_ARGUMENT_COUNT"},
		{New(fakeCollector{}), "status", nil, 2, "INVALID_ARGUMENT_COUNT"},
		{New(fakeCollector{}), "status", []string{"../eth0"}, 2, "INVALID_NETWORK_IDENTIFIER"},
		{New(fakeCollector{}), "status", []string{"eth0"}, 5, "NETWORK_INTERFACE_NOT_FOUND"},
		{New(fakeCollector{err: errors.New("failed")}), "list", nil, 3, "NETWORK_READ_FAILED"},
		{New(fakeCollector{err: errors.New("failed")}), "status", []string{"eth0"}, 3, "NETWORK_READ_FAILED"},
		{New(fakeCollector{err: &collectorError{code: "INVALID_NETWORK_DATA", message: "invalid"}}), "list", nil, 3, "INVALID_NETWORK_DATA"},
	}
	for _, test := range tests {
		reply := test.handler.Handle(context.Background(), test.command, test.arguments)
		if reply.ExitCode != test.exit || reply.Response.Error == nil || reply.Response.Error.Code != test.code {
			t.Fatalf("unexpected reply for %q: %#v", test.command, reply)
		}
	}
}
