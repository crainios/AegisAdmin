package tor

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	binary   = "/usr/sbin/tor"
	config   = "/etc/tor/torrc"
	defaults = "/usr/share/tor/tor-service-defaults-torrc"
	unit     = "tor@default.service"
	service  = "tor@default"
)

type Runner interface {
	Run(context.Context, string, ...string) (string, bool)
}
type Backend struct {
	run                                                    Runner
	binary, config, defaults, unit, service, dataDirectory string
}
type runner struct{}
type Handler struct{ b *Backend }

func New(b *Backend) *Handler { return &Handler{b} }
func NewLinuxBackend() *Backend {
	b := &Backend{run: runner{}, binary: binary, config: config, defaults: defaults, unit: unit, service: service, dataDirectory: "/var/lib/tor"}
	b.loadProfile("/etc/aegisadmin-system/tor")
	if b.binary == binary && !executable(b.binary) && executable("/usr/bin/tor") {
		b.binary = "/usr/bin/tor"
	}
	if b.defaults != "" {
		if info, err := os.Stat(b.defaults); err != nil || !info.Mode().IsRegular() {
			b.defaults = ""
		}
	}
	return b
}
func (runner) Run(ctx context.Context, n string, a ...string) (string, bool) {
	c, x := context.WithTimeout(ctx, 15*time.Second)
	defer x()
	o, e := exec.CommandContext(c, n, a...).CombinedOutput()
	return strings.TrimSpace(string(o)), e == nil
}
func (h *Handler) Handle(ctx context.Context, c string, a []string) protocol.Reply {
	if c == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine Tor.")
	}
	known := map[string]bool{"info": true, "status": true, "configtest": true, "hidden-services": true, "reload": true, "restart": true}
	if !known[c] {
		return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine Tor.")
	}
	if len(a) != 0 {
		return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
	}
	if !executable(h.b.binary) {
		return fail(5, "TOR_NOT_INSTALLED", "Tor n’est pas installé sur ce serveur.")
	}
	if f, e := os.Open(h.b.config); e != nil {
		return fail(5, "TOR_CONFIG_NOT_FOUND", "La configuration principale de Tor est introuvable.")
	} else {
		f.Close()
	}
	d, e := h.b.execute(ctx, c)
	if e != nil {
		return *e
	}
	return protocol.Reply{Response: api.Success(d)}
}
func (b *Backend) execute(ctx context.Context, c string) (map[string]any, *protocol.Reply) {
	switch c {
	case "info":
		o, ok := b.run.Run(ctx, b.binary, "--version")
		v := strings.TrimSuffix(strings.TrimPrefix(strings.Split(o, "\n")[0], "Tor version "), ".")
		if !ok || v == "" || v == o {
			return nil, reply(10, "TOR_VERSION_INVALID", "La version de Tor n’a pas pu être déterminée.")
		}
		return map[string]any{"product": "Tor", "version": v, "config_file": b.config, "service": b.service, "unit": b.unit}, nil
	case "status":
		return b.status(ctx), nil
	case "configtest":
		if !b.valid(ctx) {
			return nil, reply(9, "TOR_CONFIG_INVALID", "La configuration de Tor est invalide.")
		}
		return map[string]any{"valid": true, "message": "La configuration de Tor est valide."}, nil
	case "hidden-services":
		s, ok := hidden(b.config, b.dataDirectory)
		if !ok {
			return nil, reply(10, "TOR_HIDDEN_SERVICES_FAILED", "Les services Onion n’ont pas pu être lus.")
		}
		return map[string]any{"services": s}, nil
	case "reload":
		if !b.valid(ctx) {
			return nil, reply(9, "TOR_CONFIG_INVALID", "La configuration de Tor est invalide.")
		}
		if !b.exists(ctx) {
			return nil, reply(5, "TOR_SERVICE_NOT_FOUND", "Le service Tor est introuvable.")
		}
		if _, ok := b.run.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "--", b.unit); !ok {
			return nil, reply(7, "TOR_SERVICE_NOT_ACTIVE", "Le service Tor n’est pas actif.")
		}
		if _, ok := b.run.Run(ctx, "/usr/bin/systemctl", "reload", "--", b.unit); !ok {
			return nil, reply(10, "TOR_RELOAD_FAILED", "Le rechargement de Tor a échoué.")
		}
		return map[string]any{"action": "reload", "service": b.service, "result": "success"}, nil
	case "restart":
		if !b.valid(ctx) {
			return nil, reply(9, "TOR_CONFIG_INVALID", "La configuration de Tor est invalide.")
		}
		if !b.exists(ctx) {
			return nil, reply(5, "TOR_SERVICE_NOT_FOUND", "Le service Tor est introuvable.")
		}
		if _, ok := b.run.Run(ctx, "/usr/bin/systemd-run", "--quiet", "--collect", "--on-active=5s", "/usr/bin/systemctl", "restart", "--", b.unit); !ok {
			return nil, reply(10, "TOR_RESTART_FAILED", "La programmation du redémarrage de Tor a échoué.")
		}
		return map[string]any{"action": "restart", "service": b.service, "result": "scheduled", "delay_seconds": 5}, nil
	}
	return nil, nil
}
func (b *Backend) value(ctx context.Context, p, f string) string {
	o, ok := b.run.Run(ctx, "/usr/bin/systemctl", "show", "--property="+p, "--value", "--", b.unit)
	if !ok || o == "" {
		return f
	}
	return o
}
func (b *Backend) exists(ctx context.Context) bool {
	v := b.value(ctx, "LoadState", "not-found")
	return v == "loaded" || v == "masked"
}
func (b *Backend) valid(ctx context.Context) bool {
	arguments := []string{}
	if b.defaults != "" {
		arguments = append(arguments, "--defaults-torrc", b.defaults)
	}
	arguments = append(arguments, "-f", b.config, "--RunAsDaemon", "0", "--verify-config")
	output, ok := b.run.Run(ctx, b.binary, arguments...)
	if !ok && output != "" {
		_, _ = os.Stderr.WriteString("tor configtest: " + output + "\n")
	}
	return ok
}
func (b *Backend) status(ctx context.Context) map[string]any {
	load := b.value(ctx, "LoadState", "not-found")
	exists := load == "loaded" || load == "masked"
	d := map[string]any{"service": b.service, "unit": b.unit, "exists": exists, "active": false, "enabled": false, "load_state": load, "active_state": "inactive", "state": "unknown", "main_pid": int64(0), "memory_bytes": int64(0), "tasks": int64(0), "bootstrap_percent": nil}
	if !exists {
		return d
	}
	_, d["active"] = b.run.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "--", b.unit)
	_, d["enabled"] = b.run.Run(ctx, "/usr/bin/systemctl", "is-enabled", "--quiet", "--", b.unit)
	d["active_state"] = b.value(ctx, "ActiveState", "inactive")
	d["state"] = b.value(ctx, "SubState", "unknown")
	for p, k := range map[string]string{"MainPID": "main_pid", "MemoryCurrent": "memory_bytes", "TasksCurrent": "tasks"} {
		n, _ := strconv.ParseInt(b.value(ctx, p, "0"), 10, 64)
		if n >= 0 {
			d[k] = n
		}
	}
	o, _ := b.run.Run(ctx, "/usr/bin/journalctl", "--unit="+b.unit, "--output=cat", "--no-pager", "--lines=300")
	re := regexp.MustCompile(`Bootstrapped ([0-9]+)%`)
	m := re.FindAllStringSubmatch(o, -1)
	if len(m) > 0 {
		n, _ := strconv.Atoi(m[len(m)-1][1])
		if n <= 100 {
			d["bootstrap_percent"] = n
		}
	}
	return d
}

