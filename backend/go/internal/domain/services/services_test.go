package services

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeCollector struct {
	statuses  map[string]Service
	restart   error
	restarted string
}

func (f *fakeCollector) Status(_ context.Context, service string) Service {
	if status, exists := f.statuses[service]; exists {
		return status
	}
	return Service{ID: service, State: "not-found"}
}

func (f *fakeCollector) List(ctx context.Context) []Service {
	items := make([]Service, 0, len(defaultAllowedServices))
	for _, service := range defaultAllowedServices {
		items = append(items, f.Status(ctx, service))
	}
	return items
}

func (f *fakeCollector) Restart(_ context.Context, service string) error {
	f.restarted = service
	return f.restart
}

type runnerReply struct {
	output []byte
	err    error
}

type fixtureRunner struct {
	replies map[string]runnerReply
	calls   []string
}

func (f *fixtureRunner) Run(_ context.Context, arguments ...string) ([]byte, error) {
	key := strings.Join(arguments, " ")
	f.calls = append(f.calls, key)
	reply, exists := f.replies[key]
	if !exists {
		return nil, errors.New("unexpected systemctl call")
	}
	return reply.output, reply.err
}

func TestLinuxCollectorNormalizesSystemctlFixtures(t *testing.T) {
	const call = "show --property=Id --property=Names --property=LoadState --property=ActiveState --property=UnitFileState --property=SubState -- apache2.service"
	runner := &fixtureRunner{replies: map[string]runnerReply{
		call: {output: []byte("Id=apache2.service\nNames=apache2.service\nLoadState=loaded\nActiveState=active\nUnitFileState=disabled\nSubState=running\n")},
	}}
	collector := &LinuxCollector{runner: runner}
	status := collector.Status(context.Background(), "apache2")
	if status.ID != "apache2" || !status.Exists || !status.Active || status.Enabled || status.State != "running" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestLinuxCollectorTreatsMissingAndMaskedUnitsLikeBash(t *testing.T) {
	runner := &fixtureRunner{replies: map[string]runnerReply{
		"show --property=Id --property=Names --property=LoadState --property=ActiveState --property=UnitFileState --property=SubState -- mysql.service": {output: []byte("Id=mysql.service\nNames=mysql.service\nLoadState=not-found\n")},
		"show --property=Id --property=Names --property=LoadState --property=ActiveState --property=UnitFileState --property=SubState -- tor.service":   {output: []byte("Id=tor.service\nNames=tor.service\nLoadState=masked\nActiveState=inactive\nUnitFileState=masked\nSubState=\n")},
	}}
	collector := &LinuxCollector{runner: runner}
	missing := collector.Status(context.Background(), "mysql")
	if missing.Exists || missing.Active || missing.Enabled || missing.State != "not-found" {
		t.Fatalf("unexpected missing status: %#v", missing)
	}
	masked := collector.Status(context.Background(), "tor")
	if !masked.Exists || masked.Active || masked.Enabled || masked.State != "unknown" {
		t.Fatalf("unexpected masked status: %#v", masked)
	}
}

func TestLinuxCollectorListsAllServicesInOneCall(t *testing.T) {
	services := uniqueSorted(append(append([]string{}, defaultAllowedServices...), "php8.4-fpm", "php8.5-fpm"))
	units := make([]string, 0, len(services))
	var output strings.Builder
	for _, service := range services {
		units = append(units, service+".service")
		output.WriteString("Id=" + service + ".service\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\nSubState=running\n\n")
	}
	call := "show --property=Id --property=Names --property=LoadState --property=ActiveState --property=UnitFileState --property=SubState -- " + strings.Join(units, " ")
	runner := &fixtureRunner{replies: map[string]runnerReply{
		"list-unit-files --type=service --no-legend --no-pager php*-fpm.service": {output: []byte("php8.5-fpm.service enabled\nphp8.4-fpm.service disabled\ninvalid-php.service enabled\n")},
		call: {output: []byte(output.String())},
	}}
	items := (&LinuxCollector{runner: runner}).List(context.Background())
	if len(items) != len(services) || len(runner.calls) != 2 || !items[0].Active || !items[len(items)-1].Enabled {
		t.Fatalf("items=%#v calls=%#v", items, runner.calls)
	}
}

func TestLinuxCollectorRecognizesSystemdAlias(t *testing.T) {
	const call = "show --property=Id --property=Names --property=LoadState --property=ActiveState --property=UnitFileState --property=SubState -- mysql.service"
	runner := &fixtureRunner{replies: map[string]runnerReply{
		call: {output: []byte("Id=mariadb.service\nNames=mariadb.service mysql.service\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\nSubState=running\n")},
	}}
	status := (&LinuxCollector{runner: runner}).Status(context.Background(), "mysql")
	if !status.Exists || !status.Active || status.ID != "mysql" {
		t.Fatalf("status=%#v", status)
	}
}

func TestLinuxCollectorRestart(t *testing.T) {
	runner := &fixtureRunner{replies: map[string]runnerReply{
		"restart -- apache2.service": {output: []byte{}},
	}}
	collector := &LinuxCollector{runner: runner}
	if err := collector.Restart(context.Background(), "apache2"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "restart -- apache2.service" {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestHandlerListStatusAndRestart(t *testing.T) {
	collector := &fakeCollector{statuses: map[string]Service{
		"apache2": {ID: "apache2", Exists: true, Active: true, Enabled: true, State: "running"},
	}}
	handler := New(collector)

	list := handler.Handle(context.Background(), "list", nil)
	if list.ExitCode != 0 || list.Response.Data == nil {
		t.Fatalf("unexpected list: %#v", list)
	}
	items := (*list.Response.Data)["services"].([]Service)
	if len(items) != len(defaultAllowedServices) || items[0].ID != "apache2" || items[1].ID != "httpd" {
		t.Fatalf("unexpected service list: %#v", items)
	}

	status := handler.Handle(context.Background(), "status", []string{"apache2"})
	if status.ExitCode != 0 || status.Response.Data == nil {
		t.Fatalf("unexpected status: %#v", status)
	}
	service := (*status.Response.Data)["service"].(Service)
	if !service.Exists || service.State != "running" {
		t.Fatalf("unexpected service: %#v", service)
	}

	restart := handler.Handle(context.Background(), "restart", []string{"apache2"})
	if restart.ExitCode != 0 || collector.restarted != "apache2" || restart.Response.Data == nil || (*restart.Response.Data)["action"] != "restart" {
		t.Fatalf("unexpected restart: %#v", restart)
	}
}

func TestConfiguredServicesAndDynamicPHPPolicy(t *testing.T) {
	directory := t.TempDir()
	path := directory + "/services"
	if err := os.WriteFile(path, []byte("# Services\nsshd\nhttpd\nsshd\n../invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	services := configuredServices(path)
	if len(services) != 2 || services[0] != "httpd" || services[1] != "sshd" {
		t.Fatalf("services=%#v", services)
	}
	handler := New(&fakeCollector{})
	handler.allowed = func(service string) bool {
		return service == "httpd" || phpFPMUnitPattern.MatchString(service+".service")
	}
	if reply := handler.validateService("php8.4-fpm"); reply != nil {
		t.Fatalf("dynamic PHP service rejected: %#v", reply)
	}
	if reply := handler.validateService("nginx"); reply == nil || reply.Response.Error.Code != "SERVICE_NOT_ALLOWED" {
		t.Fatalf("unexpected nginx policy: %#v", reply)
	}
}

func TestHandlerErrors(t *testing.T) {
	tests := []struct {
		collector *fakeCollector
		command   string
		arguments []string
		exit      int
		code      string
	}{
		{&fakeCollector{}, "", nil, 2, "MISSING_COMMAND"},
		{&fakeCollector{}, "unknown", nil, 4, "COMMAND_NOT_FOUND"},
		{&fakeCollector{}, "list", []string{"extra"}, 2, "INVALID_ARGUMENT_COUNT"},
		{&fakeCollector{}, "status", nil, 2, "INVALID_ARGUMENT_COUNT"},
		{&fakeCollector{}, "status", []string{"../apache2"}, 2, "INVALID_SERVICE_IDENTIFIER"},
		{&fakeCollector{}, "status", []string{"nginx"}, 6, "SERVICE_NOT_ALLOWED"},
		{&fakeCollector{}, "restart", []string{"apache2"}, 5, "SERVICE_NOT_FOUND"},
		{&fakeCollector{statuses: map[string]Service{"apache2": {ID: "apache2", Exists: true, State: "running"}}, restart: errors.New("failed")}, "restart", []string{"apache2"}, 10, "SERVICE_RESTART_FAILED"},
	}
	for _, test := range tests {
		reply := New(test.collector).Handle(context.Background(), test.command, test.arguments)
		if reply.ExitCode != test.exit || reply.Response.Error == nil || reply.Response.Error.Code != test.code {
			t.Fatalf("unexpected reply for %q: %#v", test.command, reply)
		}
	}
}
