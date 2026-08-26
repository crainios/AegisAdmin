package firewall

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func (b *Backend) rule(ctx context.Context, id int) (Rule, *protocol.Reply) {
	if !executable(b.ufw) && executable(b.firewalld) {
		rules, failure := b.firewalldRules(ctx)
		if failure != nil {
			return Rule{}, failure
		}
		for _, rule := range rules {
			if rule.ID == id {
				return rule, nil
			}
		}
		return Rule{}, reply(6, "FIREWALL_RULE_NOT_FOUND", "La règle firewalld demandée n’existe plus.")
	}
	o, e := b.runUFW(ctx, "status", "numbered")
	if e != nil {
		return Rule{}, e
	}
	for _, r := range parseRules(o) {
		if r.ID == id {
			return r, nil
		}
	}
	return Rule{}, reply(6, "UFW_RULE_NOT_FOUND", "La règle UFW demandée n’existe plus.")
}
func fingerprint(line string) string {
	d := sha256.Sum256([]byte(line))
	return hex.EncodeToString(d[:])
}
func (b *Backend) usage(ctx context.Context, id int) (map[string]any, *protocol.Reply) {
	r, e := b.rule(ctx, id)
	if e != nil {
		return nil, e
	}
	fp := fingerprint(r.line)
	ports := map[int]bool{}
	expr := r.Destination
	if len(r.Ports) > 0 {
		expr = r.Ports[0]
	}
	for _, item := range strings.Split(expr, ",") {
		p := strings.Split(strings.TrimSpace(item), ":")
		start, x := strconv.Atoi(p[0])
		if x != nil {
			continue
		}
		end := start
		if len(p) == 2 {
			end, _ = strconv.Atoi(p[1])
		}
		for n := start; n <= end && n <= 65535; n++ {
			ports[n] = true
		}
	}
	out, _, _ := b.runner.Run(ctx, "/usr/bin/ss", "-H", "-lntup")
	listeners := []Listener{}
	serviceRe := regexp.MustCompile(`\("([^"]+)"`)
	portRe := regexp.MustCompile(`:(\d+)$`)
	for _, line := range strings.Split(out, "\n") {
		cols := strings.Fields(line)
		if len(cols) < 5 {
			continue
		}
		m := portRe.FindStringSubmatch(cols[4])
		if m == nil {
			continue
		}
		port, _ := strconv.Atoi(m[1])
		proto := strings.ToLower(cols[0])
		if !ports[port] || (r.Protocol != "any" && r.Protocol != proto) {
			continue
		}
		process := ""
		if len(cols) > 6 {
			process = strings.Join(cols[6:], " ")
		}
		var processPtr, servicePtr *string
		if process != "" {
			processPtr = &process
			if sm := serviceRe.FindStringSubmatch(process); sm != nil {
				s := sm[1]
				servicePtr = &s
			}
		}
		candidate := Listener{port, proto, servicePtr, processPtr}
		duplicate := false
		for _, v := range listeners {
			if v.Port == candidate.Port && v.Protocol == candidate.Protocol && str(v.Service) == str(candidate.Service) && str(v.Process) == str(candidate.Process) {
				duplicate = true
			}
		}
		if !duplicate {
			listeners = append(listeners, candidate)
		}
	}
	sort.Slice(listeners, func(i, j int) bool {
		if listeners[i].Port != listeners[j].Port {
			return listeners[i].Port < listeners[j].Port
		}
		if listeners[i].Protocol != listeners[j].Protocol {
			return listeners[i].Protocol < listeners[j].Protocol
		}
		return str(listeners[i].Service) < str(listeners[j].Service)
	})
	risk := false
	for _, l := range listeners {
		if l.Port == 22 || str(l.Service) == "ssh" || str(l.Service) == "sshd" {
			risk = true
		}
	}
	return map[string]any{"rule_id": id, "fingerprint": fp, "in_use": len(listeners) > 0, "ssh_access_risk": risk, "listeners": listeners}, nil
}
func (b *Backend) delete(ctx context.Context, command string, a []string) (map[string]any, *protocol.Reply) {
	id, e := validID(a[0])
	if e != nil {
		return nil, e
	}
	if !fingerprintPattern.MatchString(a[1]) {
		return nil, reply(2, "INVALID_RULE_FINGERPRINT", "L’empreinte de la règle UFW est invalide.")
	}
	if a[2] != "yes" && a[2] != "no" {
		return nil, reply(2, "INVALID_CONFIRMATION", "La confirmation de suppression est invalide.")
	}
	usage, e := b.usage(ctx, id)
	if e != nil {
		return nil, e
	}
	if usage["fingerprint"] != a[1] {
		return nil, reply(9, "UFW_RULE_CHANGED", "La règle UFW a changé. Rechargez la liste avant de recommencer.")
	}
	if a[2] != "yes" && usage["in_use"].(bool) {
		message := "Un service actif utilise le port concerné. Une confirmation explicite est requise."
		if command == "delete-execute" {
			message = "La suppression a été annulée car un service utilise désormais ce port."
		}
		return nil, reply(8, "UFW_RULE_IN_USE", message)
	}
	if command == "delete" {
		if !executable(b.ufw) && executable(b.firewalld) {
			return b.scheduleFirewalld(ctx, "delete", goCLI, "firewall", "delete-execute", a[0], a[1], a[2])
		}
		return b.schedule(ctx, "delete", goCLI, "firewall", "delete-execute", a[0], a[1], a[2])
	}
	if !executable(b.ufw) && executable(b.firewalld) {
		rule, failure := b.rule(ctx, id)
		if failure != nil {
			return nil, failure
		}
		argument := "--remove-rich-rule=" + rule.line
		if !strings.HasPrefix(rule.line, "rule ") {
			argument = "--remove-port=" + rule.line
		}
		if _, _, ok := b.runner.Run(ctx, b.firewalld, "--permanent", argument); !ok {
			return nil, reply(10, "FIREWALL_DELETE_FAILED", "La suppression de la règle firewalld a échoué.")
		}
		if _, _, ok := b.runner.Run(ctx, b.firewalld, "--reload"); !ok {
			return nil, reply(10, "FIREWALL_RELOAD_FAILED", "Le rechargement de firewalld a échoué.")
		}
		return map[string]any{"action": "delete", "backend": "firewalld", "result": "completed", "rule_id": id}, nil
	}
	_, _, ok := b.runner.Run(ctx, b.ufw, "--force", "delete", a[0])
	if !ok {
		return nil, reply(10, "UFW_DELETE_FAILED", "La suppression de la règle UFW a échoué.")
	}
	return map[string]any{"action": "delete", "backend": "ufw", "result": "completed", "rule_id": id}, nil
}
func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
