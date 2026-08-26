package services

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const systemctlCommand = "/usr/bin/systemctl"
const allowedServicesFile = "/etc/aegisadmin-system/services"

var defaultAllowedServices = []string{"apache2", "httpd", "mysql", "mariadb", "mysqld", "fail2ban", "tor", "tor@default", "cron", "crond", "ssh", "sshd"}

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9@._-]*$`)
var phpFPMUnitPattern = regexp.MustCompile(`^php[0-9]+\.[0-9]+-fpm\.service$`)

type Service struct {
	ID      string `json:"id"`
	Exists  bool   `json:"exists"`
	Active  bool   `json:"active"`
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
}

type Collector interface {
	List(context.Context) []Service
	Status(context.Context, string) Service
	Restart(context.Context, string) error
}

type commandRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type LinuxCollector struct{ runner commandRunner }

type execRunner struct{}

type Handler struct {
	collector Collector
	allowed   func(string) bool
}

func New(collector Collector) *Handler {
	return &Handler{collector: collector, allowed: serviceAllowed}
}

func NewLinuxCollector() *LinuxCollector {
	return &LinuxCollector{runner: execRunner{}}
}

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine services.")
	}

	switch command {
	case "list":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		items := h.collector.List(ctx)
		return success(map[string]any{"services": items})

	case "status":
		if len(arguments) != 1 {
			return invalidArgumentCount()
		}
		if reply := h.validateService(arguments[0]); reply != nil {
			return *reply
		}
		return success(map[string]any{"service": h.collector.Status(ctx, arguments[0])})

	case "restart":
		if len(arguments) != 1 {
			return invalidArgumentCount()
		}
		if reply := h.validateService(arguments[0]); reply != nil {
			return *reply
		}
		service := h.collector.Status(ctx, arguments[0])
		if !service.Exists {
			return failure(5, "SERVICE_NOT_FOUND", "Le service demandé est introuvable.")
		}
		if err := h.collector.Restart(ctx, arguments[0]); err != nil {
			return failure(10, "SERVICE_RESTART_FAILED", "Le redémarrage du service a échoué.")
		}
		return success(map[string]any{
			"service": arguments[0],
			"action":  "restart",
			"result":  "success",
		})

	default:
		return failure(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine services.")
	}
}

func (c *LinuxCollector) Status(ctx context.Context, service string) Service {
	statuses := c.statuses(ctx, []string{service})
	return statuses[0]
}

func (c *LinuxCollector) List(ctx context.Context) []Service {
	return c.statuses(ctx, c.allowedServices(ctx))
}

func (c *LinuxCollector) allowedServices(ctx context.Context) []string {
	services := configuredServices(allowedServicesFile)
	output, _ := c.runner.Run(ctx, "list-unit-files", "--type=service", "--no-legend", "--no-pager", "php*-fpm.service")
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 0 && phpFPMUnitPattern.MatchString(fields[0]) {
			services = append(services, strings.TrimSuffix(fields[0], ".service"))
		}
	}
	return uniqueSorted(services)
}

func (c *LinuxCollector) statuses(ctx context.Context, services []string) []Service {
	results := make([]Service, len(services))
	byUnit := make(map[string]int, len(services))
	arguments := []string{"show", "--property=Id", "--property=Names", "--property=LoadState", "--property=ActiveState", "--property=UnitFileState", "--property=SubState", "--"}
	for index, service := range services {
		results[index] = Service{ID: service, State: "not-found"}
		unit := service + ".service"
		byUnit[unit] = index
		arguments = append(arguments, unit)
	}
	output, err := c.runner.Run(ctx, arguments...)
	if err != nil && len(output) == 0 {
		return results
	}
	for _, properties := range parseSystemctlShow(string(output)) {
		index, exists := byUnit[properties["Id"]]
		if !exists {
			for _, name := range strings.Fields(properties["Names"]) {
				if candidate, found := byUnit[name]; found {
					index, exists = candidate, true
					break
				}
			}
		}
		if !exists {
			continue
		}
		loadState := properties["LoadState"]
		if loadState != "loaded" && loadState != "masked" {
			continue
		}
		state := properties["SubState"]
		if state == "" {
			state = "unknown"
		}
		results[index] = Service{ID: services[index], Exists: true, Active: properties["ActiveState"] == "active", Enabled: properties["UnitFileState"] == "enabled", State: state}
	}
	return results
}

func (c *LinuxCollector) Restart(ctx context.Context, service string) error {
	_, err := c.runner.Run(ctx, "restart", "--", service+".service")
	return err
}

func parseSystemctlShow(output string) []map[string]string {
	items := []map[string]string{}
	current := map[string]string{}
	flush := func() {
		if len(current) != 0 {
			items = append(items, current)
			current = map[string]string{}
		}
	}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found {
			if key == "Id" && current["Id"] != "" {
				flush()
			}
			current[key] = value
		}
	}
	flush()
	return items
}

func (execRunner) Run(ctx context.Context, arguments ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(commandCtx, systemctlCommand, arguments...).Output()
}

func (h *Handler) validateService(service string) *protocol.Reply {
	if !identifierPattern.MatchString(service) {
		reply := failure(2, "INVALID_SERVICE_IDENTIFIER", "L’identifiant du service est invalide.")
		return &reply
	}
	if h.allowed(service) {
		return nil
	}
	reply := failure(6, "SERVICE_NOT_ALLOWED", "Le service demandé n’est pas autorisé.")
	return &reply
}

func serviceAllowed(service string) bool {
	if phpFPMUnitPattern.MatchString(service + ".service") {
		return true
	}
	for _, allowed := range configuredServices(allowedServicesFile) {
		if service == allowed {
			return true
		}
	}
	return false
}

func configuredServices(path string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return append([]string(nil), defaultAllowedServices...)
	}
	services := []string{}
	for _, raw := range strings.Split(string(content), "\n") {
		value := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if identifierPattern.MatchString(value) {
			services = append(services, value)
		}
	}
	return uniqueSorted(services)
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func success(data map[string]any) protocol.Reply { return protocol.Reply{Response: api.Success(data)} }
func failure(exit int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exit, Response: api.Failure(code, message)}
}
func invalidArgumentCount() protocol.Reply {
	return failure(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
}
