package webdashboard

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"aegisadmin/backend/internal/protocol"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}

type Card struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Value    string `json:"value"`
	Subtitle string `json:"subtitle"`
	Status   string `json:"status"`
	URL      string `json:"url,omitempty"`
}

type Information struct {
	Label string
	Value string
}

type Process struct {
	PID, User, State, StateLabel, Status, CPU, MemoryPercent, Memory, Elapsed, Name string
	PIDValue, MemoryBytes, ElapsedSeconds                                           int64
	CPUPercentValue, MemoryPercentValue                                             float64
}

type ProcessSnapshot struct {
	Total, Returned, Limit int64
	Truncated              bool
	Processes              []Process
}

type Snapshot struct {
	Hostname    string
	System      string
	Kernel      string
	Uptime      string
	Information []Information
	Cards       []Card
}

type Collector struct {
	backend Backend
}

func New(backend Backend) *Collector {
	return &Collector{backend: backend}
}

func (c *Collector) Resources(ctx context.Context) ([]Card, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "system", Command: "info"})
	item := struct {
		domain string
		data   map[string]any
		ok     bool
	}{domain: "system"}
	if err == nil && reply.Response.Success && reply.Response.Data != nil {
		item.data = *reply.Response.Data
		item.ok = true
	}
	return systemSnapshot(item).Cards, nil
}

func (c *Collector) Supervision(ctx context.Context) ([]Card, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results := c.collect(ctx, []protocol.Request{
		{Domain: "storage", Command: "list"},
		{Domain: "services", Command: "list"},
		{Domain: "network", Command: "list"},
	})
	return []Card{storageCard(results["storage"]), servicesCard(results["services"]), networkCard(results["network"])}, nil
}

func (c *Collector) Processes(ctx context.Context) (ProcessSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ProcessSnapshot{}, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "system", Command: "processes"})
	if err != nil || !reply.Response.Success || reply.Response.Data == nil {
		return ProcessSnapshot{}, fmt.Errorf("process listing unavailable")
	}
	data := *reply.Response.Data
	snapshot := ProcessSnapshot{Total: integer(data["total"]), Returned: integer(data["returned_count"]), Limit: integer(data["limit"]), Truncated: boolean(data["truncated"])}
	for _, raw := range array(data["processes"]) {
		item := object(raw)
		state := text(item["state"], "?")
		label, status := processState(state)
		snapshot.Processes = append(snapshot.Processes, Process{
			PID: formatInteger(integer(item["pid"])), PIDValue: integer(item["pid"]), User: text(item["user"], "—"), State: state,
			StateLabel: label, Status: status, CPU: formatPercent(number(item["cpu_percent"])),
			CPUPercentValue: number(item["cpu_percent"]), MemoryPercent: formatPercent(number(item["memory_percent"])), MemoryPercentValue: number(item["memory_percent"]),
			Memory: formatBytes(integer(item["memory_bytes"])), MemoryBytes: integer(item["memory_bytes"]),
			Elapsed: formatProcessDuration(integer(item["elapsed_seconds"])), ElapsedSeconds: integer(item["elapsed_seconds"]), Name: text(item["name"], "—"),
		})
	}
	return snapshot, nil
}

func processState(state string) (string, string) {
	if state == "" {
		return "Inconnu", "neutral"
	}
	switch state[0] {
	case 'R':
		return "En cours", "success"
	case 'S':
		return "En veille", "neutral"
	case 'D':
		return "Attente disque", "warning"
	case 'T', 't':
		return "Arrêté", "warning"
	case 'Z':
		return "Zombie", "danger"
	case 'X', 'x':
		return "Mort", "danger"
	case 'K':
		return "Wakekill", "warning"
	case 'I':
		return "Inactif", "neutral"
	case 'P':
		return "Stationné", "neutral"
	case 'W':
		return "Pagination", "neutral"
	default:
		return "Inconnu", "neutral"
	}
}

