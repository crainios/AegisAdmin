package webupdates

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Info struct {
	Backend             string `json:"backend"`
	LastRefresh         any    `json:"last_refresh"`
	UpdateCount         int    `json:"update_count"`
	SecurityUpdateCount int    `json:"security_update_count"`
	RebootRequired      bool   `json:"reboot_required"`
}
type Update struct {
	Name             string `json:"name"`
	Architecture     string `json:"architecture"`
	InstalledVersion string `json:"installed_version"`
	CandidateVersion string `json:"candidate_version"`
	Security         bool   `json:"security"`
}
type Firmware struct {
	Backend        string           `json:"backend"`
	Version        *string          `json:"version"`
	Available      bool             `json:"available"`
	DeviceCount    int              `json:"device_count"`
	RebootRequired bool             `json:"reboot_required"`
	Updates        []FirmwareUpdate `json:"updates"`
}
type FirmwareUpdate struct {
	Device           string   `json:"device"`
	Vendor           string   `json:"vendor"`
	InstalledVersion string   `json:"installed_version"`
	CandidateVersion string   `json:"candidate_version"`
	Release          string   `json:"release"`
	Summary          string   `json:"summary"`
	RebootRequired   bool     `json:"reboot_required"`
	Issues           []string `json:"issues"`
}
type ComposerSite struct {
	Domain             string             `json:"domain"`
	ProjectID          string             `json:"project_id"`
	Status             string             `json:"status"`
	UpdateCount        int                `json:"update_count"`
	CompatibleCount    int                `json:"compatible_update_count"`
	SecurityIssueCount int                `json:"security_issue_count"`
	SecurityStatus     string             `json:"security_status"`
	Packages           []ComposerPackage  `json:"packages"`
	SecurityAdvisories []ComposerAdvisory `json:"security_advisories"`
	Message            string             `json:"message"`
	Stale              bool               `json:"stale"`
}
type ComposerPackage struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Status         string `json:"status"`
	Direct         bool   `json:"direct"`
}
type ComposerAdvisory struct {
	Package          string `json:"package"`
	ID               string `json:"id"`
	Title            string `json:"title"`
	AffectedVersions string `json:"affected_versions"`
	Link             string `json:"link"`
}
type Composer struct {
	Available   bool           `json:"available"`
	Refreshing  bool           `json:"refreshing"`
	RefreshedAt *string        `json:"refreshed_at"`
	Sites       []ComposerSite `json:"sites"`
	Message     string         `json:"message"`
}
type Snapshot struct {
	Info     Info
	Updates  []Update
	Firmware Firmware
	Composer Composer
}
type Summary struct {
	Success             bool   `json:"success"`
	Message             string `json:"message"`
	Complete            bool   `json:"complete"`
	UpdatesAvailable    bool   `json:"updatesAvailable"`
	UpdateCount         int    `json:"updateCount"`
	APTUpdateCount      int    `json:"aptUpdateCount"`
	FirmwareUpdateCount int    `json:"firmwareUpdateCount"`
	SecurityUpdateCount int    `json:"securityUpdateCount"`
	RebootRequired      bool   `json:"rebootRequired"`
	Status              string `json:"status"`
	StatusLabel         string `json:"statusLabel"`
	Value               string `json:"value"`
	Subtitle            string `json:"subtitle"`
	URL                 string `json:"url"`
}
type Job struct {
	JobID      string   `json:"job_id"`
	Status     string   `json:"status"`
	Backend    string   `json:"backend"`
	Lines      []string `json:"lines"`
	ExitCode   *int     `json:"exit_code"`
	StartedAt  string   `json:"started_at"`
	FinishedAt *string  `json:"finished_at"`
}
type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend: backend} }
func (c *Client) Summary(ctx context.Context) (Summary, error) {
	var summary Summary
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	var info Info
	data, err := c.call("info", nil)
	if err != nil {
		return summary, err
	}
	if err = decode(data, &info); err != nil {
		return summary, err
	}
	var firmware Firmware
	data, err = c.call("firmware", nil)
	if err != nil {
		return summary, err
	}
	if err = decode(data, &firmware); err != nil {
		return summary, err
	}
	firmwareSecurity := 0
	for _, update := range firmware.Updates {
		if len(update.Issues) > 0 {
			firmwareSecurity++
		}
	}
	summary.Success = true
	summary.Complete = firmware.Available
	summary.APTUpdateCount = info.UpdateCount
	summary.FirmwareUpdateCount = firmware.DeviceCount
	summary.UpdateCount = summary.APTUpdateCount + summary.FirmwareUpdateCount
	summary.SecurityUpdateCount = info.SecurityUpdateCount + firmwareSecurity
	summary.RebootRequired = info.RebootRequired || firmware.RebootRequired
	summary.UpdatesAvailable = summary.UpdateCount > 0
	summary.URL = "/updates"
	backend := info.Backend
	if backend == "" {
		backend = "Paquets"
	} else {
		backend = strings.ToUpper(backend)
	}
	if summary.SecurityUpdateCount > 0 {
		summary.Status, summary.StatusLabel = "danger", "Sécurité"
	} else if summary.UpdatesAvailable || summary.RebootRequired {
		summary.Status = "warning"
		if summary.UpdatesAvailable {
			summary.StatusLabel = "Mises à jour disponibles"
		} else {
			summary.StatusLabel = "Redémarrage requis"
		}
	} else if !summary.Complete {
		summary.Status, summary.StatusLabel = "neutral", "État partiel"
	} else {
		summary.Status, summary.StatusLabel = "success", "À jour"
	}
	if summary.UpdatesAvailable {
		plural := ""
		if summary.UpdateCount > 1 {
			plural = "s"
		}
		summary.Value = fmt.Sprintf("%d mise%s à jour", summary.UpdateCount, plural)
		summary.Subtitle = fmt.Sprintf("%d %s · %d firmware · %d sécurité", summary.APTUpdateCount, backend, summary.FirmwareUpdateCount, summary.SecurityUpdateCount)
	} else if !summary.Complete {
		summary.Value, summary.Subtitle = backend+" à jour", "Supervision des firmwares indisponible."
	} else if summary.RebootRequired {
		summary.Value, summary.Subtitle = "Redémarrage requis", "Aucune nouvelle mise à jour disponible."
	} else {
		summary.Value, summary.Subtitle = "Système à jour", "Aucune mise à jour "+backend+" ou firmware."
	}
	return summary, nil
}
func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	var s Snapshot
	if err := ctx.Err(); err != nil {
		return s, err
	}
	for _, x := range []struct {
		command string
		target  any
	}{{"info", &s.Info}, {"firmware", &s.Firmware}, {"composer-status", &s.Composer}} {
		d, e := c.call(x.command, nil)
		if e != nil {
			return s, e
		}
		if e = decode(d, x.target); e != nil {
			return s, e
		}
	}
	d, e := c.call("list", nil)
	if e != nil {
		return s, e
	}
	var l struct {
		Updates []Update `json:"updates"`
	}
	if e = decode(d, &l); e != nil {
		return s, e
	}
	s.Updates = l.Updates
	if s.Updates == nil {
		s.Updates = []Update{}
	}
	if s.Firmware.Updates == nil {
		s.Firmware.Updates = []FirmwareUpdate{}
	}
	if s.Composer.Sites == nil {
		s.Composer.Sites = []ComposerSite{}
	}
	return s, nil
}
func (c *Client) StartUpgrade(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	d, e := c.call("upgrade-start", nil)
	if e != nil {
		return "", e
	}
	id, ok := d["job_id"].(string)
	if !ok || len(id) != 32 {
		return "", errors.New("invalid update job")
	}
	return id, nil
}
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var j Job
	if err := ctx.Err(); err != nil {
		return j, err
	}
	d, e := c.call("upgrade-status", []string{id})
	if e != nil {
		return j, e
	}
	e = decode(d, &j)
	return j, e
}
func (c *Client) RefreshComposer(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := c.call("composer-refresh", nil)
	return err
}
func (c *Client) call(command string, args []string) (map[string]any, error) {
	r, e := c.backend.Execute(protocol.Request{Domain: "updates", Command: command, Arguments: args})
	if e != nil || !r.Response.Success || r.Response.Data == nil {
		return nil, errors.New("updates request failed")
	}
	return *r.Response.Data, nil
}
func decode(data map[string]any, target any) error {
	b, e := json.Marshal(data)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, target)
}
