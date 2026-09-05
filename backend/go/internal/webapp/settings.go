package webapp

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/authstore"
	"aegisadmin/backend/internal/i18n"
	"aegisadmin/backend/internal/websettings"
)

func (a *application) settings(w http.ResponseWriter, r *http.Request) {
	session, current, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if a.dependencies.Settings == nil || a.dependencies.AdminAccess == nil {
		http.Error(w, "Service des paramètres indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	values, err := a.dependencies.Settings.Settings(ctx)
	if err != nil {
		http.Error(w, "Les paramètres n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	defaultLanguage := values.DefaultLanguage
	if !i18n.Supported(defaultLanguage) {
		defaultLanguage = a.defaultLanguage(ctx)
	}
	smtp, err := a.dependencies.Settings.SMTPSettings(ctx)
	if err != nil {
		http.Error(w, "Les paramètres SMTP n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	logSources, logsErr := a.dependencies.Logs.LogSources(ctx)
	admin, err := a.dependencies.AdminAccess.AdminAccess(ctx)
	if err != nil {
		http.Error(w, "L’accès HTTPS n’a pas pu être contrôlé.", http.StatusServiceUnavailable)
		return
	}
	notice := ""
	if r.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">Les paramètres ont été enregistrés.</p>`
	}
	message := ""
	if admin.Message != "" {
		message = `<p class="notice">` + html.EscapeString(admin.Message) + `</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{NOTICE}}", notice, "{{DEFAULT_LANGUAGE}}", renderLanguageOptions(defaultLanguage), "{{LOG_SOURCES}}", renderSettingLogSources(logSources, values.DefaultLog, logsErr), "{{CERTBOT_EMAIL}}", html.EscapeString(values.CertbotEmail), "{{SMTP_HOST}}", html.EscapeString(smtp.Host), "{{SMTP_USERNAME}}", html.EscapeString(smtp.Username), "{{SMTP_PORT}}", strconv.Itoa(smtp.Port), "{{SMTP_SECURITY}}", renderSMTPSecurity(smtp.Security), "{{SMTP_PASSWORD_MASK}}", smtpPasswordMask(smtp.PasswordConfigured), "{{SMTP_PASSWORD_STATUS}}", smtpPasswordStatus(smtp.PasswordConfigured), "{{ADMIN_MESSAGE}}", message, "{{ADMIN_ENABLED}}", checked(admin.Enabled), "{{ADMIN_ADDRESS}}", html.EscapeString(admin.Address), "{{ADMIN_PORT}}", strconv.Itoa(admin.Port), "{{ADMIN_ALLOW}}", html.EscapeString(admin.AllowFrom)).Replace(a.settingsPage)
	page = localizeSettingsHTML(page, a.languageForUser(ctx, current.ID))
	writeHTML(w, page, http.StatusOK)
}

func localizeSettingsHTML(value, language string) string {
	if language != "en" {
		return value
	}
	return strings.NewReplacer(
		`lang="fr"`, `lang="en"`, "Mot de passe enregistré", "Password saved", "Aucun mot de passe enregistré.", "No password saved.", "Enregistrer la configuration SMTP", "Save SMTP configuration", "Restaurer la sauvegarde", "Restore backup", "Paramètres", "Settings", "Préférences applicatives et accès d’administration", "Application preferences and administration access", "Tableau de bord", "Dashboard", "Se déconnecter", "Sign out", "Les paramètres ont été enregistrés.", "Settings were saved.",
		"Préférences", "Preferences", "Langue par défaut de l’interface et de la page de connexion", "Default language for the interface and login page", "Journal ouvert par défaut", "Default log to open", "Adresse e-mail utilisée par défaut pour les certificats", "Default email address for certificates", "Enregistrer", "Save",
		"Serveur de messagerie SMTP", "SMTP mail server", "Ces paramètres seront utilisés pour les futurs envois de notifications. Le mot de passe est chiffré avant son enregistrement en base de données.", "These settings will be used for future notifications. The password is encrypted before being stored in the database.", "Serveur Host", "Server host", "Nom utilisateur (login)", "Username (login)", "Mot de passe", "Password", "Mot de passe enregistré", "Password saved", "Aucun mot de passe enregistré.", "No password saved.", "Effacer le mot de passe enregistré", "Clear saved password", "TLS implicite (SMTPS, SSL)", "Implicit TLS (SMTPS, SSL)", "Aucun", "None", "Enregistrer la configuration SMTP", "Save SMTP configuration",
		"Accès HTTPS dédié", "Dedicated HTTPS access", "Accès activé", "Access enabled", "Adresses d’écoute", "Listening addresses", "IP ou réseau autorisé", "Allowed IP or network", "Appliquer et recharger Apache", "Apply and reload Apache",
		"Sauvegarde SQLite", "SQLite backup", "Télécharger une sauvegarde cohérente", "Download a consistent backup", "Restaurer", "Restore", "Je confirme le remplacement de la base active", "I confirm replacement of the active database", "Restaurer la sauvegarde", "Restore backup", "Journaux indisponibles", "Logs unavailable",
	).Replace(value)
}

func (a *application) updateSMTPSettings(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	host := strings.TrimSpace(r.PostForm.Get("smtp_host"))
	username := strings.TrimSpace(r.PostForm.Get("smtp_username"))
	password := r.PostForm.Get("smtp_password")
	security := strings.ToLower(strings.TrimSpace(r.PostForm.Get("smtp_security")))
	port, err := strconv.Atoi(r.PostForm.Get("smtp_port"))
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "\x00\r\n /\\") || len(username) > 254 || strings.ContainsAny(username, "\x00\r\n") || len(password) > 1024 || port < 1 || port > 65535 || !slices.Contains([]string{"none", "starttls", "tls"}, security) {
		http.Error(w, "Configuration SMTP invalide.", http.StatusBadRequest)
		return
	}
	var passwordUpdate *string
	if r.PostForm.Get("clear_password") == "1" {
		password = ""
		passwordUpdate = &password
	} else if password != "" {
		passwordUpdate = &password
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	if err = a.dependencies.Settings.UpdateSMTPSettings(ctx, authstore.SMTPSettings{Host: host, Username: username, Port: port, Security: security}, passwordUpdate); err != nil {
		http.Error(w, "Enregistrement SMTP impossible.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, "/setting?result=smtp", http.StatusSeeOther)
}

func renderSMTPSecurity(selected string) string {
	labels := []struct{ value, label string }{{"none", "Aucun"}, {"starttls", "STARTTLS"}, {"tls", "TLS implicite (SMTPS, SSL)"}}
	var result strings.Builder
	for _, item := range labels {
		result.WriteString(`<option value="` + item.value + `"`)
		if item.value == selected {
			result.WriteString(` selected`)
		}
		result.WriteString(`>` + item.label + `</option>`)
	}
	return result.String()
}

func smtpPasswordMask(configured bool) string {
	if configured {
		return "************"
	}
	return ""
}

func smtpPasswordStatus(configured bool) string {
	if configured {
		return `<span class="status-badge status-badge--success">Mot de passe enregistré</span>`
	}
	return `<span class="muted">Aucun mot de passe enregistré.</span>`
}

func (a *application) backupDatabase(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	content, err := a.dependencies.Settings.BackupDatabase(ctx)
	if err != nil {
		http.Error(w, "La sauvegarde n’a pas pu être créée.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="aegisadmin-%s.sqlite"`, time.Now().UTC().Format("20060102-150405")))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	_, _ = w.Write(content)
}
func (a *application) restoreDatabase(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21*1024*1024)
	if err := r.ParseMultipartForm(21 * 1024 * 1024); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, r.FormValue("_token")) || r.FormValue("confirmation") != "restore" {
		http.Error(w, "La requête de restauration est invalide.", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("database")
	if err != nil || header.Size < 100 || header.Size > 20*1024*1024 {
		http.Error(w, "La sauvegarde est absente ou trop volumineuse.", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 20*1024*1024+1))
	if err != nil || len(content) > 20*1024*1024 {
		http.Error(w, "La sauvegarde est illisible.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 60*time.Second)
	defer cancel()
	if err = a.dependencies.Settings.RestoreDatabase(ctx, content); err != nil {
		http.Error(w, "La sauvegarde est invalide ou incompatible.", http.StatusBadRequest)
		return
	}
	a.invalidate(w, session.ID)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (a *application) updateSettings(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	defaultLog := strings.TrimSpace(r.PostForm.Get("default_log"))
	email := strings.ToLower(strings.TrimSpace(r.PostForm.Get("certbot_email")))
	defaultLanguage := strings.TrimSpace(r.PostForm.Get("default_language"))
	if !i18n.Supported(defaultLanguage) {
		http.Error(w, "Langue par défaut invalide.", http.StatusBadRequest)
		return
	}
	if len(defaultLog) > 128 || strings.ContainsAny(defaultLog, "\x00\r\n") {
		http.Error(w, "Journal par défaut invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	logSources, err := a.dependencies.Logs.LogSources(ctx)
	if err != nil {
		http.Error(w, "La liste des journaux n’a pas pu être contrôlée.", http.StatusServiceUnavailable)
		return
	}
	if defaultLog != "" && !slices.Contains(logSources, defaultLog) {
		http.Error(w, "Le journal par défaut sélectionné n’est pas disponible.", http.StatusBadRequest)
		return
	}
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || len(email) > 254 {
			http.Error(w, "Adresse Certbot invalide.", http.StatusBadRequest)
			return
		}
	}
	if err := a.dependencies.Settings.UpdateSettings(ctx, authstore.ApplicationSettings{DefaultLog: defaultLog, CertbotEmail: email, DefaultLanguage: defaultLanguage}); err != nil {
		http.Error(w, "Enregistrement impossible.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, "/setting?result=updated", http.StatusSeeOther)
}

func renderLanguageOptions(selected string) string {
	var result strings.Builder
	for _, item := range []struct{ value, label string }{{"fr", "Français"}, {"en", "English"}} {
		result.WriteString(`<option value="` + item.value + `"`)
		if item.value == selected {
			result.WriteString(` selected`)
		}
		result.WriteString(`>` + item.label + `</option>`)
	}
	return result.String()
}

func renderSettingLogSources(sources []string, selected string, sourceErr error) string {
	if sourceErr != nil {
		return `<input type="hidden" name="default_log" value="` + html.EscapeString(selected) + `"><select disabled><option>Journaux indisponibles</option></select>`
	}
	var result strings.Builder
	result.WriteString(`<select name="default_log"><option value="">Premier journal disponible</option>`)
	for _, source := range sources {
		result.WriteString(`<option value="` + html.EscapeString(source) + `"`)
		if source == selected {
			result.WriteString(` selected`)
		}
		result.WriteString(`>` + html.EscapeString(source) + `</option>`)
	}
	result.WriteString(`</select>`)
	return result.String()
}
func (a *application) updateAdminAccess(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	port, err := strconv.Atoi(r.PostForm.Get("port"))
	address := strings.TrimSpace(r.PostForm.Get("address"))
	allow := strings.TrimSpace(r.PostForm.Get("allow_from"))
	if err != nil || port < 1024 || port > 65535 || address == "" || len(address) > 768 || allow == "" || len(allow) > 64 || strings.ContainsAny(address+allow, "\x00\r") {
		http.Error(w, "Configuration HTTPS invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	_, err = a.dependencies.AdminAccess.UpdateAdminAccess(ctx, websettings.AdminAccess{Enabled: r.PostForm.Get("enabled") == "1", Address: address, Port: port, AllowFrom: allow})
	if err != nil {
		http.Error(w, "La configuration HTTPS n’a pas pu être appliquée.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, "/setting?result=admin-access", http.StatusSeeOther)
}
