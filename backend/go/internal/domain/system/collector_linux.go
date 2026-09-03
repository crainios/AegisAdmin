package system

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"aegisadmin/backend/internal/buildinfo"
)

const (
	processLimit     = 500
	psCommand        = "/usr/bin/ps"
	systemctlCommand = "/usr/bin/systemctl"
	dpkgQueryCommand = "/usr/bin/dpkg-query"
)

var backendVersion = buildinfo.Version

type LinuxCollector struct{ cpu *cpuSampler }

type cpuSampler struct {
	mu       sync.RWMutex
	path     string
	previous cpuTimes
	hasValue bool
	usage    float64
	err      error
}

type cpuTimes struct {
	total uint64
	idle  uint64
}

type temperatureCandidate struct {
	priority int
	path     string
	sensor   string
	celsius  float64
}

type process struct {
	PID            int     `json:"pid"`
	User           string  `json:"user"`
	State          string  `json:"state"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	MemoryBytes    int64   `json:"memory_bytes"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
	Name           string  `json:"name"`
	Description    string  `json:"description,omitempty"`
}

func NewLinuxCollector(ctx context.Context) *LinuxCollector {
	return &LinuxCollector{cpu: newCPUSampler(ctx, "/proc/stat", 200*time.Millisecond, time.Second)}
}

func (c *LinuxCollector) Info(ctx context.Context) (map[string]any, error) {
	osRelease, err := readOSRelease("/etc/os-release")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	hostname = safeText(hostname, "localhost")

	kernel, architecture, err := uname()
	if err != nil {
		return nil, err
	}

	uptime, err := readUptime("/proc/uptime")
	if err != nil {
		return nil, err
	}

	loadAverage, err := readLoadAverage("/proc/loadavg")
	if err != nil {
		return nil, err
	}

	model := readCPUModel("/proc/cpuinfo")
	usage, err := c.cpuUsage(ctx)
	if err != nil {
		return nil, err
	}

	memory, err := readMemory("/proc/meminfo")
	if err != nil {
		return nil, err
	}

	processTotal, err := countProcesses("/proc")
	if err != nil {
		return nil, err
	}

	distribution := safeText(osRelease["NAME"], "Linux")
	version := safeText(osRelease["VERSION"], osRelease["VERSION_ID"])
	prettyName := safeText(osRelease["PRETTY_NAME"], strings.TrimSpace(distribution+" "+version))

	return map[string]any{
		"backend":  backendVersion,
		"features": []string{"system"},
		"host": map[string]any{
			"hostname":         hostname,
			"operating_system": "Linux",
			"distribution":     distribution,
			"distribution_id":  safeText(osRelease["ID"], ""),
			"version":          version,
			"version_id":       safeText(osRelease["VERSION_ID"], ""),
			"pretty_name":      prettyName,
			"kernel":           kernel,
			"architecture":     architecture,
			"uptime_seconds":   uptime,
		},
		"cpu": map[string]any{
			"model":         model,
			"usage_percent": usage,
			"load_average":  loadAverage,
			"cores":         max(runtime.NumCPU(), 1),
		},
		"memory":      memory,
		"temperature": readTemperature(),
		"processes": map[string]any{
			"total": processTotal,
		},
	}, nil
}

func (c *LinuxCollector) cpuUsage(ctx context.Context) (float64, error) {
	if c.cpu == nil {
		return readCPUUsage(ctx, "/proc/stat")
	}
	return c.cpu.value()
}

func (c *LinuxCollector) Processes(ctx context.Context) (map[string]any, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	command := exec.CommandContext(
		commandCtx,
		psCommand,
		"-ww",
		"-eo",
		"pid=,user=,stat=,pcpu=,pmem=,rss=,etimes=,comm=",
		"--sort=-pcpu,-pmem,pid",
	)
	command.Env = append(os.Environ(), "LC_ALL=C")

	output, err := command.Output()
	if err != nil {
		return nil, err
	}

	processes, err := parseProcesses(string(output), os.Getpid())
	if err != nil {
		return nil, err
	}
	enrichProcessDescriptions(commandCtx, processes, "/proc")

	total := len(processes)
	returned := total
	if returned > processLimit {
		returned = processLimit
	}

	return map[string]any{
		"processes":      processes[:returned],
		"total":          total,
		"returned_count": returned,
		"limit":          processLimit,
		"truncated":      total > returned,
	}, nil
}

func readCPUUsage(ctx context.Context, path string) (float64, error) {
	first, err := readCPUTimes(path)
	if err != nil {
		return 0, err
	}

	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
	}

	second, err := readCPUTimes(path)
	if err != nil {
		return 0, err
	}
	return cpuUsageBetween(first, second), nil
}

