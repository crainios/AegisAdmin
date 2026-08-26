package network

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,63}$`)

var ipCommandCandidates = []string{"/usr/sbin/ip", "/usr/bin/ip"}

type Address struct {
	Address string `json:"address"`
	Prefix  int    `json:"prefix"`
}

type Interface struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Type  string    `json:"type"`
	State string    `json:"state"`
	MAC   *string   `json:"mac"`
	MTU   *int      `json:"mtu"`
	IPv4  []Address `json:"ipv4"`
	IPv6  []Address `json:"ipv6"`
}

type Collector interface {
	Interfaces(context.Context) ([]Interface, error)
}

type LinuxCollector struct{}

type Handler struct{ collector Collector }

type collectorError struct {
	code    string
	message string
}

func (e *collectorError) Error() string { return e.message }

type ipInterface struct {
	Name      string          `json:"ifname"`
	LinkType  string          `json:"link_type"`
	OperState string          `json:"operstate"`
	Address   string          `json:"address"`
	MTU       *int            `json:"mtu"`
	LinkInfo  json.RawMessage `json:"linkinfo"`
	AddrInfo  json.RawMessage `json:"addr_info"`
}

type ipLinkInfo struct {
	Kind string          `json:"info_kind"`
	Data json.RawMessage `json:"info_data"`
}

type ipInfoData struct {
	Mode string `json:"mode"`
}

type ipAddress struct {
	Family string `json:"family"`
	Local  string `json:"local"`
	Prefix *int   `json:"prefixlen"`
}

func New(collector Collector) *Handler   { return &Handler{collector: collector} }
func NewLinuxCollector() *LinuxCollector { return &LinuxCollector{} }

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine network.")
	}

	switch command {
	case "list":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		interfaces, err := h.collector.Interfaces(ctx)
		if err != nil {
			return collectorFailure(err)
		}
		items := make([]map[string]any, 0, len(interfaces))
		for _, item := range interfaces {
			items = append(items, summary(item))
		}
		return success(map[string]any{"interfaces": items})

	case "status":
		if len(arguments) != 1 {
			return invalidArgumentCount()
		}
		if !identifierPattern.MatchString(arguments[0]) {
			return failure(2, "INVALID_NETWORK_IDENTIFIER", "L’identifiant de l’interface réseau est invalide.")
		}
		interfaces, err := h.collector.Interfaces(ctx)
		if err != nil {
			return collectorFailure(err)
		}
		for _, item := range interfaces {
			if item.ID == arguments[0] {
				return success(details(item))
			}
		}
		return failure(5, "NETWORK_INTERFACE_NOT_FOUND", "L’interface réseau demandée est introuvable.")

	default:
		return failure(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine network.")
	}
}

func (c *LinuxCollector) Interfaces(ctx context.Context) ([]Interface, error) {
	command := ""
	for _, candidate := range ipCommandCandidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			command = candidate
			break
		}
	}
	if command == "" {
		return nil, &collectorError{
			code:    "DEPENDENCY_NOT_FOUND",
			message: "La commande ip nécessaire au domaine network est introuvable.",
		}
	}

	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, command, "-j", "address", "show").Output()
	if err != nil {
		return nil, errors.New("ip address show failed")
	}
	if len(output) == 0 {
		return nil, &collectorError{
			code:    "NETWORK_READ_FAILED",
			message: "La commande ip n’a retourné aucune information réseau.",
		}
	}
	interfaces, err := normalize(output)
	if err != nil {
		return nil, &collectorError{
			code:    "INVALID_NETWORK_DATA",
			message: "Les informations réseau retournées par le système sont invalides.",
		}
	}
	return interfaces, nil
}

func normalize(raw []byte) ([]Interface, error) {
	var payload []json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errors.New("network payload is not an array")
	}

	interfaces := make([]Interface, 0, len(payload))
	for _, entry := range payload {
		var source ipInterface
		if err := json.Unmarshal(entry, &source); err != nil || source.Name == "" {
			continue
		}
		item := Interface{
			ID: source.Name, Name: source.Name,
			Type: normalizeType(source), State: normalizeState(source.OperState),
			IPv4: []Address{}, IPv6: []Address{},
		}
		if source.Address != "" {
			address := source.Address
			item.MAC = &address
		}
		if source.MTU != nil && *source.MTU >= 0 {
			item.MTU = source.MTU
		}
		item.IPv4, item.IPv6 = normalizeAddresses(source.AddrInfo)
		interfaces = append(interfaces, item)
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].ID < interfaces[j].ID })
	return interfaces, nil
}

func normalizeState(state string) string {
	switch strings.ToUpper(state) {
	case "UP":
		return "up"
	case "DOWN":
		return "down"
	default:
		return "unknown"
	}
}

func normalizeType(source ipInterface) string {
	if source.Name == "lo" || strings.ToLower(source.LinkType) == "loopback" {
		return "loopback"
	}
	var linkInfo ipLinkInfo
	_ = json.Unmarshal(source.LinkInfo, &linkInfo)
	kind := strings.ToLower(linkInfo.Kind)
	switch kind {
	case "bridge", "vlan", "bond", "wireguard", "tun", "tap":
		return kind
	case "tuntap":
		var data ipInfoData
		_ = json.Unmarshal(linkInfo.Data, &data)
		if mode := strings.ToLower(data.Mode); mode == "tun" || mode == "tap" {
			return mode
		}
	}
	if strings.ToLower(source.LinkType) == "ether" {
		return "ethernet"
	}
	return "unknown"
}

func normalizeAddresses(raw json.RawMessage) ([]Address, []Address) {
	ipv4 := []Address{}
	ipv6 := []Address{}
	var addresses []json.RawMessage
	if err := json.Unmarshal(raw, &addresses); err != nil {
		return ipv4, ipv6
	}
	for _, entry := range addresses {
		var source ipAddress
		if err := json.Unmarshal(entry, &source); err != nil || source.Local == "" || source.Prefix == nil {
			continue
		}
		address := Address{Address: source.Local, Prefix: *source.Prefix}
		switch source.Family {
		case "inet":
			ipv4 = append(ipv4, address)
		case "inet6":
			ipv6 = append(ipv6, address)
		}
	}
	less := func(items []Address) func(int, int) bool {
		return func(i, j int) bool {
			if items[i].Address == items[j].Address {
				return items[i].Prefix < items[j].Prefix
			}
			return items[i].Address < items[j].Address
		}
	}
	sort.Slice(ipv4, less(ipv4))
	sort.Slice(ipv6, less(ipv6))
	return ipv4, ipv6
}

func summary(item Interface) map[string]any {
	return map[string]any{"id": item.ID, "name": item.Name, "type": item.Type, "state": item.State}
}

func details(item Interface) map[string]any {
	data := summary(item)
	data["mac"], data["mtu"], data["ipv4"], data["ipv6"] = item.MAC, item.MTU, item.IPv4, item.IPv6
	return data
}

func success(data map[string]any) protocol.Reply { return protocol.Reply{Response: api.Success(data)} }
func failure(exit int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exit, Response: api.Failure(code, message)}
}
func collectorFailure(err error) protocol.Reply {
	var typed *collectorError
	if errors.As(err, &typed) {
		return failure(3, typed.code, typed.message)
	}
	return failure(3, "NETWORK_READ_FAILED", "Les informations réseau n’ont pas pu être lues.")
}
func invalidArgumentCount() protocol.Reply {
	return failure(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
}
