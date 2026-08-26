package mysql

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

var statusVariables = []string{"Aborted_connects", "Bytes_received", "Bytes_sent", "Connections", "Created_tmp_disk_tables", "Created_tmp_tables", "Innodb_buffer_pool_bytes_data", "Innodb_buffer_pool_bytes_dirty", "Innodb_buffer_pool_pages_free", "Innodb_buffer_pool_pages_total", "Innodb_rows_deleted", "Innodb_rows_inserted", "Innodb_rows_read", "Innodb_rows_updated", "Open_tables", "Opened_tables", "Queries", "Questions", "Slow_queries", "Table_locks_waited", "Threads_cached", "Threads_connected", "Threads_created", "Threads_running", "Uptime"}
var configurationVariables = []string{"bind_address", "character_set_server", "collation_server", "innodb_buffer_pool_size", "innodb_flush_log_at_trx_commit", "innodb_log_file_size", "long_query_time", "max_allowed_packet", "max_connections", "max_heap_table_size", "performance_schema", "slow_query_log", "sql_mode", "table_open_cache", "thread_cache_size", "tmp_table_size"}
var systemDatabases = map[string]struct{}{"information_schema": {}, "mysql": {}, "performance_schema": {}, "sys": {}}

type Error struct {
	ExitCode      int
	Code, Message string
	Details       map[string]any
}
type result struct {
	stdout, stderr string
	ok             bool
}
type runner interface {
	Run(context.Context, string, ...string) (result, *Error)
}
type execRunner struct{}
type Backend struct {
	runner  runner
	clients []string
}
type Handler struct{ backend *Backend }