func newCPUSampler(ctx context.Context, path string, initialDelay, interval time.Duration) *cpuSampler {
	sampler := &cpuSampler{path: path, err: errors.New("CPU statistics are not available")}
	first, err := readCPUTimes(path)
	if err == nil {
		sampler.previous = first
		sampler.hasValue = true
		timer := time.NewTimer(initialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			sampler.err = ctx.Err()
		case <-timer.C:
			sampler.sample()
		}
	} else {
		sampler.err = err
	}
	go sampler.run(ctx, interval)
	return sampler
}

func (s *cpuSampler) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sample()
		}
	}
}

func (s *cpuSampler) sample() {
	next, err := readCPUTimes(s.path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.err = err
		return
	}
	if s.hasValue {
		s.usage = cpuUsageBetween(s.previous, next)
		s.err = nil
	}
	s.previous = next
	s.hasValue = true
}

func (s *cpuSampler) value() (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usage, s.err
}

func cpuUsageBetween(first, second cpuTimes) float64 {
	if second.total <= first.total {
		return 0
	}
	totalDelta := second.total - first.total
	idleDelta := second.idle - first.idle
	busyDelta := uint64(0)
	if idleDelta < totalDelta {
		busyDelta = totalDelta - idleDelta
	}
	usage := float64(busyDelta) / float64(totalDelta) * 100
	return roundOne(clamp(usage, 0, 100))
}

func readCPUTimes(path string) (cpuTimes, error) {
	file, err := os.Open(path)
	if err != nil {
		return cpuTimes{}, err
	}
	defer file.Close()

	var label string
	values := make([]uint64, 8)
	_, err = fmt.Fscan(file, &label, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6], &values[7])
	if err != nil || label != "cpu" {
		return cpuTimes{}, errors.New("invalid CPU statistics")
	}

	var total uint64
	for _, value := range values {
		total += value
	}
	return cpuTimes{total: total, idle: values[3] + values[4]}, nil
}

func readCPUModel(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "Processeur non renseigné"
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if (key == "model name" || key == "hardware" || key == "processor") && value != "" {
			if _, numeric := strconv.Atoi(value); numeric == nil {
				continue
			}
			if _, exists := values[key]; !exists {
				values[key] = value
			}
		}
	}

	for _, key := range []string{"model name", "hardware", "processor"} {
		if values[key] != "" {
			return values[key]
		}
	}
	return "Processeur non renseigné"
}

func readMemory(path string) (map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, raw, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		value, parseErr := strconv.ParseInt(fields[0], 10, 64)
		if parseErr == nil {
			values[key] = value * 1024
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	total := max(values["MemTotal"], 0)
	available, exists := values["MemAvailable"]
	if !exists {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	available = clamp(available, int64(0), total)
	used := max(total-available, int64(0))
	cached := values["Buffers"] + values["Cached"] + values["SReclaimable"] - values["Shmem"]
	cached = clamp(cached, int64(0), total)
	percent := 0.0
	if total > 0 {
		percent = roundOne(float64(used) / float64(total) * 100)
	}

	return map[string]any{
		"total_bytes":     total,
		"used_bytes":      used,
		"available_bytes": available,
		"cached_bytes":    cached,
		"percent":         percent,
	}, nil
}

func readUptime(path string) (int64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0, errors.New("invalid uptime")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("invalid uptime")
	}
	return max(int64(math.Floor(value)), 0), nil
}

func readLoadAverage(path string) ([]float64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(content))
	if len(fields) < 3 {
		return nil, errors.New("invalid load average")
	}

	result := make([]float64, 3)
	for index := range result {
		value, parseErr := strconv.ParseFloat(fields[index], 64)
		if parseErr != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("invalid load average")
		}
		result[index] = math.Round(value*100) / 100
	}
	return result, nil
}

func readOSRelease(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, unquoteErr := strconv.Unquote(value); unquoteErr == nil {
			value = unquoted
		}
		values[strings.ToUpper(strings.TrimSpace(key))] = value
	}
	return values, scanner.Err()
}

func countProcesses(path string) (int, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err == nil {
			total++
		}
	}
	return total, nil
}

