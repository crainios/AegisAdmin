package firewall

import (
	"context"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"aegisadmin/backend/internal/protocol"
)

var richAttribute = regexp.MustCompile(`([a-z]+)="([^"]+)"`)

func (b *Backend) runFirewalld(ctx context.Context, args ...string) (string, *protocol.Reply) {
	output, stderr, ok := b.runner.Run(ctx, b.firewalld, args...)
	if !ok {
		message := stderr
		if message == "" {
			message = output
		}
		if message == "" {
			message = "La commande firewalld a échoué."
		}
		return "", reply(10, "FIREWALLD_READ_FAILED", message)
	}
	return output, nil
}

func (b *Backend) firewalldRules(ctx context.Context) ([]Rule, *protocol.Reply) {
	rich, failure := b.runFirewalld(ctx, "--list-rich-rules")
	if failure != nil {
		return nil, failure
	}
	ports, failure := b.runFirewalld(ctx, "--list-ports")
	if failure != nil {
		return nil, failure
	}
	lines := []string{}
	for _, output := range []string{rich, ports} {
		for _, raw := range strings.Split(output, "\n") {
			if line := strings.TrimSpace(raw); line != "" {
				lines = append(lines, line)
			}
		}
	}
	sort.Strings(lines)
	rules := make([]Rule, 0, len(lines))
	for _, line := range lines {
		rule, ok := parseFirewalldRule(line)
		if !ok {
			continue
		}
		rule.ID = len(rules) + 1
		rule.line = line
		rules = append(rules, rule)
	}
	return rules, nil
}

func parseFirewalldRule(line string) (Rule, bool) {
	rule := Rule{Action: "allow", Direction: "in", Protocol: "any", Source: "any", Destination: "any", Family: "ipv4"}
	if !strings.HasPrefix(line, "rule ") {
		port, protocol, ok := strings.Cut(line, "/")
		if !ok || (protocol != "tcp" && protocol != "udp") {
			return Rule{}, false
		}
		rule.Protocol, rule.Ports = protocol, []string{strings.ReplaceAll(port, "-", ":")}
		return rule, true
	}
	attributes := map[string]string{}
	for _, match := range richAttribute.FindAllStringSubmatch(line, -1) {
		attributes[match[1]] = match[2]
	}
	if attributes["family"] == "ipv6" {
		rule.Family = "ipv6"
	}
	if source := attributes["address"]; source != "" {
		rule.Source = source
		if prefix, err := netip.ParsePrefix(source); err == nil && prefix.Addr().Is6() {
			rule.Family = "ipv6"
		}
	}
	if protocol := attributes["protocol"]; protocol == "tcp" || protocol == "udp" {
		rule.Protocol = protocol
	}
	if port := attributes["port"]; port != "" {
		rule.Ports = []string{strings.ReplaceAll(port, "-", ":")}
	}
	switch {
	case strings.HasSuffix(line, " accept"):
		rule.Action = "allow"
	case strings.HasSuffix(line, " drop"):
		rule.Action = "deny"
	case strings.HasSuffix(line, " reject"):
		rule.Action = "reject"
	default:
		return Rule{}, false
	}
	return rule, len(rule.Ports) != 0
}

func (b *Backend) executeFirewalld(ctx context.Context, command string, args []string) (map[string]any, *protocol.Reply) {
	switch command {
	case "info", "status", "rules":
		state, _, active := b.runner.Run(ctx, b.firewalld, "--state")
		active = active && strings.TrimSpace(state) == "running"
		rules, failure := b.firewalldRules(ctx)
		if failure != nil {
			return nil, failure
		}
		status := map[string]any{"active": active, "logging": "unknown", "default_incoming": "unknown", "default_outgoing": "unknown", "default_routed": "unknown"}
		if command == "rules" {
			return map[string]any{"rules": rules, "count": len(rules)}, nil
		}
		if command == "status" {
			status["rule_count"] = len(rules)
			return status, nil
		}
		version, _ := b.runFirewalld(ctx, "--version")
		return map[string]any{"backend": "firewalld", "product": "firewalld", "installed": true, "version": nullableText(version), "active": active, "ipv6": true, "default_incoming": "unknown", "default_outgoing": "unknown", "default_routed": "unknown"}, nil
	case "add", "add-execute":
		if failure := validateAdd(args); failure != nil {
			return nil, failure
		}
		if args[0] == "limit" {
			return nil, reply(2, "UNSUPPORTED_FIREWALL_ACTION", "L’action limit n’est pas prise en charge par firewalld.")
		}
		family := "ipv4"
		if args[3] != "any" {
			if address, err := netip.ParsePrefix(args[3]); err == nil && address.Addr().Is6() {
				family = "ipv6"
			} else if ip, err := netip.ParseAddr(args[3]); err == nil && ip.Is6() {
				family = "ipv6"
			}
		}
		richRules := []string{}
		for _, port := range strings.Split(args[1], ",") {
			action := map[string]string{"allow": "accept", "deny": "drop", "reject": "reject"}[args[0]]
			rich := `rule family="` + family + `"`
			if args[3] != "any" {
				rich += ` source address="` + args[3] + `"`
			}
			rich += ` port port="` + strings.ReplaceAll(strings.TrimSpace(port), ":", "-") + `" protocol="` + args[2] + `" ` + action
			richRules = append(richRules, rich)
		}
		if command == "add" {
			return b.scheduleFirewalld(ctx, "add", goCLI, "firewall", "add-execute", args[0], args[1], args[2], args[3])
		}
		for _, rich := range richRules {
			if _, _, ok := b.runner.Run(ctx, b.firewalld, "--permanent", "--add-rich-rule="+rich); !ok {
				return nil, reply(10, "FIREWALL_ADD_FAILED", "L’ajout de la règle firewalld a échoué.")
			}
		}
		if _, _, ok := b.runner.Run(ctx, b.firewalld, "--reload"); !ok {
			return nil, reply(10, "FIREWALL_RELOAD_FAILED", "Le rechargement de firewalld a échoué.")
		}
		return map[string]any{"action": "add", "backend": "firewalld", "result": "completed"}, nil
	case "enable":
		return b.scheduleFirewalld(ctx, "enable", "/usr/bin/systemctl", "enable", "--now", "firewalld.service")
	case "disable":
		return b.scheduleFirewalld(ctx, "disable", "/usr/bin/systemctl", "disable", "--now", "firewalld.service")
	case "reload":
		return b.scheduleFirewalld(ctx, "reload", b.firewalld, "--reload")
	case "delete-check":
		id, failure := validID(args[0])
		if failure != nil {
			return nil, failure
		}
		return b.usage(ctx, id)
	case "delete", "delete-execute":
		return b.delete(ctx, command, args)
	}
	return nil, nil
}

func (b *Backend) scheduleFirewalld(ctx context.Context, action, command string, args ...string) (map[string]any, *protocol.Reply) {
	all := []string{"--quiet", "--collect", "--on-active=5s", command}
	all = append(all, args...)
	_, _, ok := b.runner.Run(ctx, "/usr/bin/systemd-run", all...)
	if !ok {
		return nil, reply(10, "FIREWALLD_ACTION_SCHEDULE_FAILED", "La programmation de l’action firewalld a échoué.")
	}
	return map[string]any{"action": action, "backend": "firewalld", "result": "scheduled", "delay_seconds": 5}, nil
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}
