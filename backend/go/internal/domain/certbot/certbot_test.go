package certbot

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeRunner map[string]struct {
	output string
	status int
}

type countingRunner struct {
	fakeRunner
	calls map[string]int
}

func (r fakeRunner) Run(_ context.Context, name string, args ...string) (string, int) {
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	value := r[key]
	return value.output, value.status
}

func (r *countingRunner) Run(ctx context.Context, name string, args ...string) (string, int) {
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	r.calls[key]++
	return r.fakeRunner.Run(ctx, name, args...)
}
func executableFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInfoAndStatus(t *testing.T) {
	tool := executableFixture(t)
	paths := Paths{Certbot: tool, DpkgQuery: tool, Systemctl: tool}
	runner := fakeRunner{
		tool + " --version": {"certbot 2.11.0", 0}, tool + " -W -f=${Package}\t${Version}\n certbot": {"certbot\t2.11.0-1", 0}, tool + " plugins": {"* webroot\n* apache\n* webroot", 0},
		tool + " show certbot.timer --property LoadState --property ActiveState --property UnitFileState --property LastTriggerUSec --property NextElapseUSecRealtime":     {"LoadState=loaded\nActiveState=active\nUnitFileState=enabled\nLastTriggerUSec=Sat 2026-08-15 10:00:00 UTC\nNextElapseUSecRealtime=Sun 2026-08-16 10:00:00 UTC", 0},
		tool + " show certbot.service --property LoadState --property ActiveState --property SubState --property Result --property ExecMainCode --property ExecMainStatus": {"LoadState=loaded\nActiveState=inactive\nSubState=dead\nResult=success\nExecMainCode=1\nExecMainStatus=0", 0},
	}
	backend := &Backend{runner: runner, paths: paths, now: time.Now}
	handler := New(backend)
	info := handler.Handle(context.Background(), "info", nil)
	if !info.Response.Success || (*info.Response.Data)["version"] != "2.11.0" || len((*info.Response.Data)["plugins"].([]string)) != 2 {
		t.Fatalf("info=%#v", info)
	}
	status := handler.Handle(context.Background(), "status", nil)
	if !status.Response.Success || (*status.Response.Data)["timer_active"] != true || (*status.Response.Data)["last_trigger"] != "2026-08-15T10:00:00Z" {
		t.Fatalf("status=%#v", status)
	}
}

func TestInfoCachesSuccessfulInspection(t *testing.T) {
	tool := executableFixture(t)
	runner := &countingRunner{
		fakeRunner: fakeRunner{
			tool + " --version": {"certbot 2.11.0", 0},
			tool + " -W -f=${Package}\t${Version}\n certbot": {"certbot\t2.11.0-1", 0},
			tool + " plugins": {"* webroot\n* apache", 0},
		},
		calls: map[string]int{},
	}
	backend := &Backend{
		runner: runner,
		paths:  Paths{Certbot: tool, DpkgQuery: tool},
		now:    time.Now,
	}

	first, failure := backend.info(context.Background())
	if failure != nil {
		t.Fatalf("first failure=%#v", failure)
	}
	first["plugins"].([]string)[0] = "changed"
	second, failure := backend.info(context.Background())
	if failure != nil {
		t.Fatalf("second failure=%#v", failure)
	}
	if second["plugins"].([]string)[0] != "apache" {
		t.Fatalf("cached info was mutated: %#v", second)
	}
	for command, count := range runner.calls {
		if count != 1 {
			t.Fatalf("%s called %d times", command, count)
		}
	}
}

func TestInfoDetectsRPMPackage(t *testing.T) {
	certbot := executableFixture(t)
	rpm := executableFixture(t)
	runner := fakeRunner{
		certbot + " --version":                          {"certbot 2.11.0", 0},
		certbot + " plugins":                            {"* webroot", 0},
		rpm + " -qf --qf %{NAME}\t%{EVR}\\n " + certbot: {"python3-certbot\t2.11.0-3.el9", 0},
	}
	backend := &Backend{runner: runner, paths: Paths{Certbot: certbot, RPM: rpm}, now: time.Now}
	info, failure := backend.info(context.Background())
	if failure != nil || info["installation"] != "rpm" || info["package"] != "python3-certbot" || info["package_version"] != "2.11.0-3.el9" {
		t.Fatalf("rpm info=%#v failure=%#v", info, failure)
	}
}

