package backup

import "testing"

func TestValidateAcceptsControlledRemoteTask(t *testing.T) {
	task := Task{ID: "12345678-1234-4234-8234-123456789abc", Name: "Sites", Kind: "sites", Source: "/var/www", Destination: "/var/backups/aegisadmin/sites", RemoteHost: "backup.example.net", RemoteUser: "backup", RemotePort: 22, RemotePath: "/srv/backups/web", SSHKey: "/etc/aegisadmin-system/backup-ssh/id_ed25519", RetentionDays: 2}
	if err := Validate(task); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsSourcesOutsideAllowlist(t *testing.T) {
	task := Task{ID: "12345678-1234-4234-8234-123456789abc", Name: "Secrets", Kind: "sites", Source: "/etc", Destination: "/var/backups/aegisadmin/sites", RemoteHost: "backup.example.net", RemoteUser: "backup", RemotePort: 22, RemotePath: "/srv/backups", SSHKey: "/etc/aegisadmin-system/backup-ssh/id_ed25519", RetentionDays: 2}
	if err := Validate(task); err == nil {
		t.Fatal("Validate() accepted an unsafe source")
	}
}

func TestValidateRejectsRemoteInjection(t *testing.T) {
	task := Task{ID: "12345678-1234-4234-8234-123456789abc", Name: "Sites", Kind: "sites", Source: "/var/www", Destination: "/var/backups/aegisadmin/sites", RemoteHost: "host;touch-x", RemoteUser: "backup", RemotePort: 22, RemotePath: "/srv/backups", SSHKey: "/etc/aegisadmin-system/backup-ssh/id_ed25519", RetentionDays: 2}
	if err := Validate(task); err == nil {
		t.Fatal("Validate() accepted an unsafe host")
	}
}
