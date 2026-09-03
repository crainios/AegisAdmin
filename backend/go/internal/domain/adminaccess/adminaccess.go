package adminaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	adminProfile  = "/etc/aegisadmin-system/admin-web"
	apacheProfile = "/etc/aegisadmin-system/apache"
	certificate   = "/etc/aegisadmin-system/tls/admin-local.crt"
	privateKey    = "/etc/aegisadmin-system/tls/admin-local.key"
	webProfile    = "/etc/aegisadmin-system/web-server"
)

type Handler struct{}
type settings struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Address   string `json:"address"`
	AllowFrom string `json:"allow_from"`
}
type profile struct {
	Enabled                     bool
	AppRoot, Address, AllowFrom string
	Port                        int
}
type apache struct{ Service, Control, SitesAvailable, SitesEnabled, EnableCommand, DisableCommand string }

func New() *Handler { return &Handler{} }

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "status" && len(arguments) == 0 {
		return success(status())
	}
	if command == "update" && len(arguments) == 1 {
		var requested settings
		if json.Unmarshal([]byte(arguments[0]), &requested) != nil {
			return failure("INVALID_ARGUMENT", "Les paramètres d’accès sont invalides.")
		}
		data, err := update(ctx, requested)
		if err != nil {
			return failure("ADMIN_ACCESS_UPDATE_FAILED", err.Error())
		}
		return success(data)
	}
	if command == "reconcile-package" && len(arguments) == 0 {
		data, err := reconcilePackage(ctx)
		if err != nil {
			return failure("ADMIN_ACCESS_RECONCILE_FAILED", err.Error())
		}
		return success(data)
	}
	if command == "" {
		return failure("MISSING_COMMAND", "Aucune commande n’a été indiquée pour l’accès d’administration.")
	}
	return failure("COMMAND_NOT_FOUND", "La commande demandée n’existe pas pour l’accès d’administration.")
}

func reconcilePackage(ctx context.Context) (map[string]any, error) {
	p, err := loadAdmin(adminProfile)
	if err != nil {
		return nil, fmt.Errorf("le profil de l’accès dédié est introuvable")
	}
	return update(ctx, settings{
		Enabled: p.Enabled, Port: p.Port, Address: p.Address, AllowFrom: p.AllowFrom,
	})
}

func success(data map[string]any) protocol.Reply { return protocol.Reply{Response: api.Success(data)} }
func failure(code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: 2, Response: api.Failure(code, message)}
}

func status() map[string]any {
	p, err := loadAdmin(adminProfile)
	if err != nil {
		return map[string]any{"available": false, "enabled": false, "message": "Le profil de l’accès dédié est indisponible."}
	}
	a := loadApache(apacheProfile)
	if !apacheAvailable(a) {
		addresses, _ := parseAddresses(p.Address)
		return map[string]any{"available": true, "enabled": p.Enabled, "address": strings.Join(addresses, "\n"), "port": p.Port, "allow_from": p.AllowFrom, "url": fmt.Sprintf("https://adresse-ip-du-serveur:%d", p.Port), "mode": "direct", "message": "Accès HTTPS fourni directement par le serveur Go."}
	}
	_, enabledErr := os.Lstat(filepath.Join(a.SitesEnabled, "aegisadmin-admin.conf"))
	addresses, _ := parseAddresses(p.Address)
	return map[string]any{"available": true, "enabled": p.Enabled && enabledErr == nil, "address": strings.Join(addresses, "\n"), "port": p.Port, "allow_from": p.AllowFrom, "url": fmt.Sprintf("https://adresse-ip-du-serveur:%d", p.Port), "mode": "apache", "message": "Accès HTTPS publié par Apache."}
}