func formatProcessDuration(seconds int64) string {
	if seconds <= 0 {
		return "0 s"
	}
	units := []struct {
		seconds int64
		label   string
	}{{86400, "j"}, {3600, "h"}, {60, "min"}, {1, "s"}}
	parts := make([]string, 0, 3)
	for _, unit := range units {
		value := seconds / unit.seconds
		if value == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", value, unit.label))
		seconds %= unit.seconds
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func (c *Collector) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	requests := []protocol.Request{
		{Domain: "system", Command: "info"},
		{Domain: "storage", Command: "list"},
		{Domain: "services", Command: "list"},
		{Domain: "network", Command: "list"},
	}
	byDomain := c.collect(ctx, requests)
	snapshot := systemSnapshot(byDomain["system"])
	snapshot.Cards = append(snapshot.Cards,
		storageCard(byDomain["storage"]),
		servicesCard(byDomain["services"]),
		networkCard(byDomain["network"]),
	)
	return snapshot, nil
}

type collectionResult struct {
	domain string
	data   map[string]any
	ok     bool
}

func (c *Collector) collect(ctx context.Context, requests []protocol.Request) map[string]collectionResult {
	results := make(chan collectionResult, len(requests))
	var workers sync.WaitGroup
	for _, request := range requests {
		request := request
		workers.Add(1)
		go func() {
			defer workers.Done()
			if ctx.Err() != nil {
				results <- collectionResult{domain: request.Domain}
				return
			}
			reply, err := c.backend.Execute(request)
			if err != nil || !reply.Response.Success || reply.Response.Data == nil {
				results <- collectionResult{domain: request.Domain}
				return
			}
			results <- collectionResult{domain: request.Domain, data: *reply.Response.Data, ok: true}
		}()
	}
	workers.Wait()
	close(results)
	byDomain := make(map[string]collectionResult, len(requests))
	for item := range results {
		byDomain[item.domain] = item
	}
	return byDomain
}

func systemSnapshot(item collectionResult) Snapshot {
	snapshot := Snapshot{Hostname: "Serveur indisponible", System: "Informations système indisponibles", Kernel: "—", Uptime: "—"}
	unavailable := Card{ID: "cpu", Title: "Processeur", Value: "Indisponible", Subtitle: "Lecture impossible", Status: "neutral"}
	snapshot.Cards = []Card{unavailable, {ID: "memory", Title: "Mémoire", Value: "Indisponible", Subtitle: "Lecture impossible", Status: "neutral"}}
	if !item.ok {
		return snapshot
	}
	host := object(item.data["host"])
	cpu := object(item.data["cpu"])
	memory := object(item.data["memory"])
	temperature := object(item.data["temperature"])
	processes := object(item.data["processes"])
	snapshot.Hostname = text(host["hostname"], snapshot.Hostname)
	snapshot.System = text(host["pretty_name"], snapshot.System)
	snapshot.Kernel = text(host["kernel"], "—") + " · " + text(host["architecture"], "—")
	snapshot.Uptime = formatDuration(integer(host["uptime_seconds"]))
	version := text(host["version"], text(host["version_id"], "Non renseignée"))
	snapshot.Information = []Information{
		{Label: "Nom d’hôte", Value: snapshot.Hostname},
		{Label: "Système d’exploitation", Value: text(host["operating_system"], "Linux")},
		{Label: "Distribution", Value: text(host["distribution"], snapshot.System)},
		{Label: "Version", Value: version},
		{Label: "Noyau", Value: text(host["kernel"], "—")},
		{Label: "Architecture", Value: text(host["architecture"], "—") + " · " + text(cpu["model"], "Modèle inconnu")},
		{Label: "Durée de fonctionnement", Value: snapshot.Uptime},
	}
	cpuPercent := number(cpu["usage_percent"])
	memoryPercent := number(memory["percent"])
	snapshot.Cards[0] = Card{
		ID:    "cpu",
		Title: "Processeur", Value: formatPercent(cpuPercent),
		Subtitle: fmt.Sprintf("%d cœur(s) · Charge : %s", integer(cpu["cores"]), formatLoadAverage(cpu["load_average"])),
		Status:   usageStatus(cpuPercent),
	}
	snapshot.Cards[1] = Card{
		ID:    "memory",
		Title: "Mémoire", Value: formatPercent(memoryPercent),
		Subtitle: formatBytes(integer(memory["available_bytes"])) + " disponibles · " + formatBytes(integer(memory["used_bytes"])) + " utilisés sur " + formatBytes(integer(memory["total_bytes"])),
		Status:   usageStatus(memoryPercent),
	}
	snapshot.Cards = append(snapshot.Cards, temperatureCard(temperature), Card{
		ID: "processes", Title: "Processus", Value: formatInteger(integer(processes["total"])), URL: "/processes",
		Subtitle: "Processus actuellement détectés", Status: "neutral",
	})
	return snapshot
}

func temperatureCard(temperature map[string]any) Card {
	if !boolean(temperature["available"]) {
		return Card{ID: "temperature", Title: "Température CPU", Value: "Indisponible", Subtitle: "Aucun capteur CPU disponible", Status: "neutral"}
	}
	celsius := number(temperature["celsius"])
	status := "success"
	if celsius >= 85 {
		status = "danger"
	} else if celsius >= 70 {
		status = "warning"
	}
	return Card{ID: "temperature", Title: "Température CPU", Value: strings.ReplaceAll(strconv.FormatFloat(celsius, 'f', 1, 64), ".", ",") + " °C", Subtitle: "Capteur : " + text(temperature["sensor"], "CPU"), Status: status}
}

func formatLoadAverage(value any) string {
	values := array(value)
	if len(values) < 3 {
		return "indisponible"
	}
	parts := make([]string, 3)
	labels := []string{"1 min", "5 min", "15 min"}
	for index := range parts {
		parts[index] = strings.ReplaceAll(strconv.FormatFloat(number(values[index]), 'f', 2, 64), ".", ",") + " à " + labels[index]
	}
	return strings.Join(parts, " / ")
}

func formatInteger(value int64) string {
	raw := strconv.FormatInt(value, 10)
	for index := len(raw) - 3; index > 0; index -= 3 {
		raw = raw[:index] + " " + raw[index:]
	}
	return raw
}

func storageCard(item collectionResult) Card {
	if !item.ok {
		return unavailableCard("Stockage")
	}
	mounts := array(item.data["mounts"])
	maximum := 0.0
	for _, raw := range mounts {
		maximum = math.Max(maximum, number(object(raw)["percent"]))
	}
	return Card{
		ID: "storage", Title: "Stockage", Value: fmt.Sprintf("%d volume(s)", len(mounts)), URL: "/storage",
		Subtitle: "Occupation maximale : " + formatPercent(maximum), Status: usageStatus(maximum),
	}
}

func servicesCard(item collectionResult) Card {
	if !item.ok {
		return unavailableCard("Services")
	}
	services := array(item.data["services"])
	installed, active, failed := 0, 0, 0
	for _, raw := range services {
		service := object(raw)
		if boolean(service["exists"]) {
			installed++
		}
		if boolean(service["active"]) {
			active++
		}
		if text(service["state"], "") == "failed" {
			failed++
		}
	}
	status := "success"
	if failed > 0 {
		status = "danger"
	} else if active < installed {
		status = "warning"
	}
	return Card{ID: "services", Title: "Services", Value: fmt.Sprintf("%d / %d actifs", active, installed), Subtitle: fmt.Sprintf("%d service(s) surveillé(s)", len(services)), Status: status, URL: "/services"}
}

func networkCard(item collectionResult) Card {
	if !item.ok {
		return unavailableCard("Réseau")
	}
	interfaces := array(item.data["interfaces"])
	up, down := 0, 0
	for _, raw := range interfaces {
		switch text(object(raw)["state"], "unknown") {
		case "up":
			up++
		case "down":
			down++
		}
	}
	status := "success"
	if down > 0 {
		status = "warning"
	}
	return Card{ID: "network", Title: "Réseau", Value: fmt.Sprintf("%d / %d actives", up, len(interfaces)), Subtitle: fmt.Sprintf("%d interface(s) arrêtée(s)", down), Status: status, URL: "/network"}
}

func unavailableCard(title string) Card {
	id := strings.ToLower(strings.ReplaceAll(title, "é", "e"))
	urls := map[string]string{"stockage": "/storage", "services": "/services", "reseau": "/network"}
	return Card{ID: id, Title: title, Value: "Indisponible", Subtitle: "Lecture impossible", Status: "neutral", URL: urls[id]}
}

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func array(value any) []any {
	result, _ := value.([]any)
	return result
}

func text(value any, fallback string) string {
	result, ok := value.(string)
	if !ok || result == "" {
		return fallback
	}
	return result
}

func boolean(value any) bool {
	result, _ := value.(bool)
	return result
}

func number(value any) float64 {
	switch result := value.(type) {
	case float64:
		return result
	case int:
		return float64(result)
	case int64:
		return float64(result)
	default:
		return 0
	}
}

func integer(value any) int64 {
	return int64(number(value))
}

func formatPercent(value float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(value, 'f', 1, 64), ".", ",") + " %"
}

func usageStatus(percent float64) string {
	if percent >= 90 {
		return "danger"
	}
	if percent >= 80 {
		return "warning"
	}
	return "success"
}

func formatBytes(value int64) string {
	units := []string{"o", "Kio", "Mio", "Gio", "Tio", "Pio"}
	amount, unit := float64(value), 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	return strings.ReplaceAll(strconv.FormatFloat(amount, 'f', 1, 64), ".", ",") + " " + units[unit]
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	hours := seconds % 86400 / 3600
	minutes := seconds % 3600 / 60
	if days > 0 {
		return fmt.Sprintf("%d j %d h %d min", days, hours, minutes)
	}
	return fmt.Sprintf("%d h %d min", hours, minutes)
}
