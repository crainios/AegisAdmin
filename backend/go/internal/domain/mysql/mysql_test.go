package mysql

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct{ responses map[string]result }

func (f fakeRunner) Run(_ context.Context, name string, args ...string) (result, *Error) {
	key := name + " " + strings.Join(args, " ")
	if r, ok := f.responses[key]; ok {
		return r, nil
	}
	return result{}, errOf(10, "TEST_UNEXPECTED", key, nil)
}

func TestQueriesAndNormalization(t *testing.T) {
	client := filepath.Join(t.TempDir(), "mysql")
	if err := os.WriteFile(client, []byte("fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	responses := map[string]result{}
	queryKey := func(sql string) string {
		return client + " --batch --raw --skip-column-names --protocol=socket --connect-timeout=5 --execute " + sql
	}
	responses[queryKey(infoSQL)] = result{stdout: "11.8.2-MariaDB\tDebian MariaDB\tdbhost\t3306\t/run/mysqld/mysqld.sock\t/var/lib/mysql/\tInnoDB", ok: true}
	statusRows := []string{}
	for _, name := range statusVariables {
		statusRows = append(statusRows, name+"\t1")
	}
	responses[queryKey(inSQL("SHOW GLOBAL STATUS WHERE Variable_name IN", statusVariables))] = result{stdout: strings.Join(statusRows, "\n"), ok: true}
	responses[queryKey(databasesSQL)] = result{stdout: "information_schema\t0\t10\napplication\t2048\t3", ok: true}
	responses[queryKey(inSQL("SHOW GLOBAL VARIABLES WHERE Variable_name IN", configurationVariables))] = result{stdout: "character_set_server\tutf8mb4\nmax_connections\t151", ok: true}
	for _, unit := range []string{"mysql.service", "mariadb.service", "mysqld.service"} {
		for _, property := range []string{"LoadState", "ActiveState", "UnitFileState", "SubState"} {
			value := ""
			if unit == "mariadb.service" {
				value = map[string]string{"LoadState": "loaded", "ActiveState": "active", "UnitFileState": "enabled", "SubState": "running"}[property]
			}
			responses["/usr/bin/systemctl show --property="+property+" --value -- "+unit] = result{stdout: value, ok: true}
		}
	}
	responses["/usr/bin/systemd-run --quiet --collect --on-active=5s /usr/bin/systemctl restart -- mariadb.service"] = result{ok: true}
	b := &Backend{runner: fakeRunner{responses}, clients: []string{client}}
	info, e := b.info(context.Background())
	if e != nil || info["product"] != "MariaDB" || info["port"] != int64(3306) {
		t.Fatalf("info: %#v %#v", info, e)
	}
	status, e := b.status(context.Background())
	if e != nil || len(status["metrics"].(map[string]any)) != len(statusVariables) {
		t.Fatalf("status: %#v %#v", status, e)
	}
	dbs, e := b.databases(context.Background())
	if e != nil || len(dbs["databases"].([]map[string]any)) != 2 {
		t.Fatalf("databases: %#v %#v", dbs, e)
	}
	vars, e := b.variables(context.Background())
	if e != nil || vars["variables"].(map[string]any)["max_connections"] != "151" {
		t.Fatalf("variables: %#v %#v", vars, e)
	}
	restart, e := b.restart(context.Background())
	if e != nil || restart["service"] != "mariadb" {
		t.Fatalf("restart: %#v %#v", restart, e)
	}
}

func TestHandlerErrors(t *testing.T) {
	b := &Backend{}
	tests := []struct {
		command string
		args    []string
		exit    int
		code    string
	}{{"", nil, 2, "MISSING_COMMAND"}, {"unknown", nil, 4, "COMMAND_NOT_FOUND"}, {"info", []string{"extra"}, 2, "INVALID_ARGUMENT_COUNT"}}
	for _, test := range tests {
		r := New(b).Handle(context.Background(), test.command, test.args)
		if r.ExitCode != test.exit || r.Response.Error == nil || r.Response.Error.Code != test.code {
			t.Fatalf("reply: %#v", r)
		}
	}
}
func TestNumericValidation(t *testing.T) {
	if _, e := number("-1", "Queries"); e == nil || e.Code != "INVALID_MYSQL_NUMERIC_VALUE" {
		t.Fatalf("error: %#v", e)
	}
	if _, e := number("invalid", "Queries"); e == nil || e.Code != "INVALID_MYSQL_NUMERIC_VALUE" {
		t.Fatalf("error: %#v", e)
	}
}

func TestMysqldServiceDiscovery(t *testing.T) {
	responses := map[string]result{}
	for _, unit := range []string{"mysql.service", "mariadb.service", "mysqld.service"} {
		for _, property := range []string{"LoadState", "ActiveState", "UnitFileState", "SubState"} {
			value := ""
			if unit == "mysqld.service" {
				value = map[string]string{"LoadState": "loaded", "ActiveState": "active", "UnitFileState": "enabled", "SubState": "running"}[property]
			}
			responses["/usr/bin/systemctl show --property="+property+" --value -- "+unit] = result{stdout: value, ok: true}
		}
	}
	b := &Backend{runner: fakeRunner{responses: responses}}
	service, err := b.service(context.Background())
	if err != nil || service["unit"] != "mysqld.service" || service["service"] != "mysqld" {
		t.Fatalf("unexpected mysqld service: %#v, %#v", service, err)
	}
}