func update(ctx context.Context, s settings) (map[string]any, error) {
	if err := validate(s); err != nil {
		return nil, err
	}
	p, err := loadAdmin(adminProfile)
	if err != nil {
		return nil, fmt.Errorf("le profil de l’accès dédié est introuvable")
	}
	a := loadApache(apacheProfile)
	if !apacheAvailable(a) {
		return updateDirect(ctx, p, s)
	}
	if !filepath.IsAbs(p.AppRoot) || strings.ContainsAny(p.AppRoot, "\"\r\n") {
		return nil, fmt.Errorf("la racine AegisAdmin enregistrée est invalide")
	}
	for _, file := range []string{adminProfile, certificate, privateKey, filepath.Join(a.SitesAvailable, "aegisadmin-admin.conf")} {
		if info, e := os.Lstat(file); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("un fichier géré ne doit pas être un lien symbolique")
		}
	}
	addresses, _ := parseAddresses(s.Address)
	endpoints := make([]string, 0, len(addresses))
	for _, address := range addresses {
		endpoint := net.JoinHostPort(address, strconv.Itoa(s.Port))
		if address == "*" {
			endpoint = "*:" + strconv.Itoa(s.Port)
		}
		endpoints = append(endpoints, endpoint)
	}
	requirement := "Require all granted"
	if s.AllowFrom != "all" {
		requirement = "Require ip " + s.AllowFrom
	}
	content := render(endpoints, p.AppRoot, requirement)
	target := filepath.Join(a.SitesAvailable, "aegisadmin-admin.conf")
	old, oldErr := os.ReadFile(target)
	enabledTarget := filepath.Join(a.SitesEnabled, "aegisadmin-admin.conf")
	_, enabledErr := os.Lstat(enabledTarget)
	wasEnabled := enabledErr == nil
	if err = atomicWrite(target, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("écriture du VirtualHost impossible")
	}
	restore := func() {
		if oldErr == nil {
			_ = atomicWrite(target, old, 0644)
		} else {
			_ = os.Remove(target)
		}
		if a.SitesAvailable != a.SitesEnabled {
			if wasEnabled {
				_, _ = run(ctx, a.EnableCommand, "aegisadmin-admin.conf")
			} else {
				_, _ = run(ctx, a.DisableCommand, "aegisadmin-admin.conf")
			}
		}
		_, _ = run(ctx, a.Control, "configtest")
		_, _ = run(ctx, "/usr/bin/systemctl", "reload", a.Service)
	}
	if s.Enabled {
		if a.SitesAvailable != a.SitesEnabled {
			if _, e := os.Lstat(enabledTarget); e != nil {
				if _, e = run(ctx, a.EnableCommand, "aegisadmin-admin.conf"); e != nil {
					restore()
					return nil, fmt.Errorf("activation du VirtualHost impossible")
				}
			}
		}
	} else if a.SitesAvailable == a.SitesEnabled {
		if err = os.Remove(target); err != nil && !os.IsNotExist(err) {
			restore()
			return nil, fmt.Errorf("désactivation du VirtualHost impossible")
		}
	} else if wasEnabled {
		if _, e := run(ctx, a.DisableCommand, "aegisadmin-admin.conf"); e != nil {
			restore()
			return nil, fmt.Errorf("désactivation du VirtualHost impossible")
		}
	}
	if output, e := run(ctx, a.Control, "configtest"); e != nil {
		restore()
		return nil, fmt.Errorf("la configuration Apache est invalide : %s", strings.TrimSpace(output))
	}
	if output, e := run(ctx, "/usr/bin/systemctl", "reload", a.Service); e != nil {
		restore()
		return nil, fmt.Errorf("Apache n’a pas pu être rechargé : %s", strings.TrimSpace(output))
	}
	p.Enabled, p.Address, p.Port, p.AllowFrom = s.Enabled, strings.Join(addresses, ","), s.Port, s.AllowFrom
	if err = atomicWrite(adminProfile, []byte(fmt.Sprintf("enabled=%t\napp_root=%s\naddress=%s\nport=%d\nallow_from=%s\n", p.Enabled, p.AppRoot, p.Address, p.Port, p.AllowFrom)), 0640); err != nil {
		restore()
		return nil, fmt.Errorf("le profil de l’accès dédié n’a pas pu être enregistré")
	}
	return status(), nil
}

func updateDirect(ctx context.Context, p profile, s settings) (map[string]any, error) {
	for _, file := range []string{adminProfile, webProfile} {
		if info, err := os.Lstat(file); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("un fichier géré ne doit pas être un lien symbolique")
		}
	}
	addresses, _ := parseAddresses(s.Address)
	listeners := directListenAddresses(addresses, s.Port, s.Enabled)
	oldWeb, webErr := os.ReadFile(webProfile)
	oldAdmin, adminErr := os.ReadFile(adminProfile)
	restore := func() {
		if webErr == nil {
			_ = atomicWrite(webProfile, oldWeb, 0640)
		}
		if adminErr == nil {
			_ = atomicWrite(adminProfile, oldAdmin, 0640)
		}
		_, _ = run(ctx, "/usr/bin/systemctl", "restart", "aegisadmin-web.service")
	}
	webContent := fmt.Sprintf("# Adresse HTTPS du serveur web Go AegisAdmin.\nAEGISADMIN_WEB_LISTEN=%s\nAEGISADMIN_WEB_ALLOW_FROM=%s\n", strings.Join(listeners, ","), s.AllowFrom)
	if err := atomicWrite(webProfile, []byte(webContent), 0640); err != nil {
		return nil, fmt.Errorf("le profil du serveur web Go n’a pas pu être enregistré")
	}
	p.Enabled, p.Address, p.Port, p.AllowFrom = s.Enabled, strings.Join(addresses, ","), s.Port, s.AllowFrom
	adminContent := fmt.Sprintf("enabled=%t\napp_root=%s\naddress=%s\nport=%d\nallow_from=%s\n", p.Enabled, p.AppRoot, p.Address, p.Port, p.AllowFrom)
	if err := atomicWrite(adminProfile, []byte(adminContent), 0640); err != nil {
		restore()
		return nil, fmt.Errorf("le profil de l’accès dédié n’a pas pu être enregistré")
	}
	if output, err := run(ctx, "/usr/bin/systemctl", "restart", "aegisadmin-web.service"); err != nil {
		restore()
		return nil, fmt.Errorf("le serveur web Go n’a pas pu être redémarré : %s", strings.TrimSpace(output))
	}
	return status(), nil
}