type onion struct {
	ID       string           `json:"id"`
	Hostname *string          `json:"hostname"`
	Ports    []map[string]any `json:"ports"`
}

func hidden(path, dataDirectory string) ([]onion, bool) {
	f, e := os.Open(path)
	if e != nil {
		return nil, false
	}
	defer f.Close()
	items := []onion{}
	var cur *onion
	s := bufio.NewScanner(f)
	dirRe := regexp.MustCompile(`(?i)^HiddenServiceDir\s+(\S+)\s*$`)
	portRe := regexp.MustCompile(`(?i)^HiddenServicePort\s+(\d+)\s+(\S+)\s*$`)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if m := dirRe.FindStringSubmatch(l); m != nil {
			d := filepath.Clean(m[1])
			if d == dataDirectory || !strings.HasPrefix(d, dataDirectory+string(os.PathSeparator)) {
				cur = nil
				continue
			}
			id := strings.TrimPrefix(filepath.Base(d), "aegisadmin-")
			items = append(items, onion{ID: id, Ports: []map[string]any{}})
			cur = &items[len(items)-1]
			if h, e := os.ReadFile(filepath.Join(d, "hostname")); e == nil {
				v := strings.TrimSpace(string(h))
				if regexp.MustCompile(`^[a-z2-7]{56}\.onion$`).MatchString(v) {
					cur.Hostname = &v
				}
			}
			continue
		}
		if m := portRe.FindStringSubmatch(l); m != nil && cur != nil {
			n, _ := strconv.Atoi(m[1])
			if n >= 1 && n <= 65535 {
				cur.Ports = append(cur.Ports, map[string]any{"public_port": n, "target": m[2]})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, s.Err() == nil
}
func executable(p string) bool {
	i, e := os.Stat(p)
	return e == nil && i.Mode().IsRegular() && i.Mode()&0111 != 0
}
func reply(x int, c, m string) *protocol.Reply { r := fail(x, c, m); return &r }
func fail(x int, c, m string) protocol.Reply {
	return protocol.Reply{ExitCode: x, Response: api.Failure(c, m)}
}