func New(backend *Backend) *Handler { return &Handler{backend} }
func NewLinuxBackend() *Backend {
	return &Backend{runner: execRunner{}, clients: []string{
		"/usr/bin/mysql", "/usr/bin/mariadb",
		"/usr/local/bin/mysql", "/usr/local/bin/mariadb",
	}}
}

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(errOf(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine mysql.", nil))
	}
	known := map[string]bool{"info": true, "status": true, "databases": true, "variables": true, "restart": true}
	if !known[command] {
		return failure(errOf(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine mysql.", nil))
	}
	if len(arguments) != 0 {
		return failure(errOf(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.", nil))
	}
	data, err := h.backend.execute(ctx, command)
	if err != nil {
		return failure(err)
	}
	return protocol.Reply{Response: api.Success(data)}
}

func (execRunner) Run(ctx context.Context, name string, args ...string) (result, *Error) {
	commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, name, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	var out, er strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &er
	runErr := cmd.Run()
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return result{}, errOf(124, "MYSQL_COMMAND_TIMEOUT", "La commande MySQL a dépassé le délai autorisé.", nil)
	}
	r := result{strings.TrimSpace(strings.ToValidUTF8(out.String(), "�")), strings.TrimSpace(strings.ToValidUTF8(er.String(), "�")), runErr == nil}
	if runErr == nil {
		return r, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return r, nil
	}
	return result{}, errOf(10, "MYSQL_COMMAND_FAILED", "La commande MySQL n’a pas pu être exécutée.", map[string]any{"reason": runErr.Error()})
}

func (b *Backend) client() (string, *Error) {
	for _, path := range b.clients {
		if info, e := os.Stat(path); e == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return path, nil
		}
	}
	return "", errOf(4, "MYSQL_CLIENT_NOT_FOUND", "Aucun client MySQL ou MariaDB autorisé n’est installé.", nil)
}
func (b *Backend) query(ctx context.Context, sql string) ([][]string, *Error) {
	client, e := b.client()
	if e != nil {
		return nil, e
	}
	r, e := b.runner.Run(ctx, client, "--batch", "--raw", "--skip-column-names", "--protocol=socket", "--connect-timeout=5", "--execute", sql)
	if e != nil {
		return nil, e
	}
	if !r.ok {
		return nil, errOf(10, "MYSQL_QUERY_FAILED", "Le serveur MySQL n’a pas pu être interrogé.", map[string]any{"client": client, "output": r.stderr})
	}
	rows := [][]string{}
	for _, line := range strings.Split(r.stdout, "\n") {
		if line != "" {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	return rows, nil
}
func number(value, name string) (int64, *Error) {
	n, e := strconv.ParseInt(value, 10, 64)
	if e != nil || n < 0 {
		return 0, errOf(10, "INVALID_MYSQL_NUMERIC_VALUE", map[bool]string{true: "Une valeur numérique retournée par MySQL est négative.", false: "Une valeur numérique retournée par MySQL est invalide."}[e == nil && n < 0], map[string]any{"variable": name, "value": value})
	}
	return n, nil
}

func (b *Backend) systemctlValue(ctx context.Context, unit, property string) (string, *Error) {
	r, e := b.runner.Run(ctx, "/usr/bin/systemctl", "show", "--property="+property, "--value", "--", unit)
	if e != nil {
		return "", e
	}
	if !r.ok {
		return "", nil
	}
	return r.stdout, nil
}
func (b *Backend) service(ctx context.Context) (map[string]any, *Error) {
	items := []map[string]any{}
	for _, unit := range []string{"mysql.service", "mariadb.service", "mysqld.service"} {
		load, e := b.systemctlValue(ctx, unit, "LoadState")
		if e != nil {
			return nil, e
		}
		if load != "loaded" && load != "masked" {
			continue
		}
		active, e := b.systemctlValue(ctx, unit, "ActiveState")
		if e != nil {
			return nil, e
		}
		enabled, e := b.systemctlValue(ctx, unit, "UnitFileState")
		if e != nil {
			return nil, e
		}
		sub, e := b.systemctlValue(ctx, unit, "SubState")
		if e != nil {
			return nil, e
		}
		state := sub
		if state == "" {
			state = active
		}
		if state == "" {
			state = "unknown"
		}
		items = append(items, map[string]any{"unit": unit, "service": strings.TrimSuffix(unit, ".service"), "exists": true, "active": active == "active", "enabled": enabled == "enabled" || enabled == "enabled-runtime" || enabled == "static", "state": state})
	}
	if len(items) == 0 {
		return map[string]any{"unit": nil, "service": nil, "exists": false, "active": false, "enabled": false, "state": "not-found"}, nil
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i]["active"].(bool) && !items[j]["active"].(bool) })
	return items[0], nil
}

const infoSQL = "SELECT @@version, @@version_comment, @@hostname, @@port, @@socket, @@datadir, @@default_storage_engine"
const databasesSQL = "SELECT s.SCHEMA_NAME, COALESCE(SUM(t.DATA_LENGTH + t.INDEX_LENGTH), 0) AS size_bytes, COUNT(t.TABLE_NAME) AS table_count FROM information_schema.SCHEMATA AS s LEFT JOIN information_schema.TABLES AS t ON t.TABLE_SCHEMA = s.SCHEMA_NAME GROUP BY s.SCHEMA_NAME ORDER BY s.SCHEMA_NAME"

func inSQL(prefix string, names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	return prefix + " (" + strings.Join(quoted, ", ") + ")"
}
func (b *Backend) execute(ctx context.Context, command string) (map[string]any, *Error) {
	switch command {
	case "info":
		return b.info(ctx)
	case "status":
		return b.status(ctx)
	case "databases":
		return b.databases(ctx)
	case "variables":
		return b.variables(ctx)
	case "restart":
		return b.restart(ctx)
	}
	return nil, nil
}
func (b *Backend) info(ctx context.Context) (map[string]any, *Error) {
	rows, e := b.query(ctx, infoSQL)
	if e != nil {
		return nil, e
	}
	if len(rows) != 1 || len(rows[0]) != 7 {
		return nil, errOf(10, "INVALID_MYSQL_INFO_RESPONSE", "Les informations générales retournées par MySQL sont invalides.", map[string]any{"row_count": len(rows), "column_count": func() int {
			if len(rows) > 0 {
				return len(rows[0])
			}
			return 0
		}()})
	}
	r := rows[0]
	if strings.TrimSpace(r[0]) == "" || strings.TrimSpace(r[2]) == "" || !strings.HasPrefix(r[4], "/") || !strings.HasPrefix(r[5], "/") || strings.TrimSpace(r[6]) == "" {
		return nil, errOf(10, "INVALID_MYSQL_INFO_RESPONSE", "Les informations générales retournées par MySQL sont invalides.", nil)
	}
	port, nerr := number(r[3], "port")
	if nerr != nil {
		return nil, nerr
	}
	client, _ := b.client()
	service, se := b.service(ctx)
	if se != nil {
		return nil, se
	}
	combined := strings.ToLower(r[0] + " " + r[1])
	product := "Compatible MySQL"
	if strings.Contains(combined, "mariadb") {
		product = "MariaDB"
	} else if strings.Contains(combined, "mysql") {
		product = "MySQL"
	}
	return map[string]any{"product": product, "version": r[0], "version_comment": r[1], "hostname": r[2], "port": port, "socket": r[4], "data_directory": r[5], "default_storage_engine": r[6], "client": client, "service": service}, nil
}
func (b *Backend) status(ctx context.Context) (map[string]any, *Error) {
	rows, e := b.query(ctx, inSQL("SHOW GLOBAL STATUS WHERE Variable_name IN", statusVariables))
	if e != nil {
		return nil, e
	}
	raw := map[string]string{}
	for _, r := range rows {
		if len(r) != 2 {
			return nil, errOf(10, "INVALID_MYSQL_STATUS_RESPONSE", "Une métrique MySQL retournée est invalide.", nil)
		}
		raw[r[0]] = r[1]
	}
	metrics := map[string]any{}
	for _, name := range statusVariables {
		value, ok := raw[name]
		if !ok {
			return nil, errOf(10, "MISSING_MYSQL_STATUS_VARIABLE", "Une métrique MySQL obligatoire est absente.", map[string]any{"variable": name})
		}
		n, ne := number(value, name)
		if ne != nil {
			return nil, ne
		}
		metrics[strings.ToLower(name)] = n
	}
	return map[string]any{"metrics": metrics}, nil
}
func (b *Backend) databases(ctx context.Context) (map[string]any, *Error) {
	rows, e := b.query(ctx, databasesSQL)
	if e != nil {
		return nil, e
	}
	items := []map[string]any{}
	for _, r := range rows {
		if len(r) != 3 || strings.TrimSpace(r[0]) == "" {
			return nil, errOf(10, "INVALID_MYSQL_DATABASES_RESPONSE", "Une base de données retournée par MySQL est invalide.", nil)
		}
		size, se := number(r[1], r[0]+".size_bytes")
		if se != nil {
			return nil, se
		}
		count, ce := number(r[2], r[0]+".table_count")
		if ce != nil {
			return nil, ce
		}
		kind := "user"
		if _, ok := systemDatabases[r[0]]; ok {
			kind = "system"
		}
		items = append(items, map[string]any{"name": r[0], "type": kind, "size_bytes": size, "table_count": count})
	}
	return map[string]any{"databases": items}, nil
}
func (b *Backend) variables(ctx context.Context) (map[string]any, *Error) {
	rows, e := b.query(ctx, inSQL("SHOW GLOBAL VARIABLES WHERE Variable_name IN", configurationVariables))
	if e != nil {
		return nil, e
	}
	values := map[string]any{}
	for _, r := range rows {
		if len(r) != 2 {
			return nil, errOf(10, "INVALID_MYSQL_VARIABLES_RESPONSE", "Une variable de configuration MySQL est invalide.", nil)
		}
		values[strings.ToLower(r[0])] = r[1]
	}
	return map[string]any{"variables": values}, nil
}
func (b *Backend) restart(ctx context.Context) (map[string]any, *Error) {
	service, e := b.service(ctx)
	if e != nil {
		return nil, e
	}
	if !service["exists"].(bool) {
		return nil, errOf(4, "MYSQL_SERVICE_NOT_FOUND", "Aucun service MySQL ou MariaDB autorisé n’a été détecté.", nil)
	}
	r, re := b.runner.Run(ctx, "/usr/bin/systemd-run", "--quiet", "--collect", "--on-active=5s", "/usr/bin/systemctl", "restart", "--", service["unit"].(string))
	if re != nil {
		return nil, re
	}
	if !r.ok {
		output := r.stderr
		if output == "" {
			output = r.stdout
		}
		return nil, errOf(10, "MYSQL_RESTART_SCHEDULE_FAILED", "La programmation du redémarrage du serveur de bases de données a échoué.", map[string]any{"output": output})
	}
	return map[string]any{"action": "restart", "service": service["service"], "result": "scheduled", "delay_seconds": 5}, nil
}
func errOf(exit int, code, message string, details map[string]any) *Error {
	return &Error{exit, code, message, details}
}
func failure(e *Error) protocol.Reply {
	r := api.Failure(e.Code, e.Message)
	if r.Error != nil && len(e.Details) > 0 {
		r.Error.Details = e.Details
	}
	return protocol.Reply{ExitCode: e.ExitCode, Response: r}
}