func directListenAddresses(addresses []string, port int, enabled bool) []string {
	if !enabled {
		return []string{"127.0.0.1:9080"}
	}
	listeners := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address == "*" {
			listeners = append(listeners, ":"+strconv.Itoa(port))
		} else {
			listeners = append(listeners, net.JoinHostPort(address, strconv.Itoa(port)))
		}
	}
	return listeners
}

func apacheAvailable(a apache) bool {
	for _, path := range []string{a.Control, a.EnableCommand, a.DisableCommand} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			return false
		}
	}
	return true
}

func validate(s settings) error {
	if s.Port < 1024 || s.Port > 65535 {
		return fmt.Errorf("le port doit être compris entre 1024 et 65535")
	}
	if _, err := parseAddresses(s.Address); err != nil {
		return err
	}
	if s.AllowFrom != "all" {
		if net.ParseIP(s.AllowFrom) == nil {
			if _, _, err := net.ParseCIDR(s.AllowFrom); err != nil {
				return fmt.Errorf("l’adresse ou le réseau autorisé est invalide")
			}
		}
	}
	return nil
}

func parseAddresses(value string) ([]string, error) {
	items := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	if len(items) == 0 || len(items) > 16 {
		return nil, fmt.Errorf("indiquez entre une et seize adresses d’écoute")
	}
	seen := make(map[string]struct{}, len(items))
	addresses := make([]string, 0, len(items))
	for _, item := range items {
		if item == "*" && len(items) != 1 {
			return nil, fmt.Errorf("le caractère * ne peut pas être combiné avec une adresse IP")
		}
		if item != "*" && net.ParseIP(item) == nil {
			return nil, fmt.Errorf("l’adresse d’écoute %q est invalide", item)
		}
		if _, exists := seen[item]; exists {
			return nil, fmt.Errorf("l’adresse d’écoute %q est indiquée plusieurs fois", item)
		}
		seen[item] = struct{}{}
		addresses = append(addresses, item)
	}
	return addresses, nil
}

func loadAdmin(path string) (profile, error) {
	values, err := readProfile(path)
	if err != nil {
		return profile{}, err
	}
	port, err := strconv.Atoi(values["port"])
	if err != nil {
		return profile{}, err
	}
	p := profile{Enabled: values["enabled"] == "true", AppRoot: values["app_root"], Address: values["address"], AllowFrom: values["allow_from"], Port: port}
	if p.AppRoot == "" || validate(settings{Port: p.Port, Address: p.Address, AllowFrom: p.AllowFrom}) != nil {
		return profile{}, fmt.Errorf("invalid profile")
	}
	return p, nil
}
func loadApache(path string) apache {
	v, _ := readProfile(path)
	value := func(k, d string) string {
		if filepath.IsAbs(v[k]) || k == "service" {
			if v[k] != "" {
				return v[k]
			}
		}
		return d
	}
	return apache{value("service", "apache2"), value("control", "/usr/sbin/apachectl"), value("sites_available", "/etc/apache2/sites-available"), value("sites_enabled", "/etc/apache2/sites-enabled"), value("enable_command", "/usr/sbin/a2ensite"), value("disable_command", "/usr/sbin/a2dissite")}
}
func readProfile(path string) (map[string]string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	v := map[string]string{}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		k, x, ok := strings.Cut(line, "=")
		if ok {
			v[strings.TrimSpace(k)] = strings.TrimSpace(x)
		}
	}
	return v, nil
}
func atomicWrite(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, e := os.CreateTemp(dir, ".aegisadmin-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(content)
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(name, path)
}
func run(parent context.Context, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, command, args...).CombinedOutput()
	return string(out), e
}
func render(endpoints []string, root, requirement string) string {
	_ = root
	listen := "Listen " + strings.Join(endpoints, "\nListen ")
	return fmt.Sprintf("%s\n\n<VirtualHost %s>\n    ServerName aegisadmin.local\n\n    SSLEngine on\n    SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1\n    SSLCertificateFile \"%s\"\n    SSLCertificateKeyFile \"%s\"\n    SSLProxyEngine on\n    SSLProxyVerify none\n    SSLProxyCheckPeerName off\n\n    Header always set X-Content-Type-Options \"nosniff\"\n    Header always set Referrer-Policy \"no-referrer\"\n    Header always set X-Frame-Options \"DENY\"\n\n    <Location />\n        %s\n        ProxyPass https://127.0.0.1:9080/\n        ProxyPassReverse https://127.0.0.1:9080/\n    </Location>\n</VirtualHost>\n", listen, strings.Join(endpoints, " "), certificate, privateKey, requirement)
}
