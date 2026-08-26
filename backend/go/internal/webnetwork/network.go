package webnetwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"aegisadmin/backend/internal/protocol"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}

type Address struct {
	Address string `json:"address"`
	Prefix  int    `json:"prefix"`
}

type Interface struct {
	ID, Name, Type, State, TypeLabel, Status, StatusLabel string
	MAC                                                   *string
	MTU                                                   *int
	IPv4, IPv6                                            []Address
}

type Summary struct{ Total, Up, Down, Unknown int }
type Snapshot struct {
	Interfaces []Interface
	Selected   *Interface
	Summary    Summary
}

type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend: backend} }

func (c *Client) NetworkSnapshot(ctx context.Context, requested string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "network", Command: "list"})
	if err != nil || !reply.Response.Success || reply.Response.Data == nil {
		return Snapshot{}, errors.New("network list unavailable")
	}
	var payload struct {
		Interfaces []struct {
			ID, Name, Type, State string
		} `json:"interfaces"`
	}
	if err = decode(*reply.Response.Data, &payload); err != nil {
		return Snapshot{}, fmt.Errorf("decode network list: %w", err)
	}
	snapshot := Snapshot{Interfaces: make([]Interface, 0, len(payload.Interfaces))}
	selected := ""
	for _, source := range payload.Interfaces {
		item := Interface{ID: source.ID, Name: source.Name, Type: source.Type, State: source.State}
		item.TypeLabel = typeLabel(item.Type)
		item.Status, item.StatusLabel = status(item.State)
		snapshot.Interfaces = append(snapshot.Interfaces, item)
		snapshot.Summary.Total++
		switch item.State {
		case "up":
			snapshot.Summary.Up++
			if selected == "" && item.Type != "loopback" {
				selected = item.ID
			}
		case "down":
			snapshot.Summary.Down++
		default:
			snapshot.Summary.Unknown++
		}
		if requested != "" && requested == item.ID {
			selected = item.ID
		}
	}
	if selected == "" && len(snapshot.Interfaces) != 0 {
		selected = snapshot.Interfaces[0].ID
	}
	if selected == "" {
		return snapshot, nil
	}
	detailReply, err := c.backend.Execute(protocol.Request{Domain: "network", Command: "status", Arguments: []string{selected}})
	if err != nil || !detailReply.Response.Success || detailReply.Response.Data == nil {
		return Snapshot{}, errors.New("network status unavailable")
	}
	var details struct {
		ID, Name, Type, State string
		MAC                   *string   `json:"mac"`
		MTU                   *int      `json:"mtu"`
		IPv4                  []Address `json:"ipv4"`
		IPv6                  []Address `json:"ipv6"`
	}
	if err = decode(*detailReply.Response.Data, &details); err != nil {
		return Snapshot{}, fmt.Errorf("decode network status: %w", err)
	}
	item := Interface{ID: details.ID, Name: details.Name, Type: details.Type, State: details.State, MAC: details.MAC, MTU: details.MTU, IPv4: details.IPv4, IPv6: details.IPv6}
	item.TypeLabel = typeLabel(item.Type)
	item.Status, item.StatusLabel = status(item.State)
	snapshot.Selected = &item
	return snapshot, nil
}

func decode(data map[string]any, target any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func status(state string) (string, string) {
	switch state {
	case "up":
		return "success", "Active"
	case "down":
		return "warning", "Arrêtée"
	default:
		return "neutral", "État inconnu"
	}
}

func typeLabel(value string) string {
	labels := map[string]string{"ethernet": "Ethernet", "loopback": "Boucle locale", "bridge": "Pont", "vlan": "VLAN", "bond": "Agrégation", "wireguard": "WireGuard", "tun": "Tunnel TUN", "tap": "Tunnel TAP"}
	if label := labels[value]; label != "" {
		return label
	}
	return "Inconnu"
}