func enrichProcessDescriptions(ctx context.Context, processes []process, procRoot string) {
	unitsByPID := make(map[int]string)
	unitSet := make(map[string]struct{})
	for _, item := range processes {
		unit := readProcessUnit(filepath.Join(procRoot, strconv.Itoa(item.PID), "cgroup"))
		if unit == "" {
			continue
		}
		unitsByPID[item.PID] = unit
		unitSet[unit] = struct{}{}
	}
	unitDescriptions := systemdUnitDescriptions(ctx, sortedKeys(unitSet))

	executablesByPID := make(map[int]string)
	executableSet := make(map[string]struct{})
	for index := range processes {
		if description := unitDescriptions[unitsByPID[processes[index].PID]]; description != "" {
			processes[index].Description = description
			continue
		}
		executable, err := os.Readlink(filepath.Join(procRoot, strconv.Itoa(processes[index].PID), "exe"))
		if err != nil {
			continue
		}
		executable = strings.TrimSuffix(executable, " (deleted)")
		if !filepath.IsAbs(executable) {
			continue
		}
		executablesByPID[processes[index].PID] = executable
		executableSet[executable] = struct{}{}
	}
	packageByExecutable := debianPackageOwners(ctx, sortedKeys(executableSet))
	packageSet := make(map[string]struct{})
	for _, packageName := range packageByExecutable {
		packageSet[packageName] = struct{}{}
	}
	packageDescriptions := debianPackageDescriptions(ctx, sortedKeys(packageSet))

	for index := range processes {
		if processes[index].Description != "" {
			continue
		}
		packageName := packageByExecutable[executablesByPID[processes[index].PID]]
		if description := packageDescriptions[packageName]; description != "" {
			processes[index].Description = description
			continue
		}
		processes[index].Description = knownProcessDescription(processes[index].Name)
	}
}

func readProcessUnit(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		for _, component := range strings.Split(line, "/") {
			if strings.HasSuffix(component, ".service") && validSystemdUnit(component) {
				return component
			}
		}
	}
	return ""
}

func validSystemdUnit(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("_.@:-\\x", character) {
			return false
		}
	}
	return true
}

func systemdUnitDescriptions(ctx context.Context, units []string) map[string]string {
	result := make(map[string]string)
	if len(units) == 0 {
		return result
	}
	arguments := []string{"show", "--property=Id", "--property=Description", "--"}
	arguments = append(arguments, units...)
	command := exec.CommandContext(ctx, systemctlCommand, arguments...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return result
	}
	return parseSystemdDescriptions(string(output))
}

func parseSystemdDescriptions(output string) map[string]string {
	result := make(map[string]string)
	for _, block := range strings.Split(strings.TrimSpace(output), "\n\n") {
		properties := make(map[string]string)
		for _, line := range strings.Split(block, "\n") {
			key, value, found := strings.Cut(line, "=")
			if found {
				properties[key] = strings.TrimSpace(value)
			}
		}
		if id, description := properties["Id"], safeDescription(properties["Description"]); id != "" && description != "" {
			result[id] = description
		}
	}
	return result
}

func debianPackageOwners(ctx context.Context, executables []string) map[string]string {
	result := make(map[string]string)
	if len(executables) == 0 {
		return result
	}
	arguments := append([]string{"-S", "--"}, executables...)
	command := exec.CommandContext(ctx, dpkgQueryCommand, arguments...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.CombinedOutput()
	if err != nil && len(output) == 0 {
		return result
	}
	for _, line := range strings.Split(string(output), "\n") {
		separator := strings.LastIndex(line, ": ")
		if separator < 1 {
			continue
		}
		packageName := strings.TrimSpace(strings.Split(line[:separator], ",")[0])
		executable := strings.TrimSpace(line[separator+2:])
		if packageName != "" && filepath.IsAbs(executable) {
			result[executable] = packageName
		}
	}
	return result
}

func debianPackageDescriptions(ctx context.Context, packages []string) map[string]string {
	result := make(map[string]string)
	if len(packages) == 0 {
		return result
	}
	arguments := []string{"-W", `-f=${binary:Package}\t${binary:Summary}\n`, "--"}
	arguments = append(arguments, packages...)
	command := exec.CommandContext(ctx, dpkgQueryCommand, arguments...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(output), "\n") {
		packageName, description, found := strings.Cut(line, "\t")
		if found && strings.TrimSpace(packageName) != "" {
			result[strings.TrimSpace(packageName)] = safeDescription(description)
		}
	}
	return result
}

func safeDescription(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:240])
	}
	return value
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func knownProcessDescription(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	descriptions := map[string]string{
		"apache2":         "Serveur web Apache et traitement de ses connexions.",
		"cron":            "Planification et lancement des tâches périodiques.",
		"fail2ban-server": "Surveillance des journaux et bannissement des adresses malveillantes.",
		"mariadbd":        "Serveur de bases de données MariaDB.",
		"mysqld":          "Serveur de bases de données MySQL.",
		"nginx":           "Serveur web et proxy inverse Nginx.",
		"postgres":        "Serveur de bases de données PostgreSQL.",
		"redis-server":    "Serveur de cache et de données Redis.",
		"sshd":            "Serveur d’accès distant sécurisé SSH.",
		"tor":             "Routage des connexions et services cachés via le réseau Tor.",
	}
	if description := descriptions[normalized]; description != "" {
		return description
	}
	for prefix, description := range map[string]string{
		"aegisadmin-": "Composant interne d’AegisAdmin.",
		"php-fpm":     "Exécute les applications PHP pour le serveur web.",
		"php8.":       "Exécute les applications PHP pour le serveur web.",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return description
		}
	}
	return ""
}