func TestInfoDetectsManualInstallation(t *testing.T) {
	certbot := executableFixture(t)
	runner := fakeRunner{certbot + " --version": {"certbot 2.11.0", 0}, certbot + " plugins": {"* webroot", 0}}
	backend := &Backend{runner: runner, paths: Paths{Certbot: certbot}, now: time.Now}
	info, failure := backend.info(context.Background())
	if failure != nil || info["installation"] != "manual" || info["package"] != nil || info["package_version"] != nil {
		t.Fatalf("manual info=%#v failure=%#v", info, failure)
	}
}

func TestCertificateFixture(t *testing.T) {
	root := t.TempDir()
	renewal := filepath.Join(root, "renewal")
	live := filepath.Join(root, "live")
	archive := filepath.Join(root, "archive")
	for _, path := range []string{renewal, filepath.Join(live, "example.com"), filepath.Join(archive, "example.com")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config, err := os.ReadFile(filepath.Join("testdata", "renewal.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(renewal, "example.com.conf"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	notAfter := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	template := x509.Certificate{SerialNumber: big.NewInt(0x1234), Subject: pkix.Name{CommonName: "example.com"}, Issuer: pkix.Name{Organization: []string{"Test CA"}}, NotBefore: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), NotAfter: notAfter, DNSNames: []string{"www.example.com", "example.com"}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificatePath := filepath.Join(archive, "example.com", "cert1.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(certificatePath, filepath.Join(live, "example.com", "cert.pem")); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{paths: Paths{RenewalDirectory: renewal, LiveDirectory: live, ArchiveDirectory: archive}, now: func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }}
	data, failure := backend.certificates(context.Background())
	if failure != nil {
		t.Fatalf("failure=%#v", failure)
	}
	items := data["certificates"].([]Certificate)
	if len(items) != 1 || items[0].KeyType != "RSA" || items[0].Issuer != "example.com" || items[0].DaysRemaining != 31 || items[0].Authenticator != "webroot" {
		t.Fatalf("items=%#v", items)
	}
}

func TestParametersAndErrors(t *testing.T) {
	parameters, failure := issueParameters([]string{"admin@example.com", "redirect", "example.com", "www.example.com"})
	if failure != nil || parameters["redirect"] != true {
		t.Fatalf("parameters=%#v failure=%#v", parameters, failure)
	}
	for _, args := range [][]string{{"bad", "redirect", "example.com"}, {"admin@example.com", "invalid", "example.com"}, {"admin@example.com", "redirect", "Example.com"}, {"admin@example.com", "redirect", "hidden.onion"}, {"admin@example.com", "redirect", "example.com", "example.com"}} {
		if _, failure := issueParameters(args); failure == nil || failure.Response.Error.Code != "INVALID_CERTBOT_ISSUE_PARAMETERS" {
			t.Fatalf("args=%#v failure=%#v", args, failure)
		}
	}
	handler := New(&Backend{})
	for _, item := range []struct {
		command string
		args    []string
		code    string
	}{{"", nil, "MISSING_COMMAND"}, {"unknown", nil, "COMMAND_NOT_FOUND"}, {"info", []string{"x"}, "INVALID_ARGUMENT_COUNT"}} {
		reply := handler.Handle(context.Background(), item.command, item.args)
		if reply.Response.Error == nil || reply.Response.Error.Code != item.code {
			t.Fatalf("reply=%#v", reply)
		}
	}
}

func TestSerialFormattingKeepsFullHexBytes(t *testing.T) {
	serial := formatSerial(big.NewInt(0x057e).Text(16))
	if serial != "057e" {
		t.Fatalf("serial = %q", serial)
	}
}
