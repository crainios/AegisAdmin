package firewall

import (
	"context"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const ufw = "/usr/sbin/ufw"
const firewallCMD = "/usr/bin/firewall-cmd"
const defaultsFile = "/etc/default/ufw"

var goCLI = "/usr/local/sbin/aegisadmin-system-go"

type Runner interface {
	Run(context.Context, string, ...string) (string, string, bool)
}
type Backend struct {
	runner                   Runner
	ufw, firewalld, defaults string
}
type execRunner struct{}
type Handler struct{ backend *Backend }
type Rule struct {
	ID          int      `json:"id"`
	Action      string   `json:"action"`
	Direction   string   `json:"direction"`
	Protocol    string   `json:"protocol"`
	Ports       []string `json:"ports"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Family      string   `json:"family"`
	line        string
}
type Listener struct {
	Port     int     `json:"port"`
	Protocol string  `json:"protocol"`
	Service  *string `json:"service"`
	Process  *string `json:"process"`
}

var counts = map[string]int{"info": 0, "status": 0, "rules": 0, "add": 4, "add-execute": 4, "delete-check": 1, "delete": 3, "delete-execute": 3, "enable": 0, "disable": 0, "reload": 0}
var numbered = regexp.MustCompile(`^\[\s*(\d+)\]\s+(.+?)\s{2,}(.+?)\s{2,}(.+?)\s*$`)
var versionPattern = regexp.MustCompile(`(?i)\bufw\s+([0-9][0-9A-Za-z.+:~_-]*)`)
var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func New(b *Backend) *Handler { return &Handler{b} }
func NewLinuxBackend() *Backend {
	return &Backend{runner: execRunner{}, ufw: ufw, firewalld: firewallCMD, defaults: defaultsFile}
}
func (execRunner) Run(ctx context.Context, n string, a ...string) (string, string, bool) {
	c, x := context.WithTimeout(ctx, 15*time.Second)
	defer x()
	cmd := exec.CommandContext(c, n, a...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	var out, er strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &er
	e := cmd.Run()
	return strings.TrimSpace(out.String()), strings.TrimSpace(er.String()), e == nil
}

func (h *Handler) Handle(ctx context.Context, c string, a []string) protocol.Reply {
	if c == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine Firewall.")
	}
	n, ok := counts[c]
	if !ok {
		return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine Firewall.")
	}
	if len(a) != n {
		return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
	}
	if !executable(h.backend.ufw) && !executable(h.backend.firewalld) {
		return fail(5, "FIREWALL_NOT_INSTALLED", "Aucun pare-feu UFW ou firewalld pris en charge n’est installé sur ce serveur.")
	}
	d, e := h.backend.execute(ctx, c, a)
	if e != nil {
		return *e
	}
	return protocol.Reply{Response: api.Success(d)}
}
func (b *Backend) runUFW(ctx context.Context, a ...string) (string, *protocol.Reply) {
	o, er, ok := b.runner.Run(ctx, b.ufw, a...)
	if !ok {
		m := er
		if m == "" {
			m = o
		}
		if m == "" {
			m = "La commande UFW a échoué."
		}
		return "", reply(10, "UFW_READ_FAILED", m)
	}
	return o, nil
}
func (b *Backend) read(ctx context.Context) (map[string]any, []Rule, *protocol.Reply) {
	verbose, e := b.runUFW(ctx, "status", "verbose")
	if e != nil {
		return nil, nil, e
	}
	status := parseVerbose(verbose)
	numberedOutput, e := b.runUFW(ctx, "status", "numbered")
	if e != nil {
		return nil, nil, e
	}
	return status, parseRules(numberedOutput), nil
}
func (b *Backend) execute(ctx context.Context, c string, a []string) (map[string]any, *protocol.Reply) {
	if !executable(b.ufw) && executable(b.firewalld) {
		return b.executeFirewalld(ctx, c, a)
	}
	if c == "add-execute" {
		return nil, reply(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine Firewall.")
	}
	switch c {
	case "info":
		s, _, e := b.read(ctx)
		if e != nil {
			return nil, e
		}
		vout, e := b.runUFW(ctx, "version")
		if e != nil {
			return nil, e
		}
		var version any = nil
		if m := versionPattern.FindStringSubmatch(vout); m != nil {
			version = m[1]
		}
		return map[string]any{"backend": "ufw", "product": "UFW", "installed": true, "version": version, "active": s["active"], "ipv6": readIPv6(b.defaults), "default_incoming": s["default_incoming"], "default_outgoing": s["default_outgoing"], "default_routed": s["default_routed"]}, nil
	case "status":
		s, r, e := b.read(ctx)
		if e != nil {
			return nil, e
		}
		s["rule_count"] = len(r)
		return s, nil
	case "rules":
		_, r, e := b.read(ctx)
		if e != nil {
			return nil, e
		}
		return map[string]any{"rules": r, "count": len(r)}, nil
	case "add":
		if e := validateAdd(a); e != nil {
			return nil, e
		}
		return b.schedule(ctx, "add", b.ufw, a[0], "from", a[3], "to", "any", "port", a[1], "proto", a[2])
	case "enable":
		return b.schedule(ctx, "enable", b.ufw, "--force", "enable")
	case "disable":
		return b.schedule(ctx, "disable", b.ufw, "--force", "disable")
	case "reload":
		return b.schedule(ctx, "reload", b.ufw, "reload")
	case "delete-check":
		id, e := validID(a[0])
		if e != nil {
			return nil, e
		}
		return b.usage(ctx, id)
	case "delete", "delete-execute":
		return b.delete(ctx, c, a)
	}
	return nil, nil
}

func parseVerbose(o string) map[string]any {
	d := map[string]any{"active": false, "logging": "unknown", "default_incoming": "unknown", "default_outgoing": "unknown", "default_routed": "unknown"}
	status := regexp.MustCompile(`(?i)^Status:\s*(active|inactive)$`)
	logging := regexp.MustCompile(`(?i)^Logging:\s*(.+?)\s*$`)
	def := regexp.MustCompile(`(?i)^Default:\s*([^\s]+)\s*\(incoming\),\s*([^\s]+)\s*\(outgoing\),\s*([^\s]+)\s*\(routed\)$`)
	for _, raw := range strings.Split(o, "\n") {
		l := strings.TrimSpace(raw)
		if m := status.FindStringSubmatch(l); m != nil {
			d["active"] = strings.EqualFold(m[1], "active")
		}
		if m := logging.FindStringSubmatch(l); m != nil {
			d["logging"] = strings.ToLower(strings.TrimSpace(m[1]))
		}
		if m := def.FindStringSubmatch(l); m != nil {
			d["default_incoming"] = strings.ToLower(m[1])
			d["default_outgoing"] = strings.ToLower(m[2])
			d["default_routed"] = strings.ToLower(m[3])
		}
	}
	return d
}
func parseRules(o string) []Rule {
	r := []Rule{}
	proto := regexp.MustCompile(`(?i)/(tcp|udp)\s*$`)
	ports := regexp.MustCompile(`^[0-9]+(?:[:,-][0-9]+)*$`)
	v6 := regexp.MustCompile(`(?i)\s*\(v6\)\s*$`)
	for _, raw := range strings.Split(o, "\n") {
		line := strings.TrimSpace(raw)
		m := numbered.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		target, actionText, source := strings.TrimSpace(m[2]), strings.TrimSpace(m[3]), strings.TrimSpace(m[4])
		family := "ipv4"
		if strings.Contains(strings.ToLower(target), "(v6)") || strings.Contains(strings.ToLower(source), "(v6)") {
			family = "ipv6"
		}
		target = v6.ReplaceAllString(target, "")
		source = v6.ReplaceAllString(source, "")
		parts := strings.Fields(actionText)
		action, direction := "unknown", "in"
		if len(parts) > 0 {
			action = strings.ToLower(parts[0])
		}
		if len(parts) > 1 {
			direction = strings.ToLower(parts[1])
		}
		protocol, destination := "any", strings.TrimSpace(target)
		if p := proto.FindStringSubmatchIndex(target); p != nil {
			protocol = strings.ToLower(target[p[2]:p[3]])
			destination = strings.TrimSpace(target[:p[0]])
		}
		ps := []string{}
		if ports.MatchString(destination) {
			ps = []string{destination}
			destination = "any"
		}
		if destination == "" {
			destination = "any"
		}
		r = append(r, Rule{id, action, direction, protocol, ps, source, destination, family, raw})
	}
	return r
}
func readIPv6(path string) any {
	content, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	re := regexp.MustCompile(`(?im)^\s*IPV6\s*=\s*["']?([^"'\s#]+)`)
	m := re.FindSubmatch(content)
	if m == nil {
		return nil
	}
	switch strings.ToLower(string(m[1])) {
	case "yes", "true", "1", "on":
		return true
	case "no", "false", "0", "off":
		return false
	}
	return nil
}
func validID(v string) (int, *protocol.Reply) {
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(v) {
		return 0, reply(2, "INVALID_RULE_ID", "L’identifiant de la règle UFW est invalide.")
	}
	n, _ := strconv.Atoi(v)
	return n, nil
}
func validateAdd(a []string) *protocol.Reply {
	if !map[string]bool{"allow": true, "deny": true, "reject": true, "limit": true}[a[0]] {
		return reply(2, "INVALID_FIREWALL_ACTION", "L’action de la règle du pare-feu est invalide.")
	}
	if a[2] != "tcp" && a[2] != "udp" {
		return reply(2, "INVALID_FIREWALL_PROTOCOL", "Le protocole de la règle du pare-feu est invalide.")
	}
	for _, p := range strings.Split(a[1], ",") {
		x := strings.Split(strings.TrimSpace(p), ":")
		if len(x) > 2 {
			return reply(2, "INVALID_FIREWALL_RULE", "Le port, la plage de ports ou la source est invalide.")
		}
		start, e := strconv.Atoi(x[0])
		end := start
		if len(x) == 2 {
			end, e = strconv.Atoi(x[1])
		}
		if e != nil || start < 1 || start > end || end > 65535 {
			return reply(2, "INVALID_FIREWALL_RULE", "Le port, la plage de ports ou la source est invalide.")
		}
	}
	if a[3] != "any" {
		if _, e := netip.ParsePrefix(a[3]); e != nil {
			if ip, e2 := netip.ParseAddr(a[3]); e2 != nil || !ip.IsValid() {
				return reply(2, "INVALID_FIREWALL_RULE", "Le port, la plage de ports ou la source est invalide.")
			}
		}
	}
	return nil
}
func (b *Backend) schedule(ctx context.Context, action, command string, args ...string) (map[string]any, *protocol.Reply) {
	all := []string{"--quiet", "--collect", "--on-active=5s", command}
	all = append(all, args...)
	_, _, ok := b.runner.Run(ctx, "/usr/bin/systemd-run", all...)
	if !ok {
		code := map[string]string{"add": "UFW_ADD_FAILED", "enable": "UFW_ENABLE_FAILED", "disable": "UFW_DISABLE_FAILED", "reload": "UFW_RELOAD_FAILED"}[action]
		if action == "delete" {
			code = "UFW_ACTION_SCHEDULE_FAILED"
		}
		return nil, reply(10, code, "La programmation de l’action UFW a échoué.")
	}
	return map[string]any{"action": action, "backend": "ufw", "result": "scheduled", "delay_seconds": 5}, nil
}
func executable(p string) bool {
	i, e := os.Stat(p)
	return e == nil && i.Mode().IsRegular() && i.Mode()&0111 != 0
}
func reply(x int, c, m string) *protocol.Reply { r := fail(x, c, m); return &r }
func fail(x int, c, m string) protocol.Reply {
	return protocol.Reply{ExitCode: x, Response: api.Failure(c, m)}
}
