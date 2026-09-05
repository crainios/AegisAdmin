package webupdates

import (
	"context"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestDecodeComposerDetails(t *testing.T) {
	data := map[string]any{
		"domain": "example.test", "status": "outdated", "compatible_update_count": 1,
		"security_status": "alerts", "stale": true,
		"packages":            []any{map[string]any{"name": "vendor/package", "current_version": "1.0.0", "latest_version": "1.1.0", "status": "semver-safe-update", "direct": true}},
		"security_advisories": []any{map[string]any{"package": "vendor/package", "id": "CVE-TEST", "title": "Test", "affected_versions": "<1.1.0", "link": "https://example.test/advisory"}},
	}
	var site ComposerSite
	if err := decode(data, &site); err != nil {
		t.Fatal(err)
	}
	if site.CompatibleCount != 1 || !site.Stale || len(site.Packages) != 1 || !site.Packages[0].Direct || site.Packages[0].CurrentVersion != "1.0.0" || len(site.SecurityAdvisories) != 1 || site.SecurityAdvisories[0].ID != "CVE-TEST" {
		t.Fatalf("composer details were lost: %#v", site)
	}
}

func TestSummaryCombinesPackageAndFirmwareUpdates(t *testing.T) {
	client := New(summaryBackend{data: map[string]map[string]any{
		"info":     {"backend": "apt", "update_count": 2, "security_update_count": 1, "reboot_required": false},
		"firmware": {"available": true, "device_count": 1, "reboot_required": true, "updates": []any{map[string]any{"device": "UEFI", "issues": []any{"CVE-TEST"}}}},
	}})
	summary, err := client.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.UpdateCount != 3 || summary.SecurityUpdateCount != 2 || summary.Status != "danger" || summary.Value != "3 mises à jour" || summary.RebootRequired {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

type summaryBackend struct{ data map[string]map[string]any }

func (f summaryBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	return protocol.Reply{Response: api.Success(f.data[request.Command])}, nil
}
