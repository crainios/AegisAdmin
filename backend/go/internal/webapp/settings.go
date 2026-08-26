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
	"aegisadmin/backend/internal/websettings"
)

func (a *application) settings(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
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
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{NOTICE}}", notice, "{{LOG_SOURCES}}", renderSettingLogSources(logSources, values.DefaultLog, logsErr), "{{CERTBOT_EMAIL}}", html.EscapeString(values.CertbotEmail), "{{ADMIN_MESSAGE}}", message, "{{ADMIN_ENABLED}}", checked(admin.Enabled), "{{ADMIN_ADDRESS}}", html.EscapeString(admin.Address), "{{ADMIN_PORT}}", strconv.Itoa(admin.Port), "{{ADMIN_ALLOW}}", html.EscapeString(admin.AllowFrom)).Replace(a.settingsPage)
	writeHTML(w, page, http.StatusOK)
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
	if err := a.dependencies.Settings.UpdateSettings(ctx, authstore.ApplicationSettings{DefaultLog: defaultLog, CertbotEmail: email}); err != nil {
		http.Error(w, "Enregistrement impossible.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, "/setting?result=updated", http.StatusSeeOther)
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
