package fail2ban

import (
	"context"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const client = "/usr/bin/fail2ban-client"
const unit = "fail2ban.service"
const service = "fail2ban"
const profilePath = "/etc/aegisadmin-system/fail2ban"

type Runner interface {
	Run(context.Context, string, ...string) (string, bool)
}
type Backend struct {
	runner                Runner
	client, unit, service string
}
type execRunner struct{}
type Handler struct{ backend *Backend }

var counts = map[string]int{"info": 0, "status": 0, "jail": 1, "configtest": 0, "reload": 0, "restart": 0, "ban": 2, "unban": 2}
var jailPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func New(b *Backend) *Handler { return &Handler{b} }
func NewLinuxBackend() *Backend {
	b := &Backend{runner: execRunner{}, client: client, unit: unit, service: service}
	b.loadProfile(profilePath)
	if b.client == client && !executable(b.client) && executable("/usr/local/bin/fail2ban-client") {
		b.client = "/usr/local/bin/fail2ban-client"
	}
	return b
}
func (execRunner) Run(ctx context.Context, name string, args ...string) (string, bool) {
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	o, e := exec.CommandContext(c, name, args...).CombinedOutput()
	return strings.TrimSpace(strings.ToValidUTF8(string(o), "�")), e == nil
}

func (h *Handler) Handle(ctx context.Context, command string, args []string) protocol.Reply {
	if command == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine Fail2ban.")
	}
	expected, ok := counts[command]
	if !ok {
		return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine Fail2ban.")
	}
	if len(args) != expected {
		return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
	}
	if !executable(h.backend.client) {
		return fail(5, "FAIL2BAN_NOT_INSTALLED", "Fail2ban n’est pas installé sur ce serveur.")
	}
	data, e := h.backend.execute(ctx, command, args)
	if e != nil {
		return *e
	}
	return protocol.Reply{Response: api.Success(data)}
}
func (b *Backend) execute(ctx context.Context, c string, a []string) (map[string]any, *protocol.Reply) {
	switch c {
	case "info":
		o, _ := b.runner.Run(ctx, b.client, "--version")
		v := strings.TrimPrefix(o, "Fail2Ban v")
		if v == "" || v == o {
			return nil, reply(10, "FAIL2BAN_VERSION_INVALID", "La version de Fail2ban n’a pas pu être déterminée.")
		}
		return map[string]any{"product": "Fail2ban", "version": v, "service": b.service, "unit": b.unit}, nil
	case "status":
		return b.status(ctx)
	case "jail":
		if e := b.requireActive(ctx); e != nil {
			return nil, e
		}
		if e := b.requireJail(ctx, a[0]); e != nil {
			return nil, e
		}
		s, ok := b.jailStatus(ctx, a[0])
		if !ok {
			return nil, reply(10, "FAIL2BAN_JAIL_STATUS_FAILED", "L’état de la prison Fail2ban n’a pas pu être obtenu.")
		}
		return map[string]any{"jail": a[0], "status": s}, nil
	case "configtest":
		if _, ok := b.runner.Run(ctx, b.client, "--test"); !ok {
			return nil, reply(9, "FAIL2BAN_CONFIG_INVALID", "La configuration de Fail2ban est invalide.")
		}
		return map[string]any{"valid": true, "message": "La configuration de Fail2ban est valide."}, nil
	case "reload":
		if e := b.requireActive(ctx); e != nil {
			return nil, e
		}
		if _, ok := b.runner.Run(ctx, b.client, "--test"); !ok {
			return nil, reply(9, "FAIL2BAN_CONFIG_INVALID", "La configuration de Fail2ban est invalide.")
		}
		if _, ok := b.runner.Run(ctx, b.client, "reload"); !ok {
			return nil, reply(10, "FAIL2BAN_RELOAD_FAILED", "Le rechargement de Fail2ban a échoué.")
		}
		return map[string]any{"action": "reload", "service": b.service, "result": "success"}, nil
	case "restart":
		if !b.exists(ctx) {
			return nil, reply(5, "FAIL2BAN_SERVICE_NOT_FOUND", "Le service Fail2ban est introuvable.")
		}
		if _, ok := b.runner.Run(ctx, b.client, "--test"); !ok {
			return nil, reply(9, "FAIL2BAN_CONFIG_INVALID", "La configuration de Fail2ban est invalide.")
		}
		if _, ok := b.runner.Run(ctx, "/usr/bin/systemd-run", "--quiet", "--collect", "--on-active=5s", "/usr/bin/systemctl", "restart", "--", b.unit); !ok {
			return nil, reply(10, "FAIL2BAN_RESTART_FAILED", "La programmation du redémarrage de Fail2ban a échoué.")
		}
		return map[string]any{"action": "restart", "service": b.service, "result": "scheduled", "delay_seconds": 5}, nil
	case "ban", "unban":
		if e := b.requireActive(ctx); e != nil {
			return nil, e
		}
		if e := b.requireJail(ctx, a[0]); e != nil {
			return nil, e
		}
		ip, e := netip.ParseAddr(a[1])
		if e != nil {
			return nil, reply(6, "INVALID_IP_ADDRESS", "L’adresse IP fournie est invalide.")
		}
		verb := map[string]string{"ban": "banip", "unban": "unbanip"}[c]
		if _, ok := b.runner.Run(ctx, b.client, "set", a[0], verb, ip.String()); !ok {
			if c == "ban" {
				return nil, reply(10, "FAIL2BAN_BAN_FAILED", "L’adresse IP n’a pas pu être bannie.")
			}
			return nil, reply(10, "FAIL2BAN_UNBAN_FAILED", "L’adresse IP n’a pas pu être débannie.")
		}
		return map[string]any{"action": c, "jail": a[0], "address": a[1], "result": "success"}, nil
	}
	return nil, nil
}
func (b *Backend) value(ctx context.Context, p, f string) string {
	o, ok := b.runner.Run(ctx, "/usr/bin/systemctl", "show", "--property="+p, "--value", "--", b.unit)
	if !ok || o == "" {
		return f
	}
	return o
}
func (b *Backend) exists(ctx context.Context) bool {
	v := b.value(ctx, "LoadState", "not-found")
	return v == "loaded" || v == "masked"
}
func (b *Backend) active(ctx context.Context) bool {
	_, ok := b.runner.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "--", b.unit)
	return ok
}
func (b *Backend) requireActive(ctx context.Context) *protocol.Reply {
	if !b.exists(ctx) {
		return reply(5, "FAIL2BAN_SERVICE_NOT_FOUND", "Le service Fail2ban est introuvable.")
	}
	if !b.active(ctx) {
		return reply(7, "FAIL2BAN_SERVICE_NOT_ACTIVE", "Le service Fail2ban n’est pas actif.")
	}
	return nil
}
func (b *Backend) jails(ctx context.Context) ([]string, bool) {
	o, ok := b.runner.Run(ctx, b.client, "status")
	if !ok {
		return nil, false
	}
	re := regexp.MustCompile(`(?m)Jail list:\s*(.*)$`)
	m := re.FindStringSubmatch(o)
	if m == nil {
		return []string{}, true
	}
	seen := map[string]bool{}
	for _, v := range strings.Split(m[1], ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			seen[v] = true
		}
	}
	r := []string{}
	for v := range seen {
		r = append(r, v)
	}
	sort.Slice(r, func(i, j int) bool { return strings.ToLower(r[i]) < strings.ToLower(r[j]) })
	return r, true
}
func (b *Backend) requireJail(ctx context.Context, j string) *protocol.Reply {
	if !jailPattern.MatchString(j) {
		return reply(6, "INVALID_JAIL", "Le nom de la prison Fail2ban est invalide.")
	}
	js, ok := b.jails(ctx)
	if !ok {
		return reply(10, "FAIL2BAN_JAILS_FAILED", "La liste des prisons Fail2ban n’a pas pu être obtenue.")
	}
	for _, v := range js {
		if v == j {
			return nil
		}
	}
	return reply(6, "FAIL2BAN_JAIL_NOT_FOUND", "La prison Fail2ban demandée n’existe pas.")
}
func integer(text, label string) int64 {
	re := regexp.MustCompile(regexp.QuoteMeta(label) + `:\s*(\d+)`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	return n
}
func (b *Backend) jailStatus(ctx context.Context, j string) (map[string]any, bool) {
	o, ok := b.runner.Run(ctx, b.client, "status", j)
	if !ok {
		return nil, false
	}
	ips := []netip.Addr{}
	re := regexp.MustCompile(`(?m)Banned IP list:\s*(.*)$`)
	if m := re.FindStringSubmatch(o); m != nil {
		seen := map[netip.Addr]bool{}
		for _, v := range strings.Fields(m[1]) {
			if ip, e := netip.ParseAddr(v); e == nil {
				seen[ip] = true
			}
		}
		for ip := range seen {
			ips = append(ips, ip)
		}
		sort.Slice(ips, func(i, j int) bool { return ips[i].Compare(ips[j]) < 0 })
	}
	values := []string{}
	for _, ip := range ips {
		values = append(values, ip.String())
	}
	return map[string]any{"currently_failed": integer(o, "Currently failed"), "total_failed": integer(o, "Total failed"), "currently_banned": integer(o, "Currently banned"), "total_banned": integer(o, "Total banned"), "banned_ips": values}, true
}
func (b *Backend) status(ctx context.Context) (map[string]any, *protocol.Reply) {
	load := b.value(ctx, "LoadState", "not-found")
	exists := load == "loaded" || load == "masked"
	d := map[string]any{"service": b.service, "unit": b.unit, "exists": exists, "active": false, "enabled": false, "load_state": load, "active_state": "inactive", "state": "unknown", "main_pid": int64(0), "memory_bytes": int64(0), "tasks": int64(0), "jails": []string{}}
	if !exists {
		return d, nil
	}
	d["active"] = b.active(ctx)
	_, d["enabled"] = b.runner.Run(ctx, "/usr/bin/systemctl", "is-enabled", "--quiet", "--", b.unit)
	d["active_state"] = b.value(ctx, "ActiveState", "inactive")
	d["state"] = b.value(ctx, "SubState", "unknown")
	for p, k := range map[string]string{"MainPID": "main_pid", "MemoryCurrent": "memory_bytes", "TasksCurrent": "tasks"} {
		n, _ := strconv.ParseInt(b.value(ctx, p, "0"), 10, 64)
		if n >= 0 {
			d[k] = n
		}
	}
	if d["active"].(bool) {
		j, ok := b.jails(ctx)
		if !ok {
			return nil, reply(10, "FAIL2BAN_JAILS_FAILED", "La liste des prisons Fail2ban n’a pas pu être obtenue.")
		}
		d["jails"] = j
	}
	return d, nil
}
func executable(p string) bool {
	i, e := os.Stat(p)
	return e == nil && i.Mode().IsRegular() && i.Mode()&0111 != 0
}
func reply(x int, c, m string) *protocol.Reply { r := fail(x, c, m); return &r }
func fail(x int, c, m string) protocol.Reply {
	return protocol.Reply{ExitCode: x, Response: api.Failure(c, m)}
}