func parseProcesses(output string, collectorPID int) ([]process, error) {
	processes := make([]process, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 8 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, err
		}
		if pid < 1 || pid == collectorPID {
			continue
		}
		cpu, err := parseNonNegativeFloat(fields[3])
		if err != nil {
			return nil, err
		}
		memory, err := parseNonNegativeFloat(fields[4])
		if err != nil {
			return nil, err
		}
		rss, err := parseNonNegativeInt(fields[5])
		if err != nil {
			return nil, err
		}
		elapsed, err := parseNonNegativeInt(fields[6])
		if err != nil {
			return nil, err
		}
		if fields[2] == "" {
			continue
		}
		processes = append(processes, process{
			PID:            pid,
			User:           safeText(fields[1], "inconnu"),
			State:          fields[2],
			CPUPercent:     cpu,
			MemoryPercent:  memory,
			MemoryBytes:    rss * 1024,
			ElapsedSeconds: elapsed,
			Name:           safeText(fields[7], "Processus inconnu"),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(processes, func(left, right int) bool {
		if processes[left].CPUPercent != processes[right].CPUPercent {
			return processes[left].CPUPercent > processes[right].CPUPercent
		}
		if processes[left].MemoryPercent != processes[right].MemoryPercent {
			return processes[left].MemoryPercent > processes[right].MemoryPercent
		}
		return processes[left].PID < processes[right].PID
	})
	return processes, nil
}

func parseNonNegativeFloat(raw string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("invalid percentage")
	}
	return roundOne(value), nil
}

func parseNonNegativeInt(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid integer")
	}
	return value, nil
}

func readTemperature() map[string]any {
	candidates := make([]temperatureCandidate, 0)
	inputs, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp*_input")
	sort.Strings(inputs)
	for _, input := range inputs {
		prefix := strings.TrimSuffix(input, "_input")
		label := readTrimmed(prefix + "_label")
		name := readTrimmed(filepath.Join(filepath.Dir(input), "name"))
		priority := -1
		switch {
		case strings.EqualFold(label, "package id 0"):
			priority = 0
		case isCPUHwmon(name) && isCPUTemperatureLabel(label):
			priority = 2
		case strings.EqualFold(name, "coretemp") && strings.HasPrefix(strings.ToLower(label), "core "):
			priority = 3
		}
		if priority >= 0 {
			candidates = addTemperature(candidates, priority, input, safeText(label, safeText(name, "CPU")))
		}
	}

	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	sort.Strings(zones)
	for _, zone := range zones {
		sensor := readTrimmed(filepath.Join(zone, "type"))
		if strings.EqualFold(sensor, "x86_pkg_temp") {
			candidates = addTemperature(candidates, 1, filepath.Join(zone, "temp"), sensor)
		}
	}

	if len(candidates) == 0 {
		return map[string]any{"available": false, "celsius": nil, "sensor": ""}
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].priority != candidates[right].priority {
			return candidates[left].priority < candidates[right].priority
		}
		if candidates[left].priority == 3 {
			return candidates[left].celsius > candidates[right].celsius
		}
		return candidates[left].path < candidates[right].path
	})
	selected := candidates[0]
	return map[string]any{"available": true, "celsius": selected.celsius, "sensor": selected.sensor}
}

func addTemperature(candidates []temperatureCandidate, priority int, path string, sensor string) []temperatureCandidate {
	raw := readTrimmed(path)
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < -50000 || value > 200000 {
		return candidates
	}
	return append(candidates, temperatureCandidate{
		priority: priority,
		path:     path,
		sensor:   sensor,
		celsius:  roundOne(float64(value) / 1000),
	})
}

func isCPUHwmon(name string) bool {
	name = strings.ToLower(name)
	return name == "coretemp" || name == "k10temp" || name == "zenpower"
}

func isCPUTemperatureLabel(label string) bool {
	label = strings.ToLower(label)
	return label == "tctl" || label == "tdie" || label == "cpu"
}

func readTrimmed(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func uname() (string, string, error) {
	var value syscall.Utsname
	if err := syscall.Uname(&value); err != nil {
		return "", "", err
	}
	return int8String(value.Release[:]), int8String(value.Machine[:]), nil
}

func int8String(value []int8) string {
	bytes := make([]byte, 0, len(value))
	for _, character := range value {
		if character == 0 {
			break
		}
		bytes = append(bytes, byte(character))
	}
	return string(bytes)
}

func safeText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func roundOne(value float64) float64 {
	return math.Round(value*10) / 10
}

func clamp[T ~int64 | ~float64](value T, minimum T, maximum T) T {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
