package webapp

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aegisadmin/backend/internal/authstore"
	"aegisadmin/backend/internal/webauth"
	"aegisadmin/backend/internal/websession"
)

var adminLoginPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
var accessEventLabels = map[string]string{"login_success": "Connexion réussie", "login_failure": "Connexion refusée", "two_factor_success": "Double authentification réussie", "two_factor_failure": "Double authentification refusée", "logout": "Déconnexion", "password_changed": "Mot de passe modifié", "two_factor_enabled": "Double authentification activée", "two_factor_disabled": "Double authentification désactivée"}

func (a *application) users(response http.ResponseWriter, request *http.Request) {
	session, user, ok := a.rootUser(response, request)
	if !ok {
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	items, err := a.dependencies.UserAdmin.AdminUsers(ctx)
	if err != nil {
		http.Error(response, "Les utilisateurs n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	notice := ""
	if request.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">L’opération a été enregistrée.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{NOTICE}}", notice, "{{STATISTICS}}", renderUserStatistics(items), "{{USERS}}", renderUserRows(items, user.ID)).Replace(a.usersPage)
	writeHTML(response, page, http.StatusOK)
}

func (a *application) userAccessLog(response http.ResponseWriter, request *http.Request) {
	session, user, ok := a.authenticatedUser(response, request)
	if !ok {
		return
	}
	if user.Type != "root" {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.AccessLog == nil {
		http.Error(response, "Journal des accès indisponible.", http.StatusServiceUnavailable)
		return
	}
	page, _ := strconv.Atoi(request.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	filter := authstore.AccessLogFilter{Login: request.URL.Query().Get("login"), IPAddress: request.URL.Query().Get("ip"), Event: request.URL.Query().Get("event"), Page: page, PerPage: 50}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	result, err := a.dependencies.AccessLog.AccessLog(ctx, filter)
	if err != nil {
		http.Error(response, "Le journal des accès n’a pas pu être chargé.", http.StatusServiceUnavailable)
		return
	}
	logins, err := a.dependencies.AccessLog.AccessLogUsers(ctx)
	if err != nil {
		http.Error(response, "La liste des utilisateurs n’a pas pu être chargée.", http.StatusServiceUnavailable)
		return
	}
	userOptions := `<option value="">Tous</option>`
	for _, login := range logins {
		userOptions += `<option value="` + html.EscapeString(login) + `"` + selected(strings.EqualFold(filter.Login, login)) + `>` + html.EscapeString(login) + `</option>`
	}
	var entries strings.Builder
	for _, item := range result.Items {
		outcome, class := "Refusé", "danger"
		if item.Success {
			outcome, class = "Réussi", "success"
		}
		label := accessEventLabels[item.Event]
		if label == "" {
			label = item.Event
		}
		entries.WriteString(`<tr><td>` + html.EscapeString(item.OccurredAt) + `</td><th>` + html.EscapeString(item.Login) + `</th><td>` + html.EscapeString(label) + `</td><td><span class="status-badge status-badge--` + class + `">` + outcome + `</span></td><td>` + html.EscapeString(item.IPAddress) + `</td><td class="access-log-agent">` + html.EscapeString(item.UserAgent) + `</td></tr>`)
	}
	if len(result.Items) == 0 {
		entries.WriteString(`<tr><td colspan="6" class="muted">Aucun événement.</td></tr>`)
	}
	events := `<option value="">Tous</option>`
	for _, event := range authstore.AccessLogEvents {
		selected := ""
		if filter.Event == event {
			selected = ` selected`
		}
		events += `<option value="` + event + `"` + selected + `>` + html.EscapeString(accessEventLabels[event]) + `</option>`
	}
	query := func(target int) string {
		values := url.Values{}
		if filter.Login != "" {
			values.Set("login", filter.Login)
		}
		if filter.IPAddress != "" {
			values.Set("ip", filter.IPAddress)
		}
		if filter.Event != "" {
			values.Set("event", filter.Event)
		}
		values.Set("page", strconv.Itoa(target))
		return "/users/access-log?" + values.Encode()
	}
	pagination := ""
	if page > 1 {
		pagination += `<a class="secondary-link" href="` + html.EscapeString(query(page-1)) + `">Page précédente</a>`
	}
	if page*result.PerPage < result.Total {
		pagination += `<a class="secondary-link" href="` + html.EscapeString(query(page+1)) + `">Page suivante</a>`
	}
	pageHTML := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{USERS}}", userOptions, "{{IP}}", html.EscapeString(filter.IPAddress), "{{EVENTS}}", events, "{{TOTAL}}", strconv.Itoa(result.Total), "{{ENTRIES}}", entries.String(), "{{PAGINATION}}", pagination).Replace(a.userAccessLogPage)
	writeHTML(response, pageHTML, http.StatusOK)
}

func (a *application) newUser(response http.ResponseWriter, request *http.Request) {
	session, _, ok := a.rootUser(response, request)
	if !ok {
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	modules, err := a.dependencies.UserAdmin.AssignableModules(ctx)
	if err != nil {
		http.Error(response, "Les modules n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	page := a.renderUserCreatePage(session.CSRFToken, modules, nil, authstore.AdminUser{}, "")
	writeHTML(response, page, http.StatusOK)
}

func (a *application) renderUserCreatePage(token string, modules []authstore.AssignableModule, permissions map[int64]string, user authstore.AdminUser, message string) string {
	errorNotice := ""
	if message != "" {
		errorNotice = `<p class="error" role="alert">` + html.EscapeString(message) + `</p>`
	}
	return strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(token),
		"{{ERROR}}", errorNotice,
		"{{LOGIN}}", html.EscapeString(user.Login),
		"{{FIRST_NAME}}", html.EscapeString(user.FirstName),
		"{{LAST_NAME}}", html.EscapeString(user.LastName),
		"{{EMAIL}}", html.EscapeString(user.Email),
		"{{TWO_FACTOR_REQUIRED}}", checked(user.TwoFactorRequired),
		"{{CREATE_PERMISSIONS}}", renderPermissionFields(modules, permissions, ""),
	).Replace(a.userCreatePage)
}

func (a *application) editUser(response http.ResponseWriter, request *http.Request) {
	session, current, ok := a.rootUser(response, request)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	items, err := a.dependencies.UserAdmin.AdminUsers(ctx)
	if err != nil {
		http.Error(response, "L’utilisateur n’a pas pu être chargé.", http.StatusServiceUnavailable)
		return
	}
	var selectedUser *authstore.AdminUser
	for index := range items {
		if items[index].ID == id {
			selectedUser = &items[index]
			break
		}
	}
	if selectedUser == nil {
		http.NotFound(response, request)
		return
	}
	modules, err := a.dependencies.UserAdmin.AssignableModules(ctx)
	if err != nil {
		http.Error(response, "Les modules n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	content := a.renderAdminUsers(ctx, []authstore.AdminUser{*selectedUser}, modules, session.CSRFToken, current.ID)
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{USER}}", content).Replace(a.userEditPage)
	writeHTML(response, page, http.StatusOK)
}

func renderUserStatistics(items []authstore.AdminUser) string {
	active, suspended, twoFactor := 0, 0, 0
	for _, item := range items {
		if item.Status == "active" {
			active++
		} else {
			suspended++
		}
		if item.TwoFactorEnabled {
			twoFactor++
		}
	}
	values := []struct {
		label string
		value int
	}{{"Comptes", len(items)}, {"Actifs", active}, {"Suspendus", suspended}, {"2FA activée", twoFactor}}
	var result strings.Builder
	for _, value := range values {
		result.WriteString(`<article class="summary-item"><span>` + value.label + `</span><strong>` + strconv.Itoa(value.value) + `</strong></article>`)
	}
	return result.String()
}

func renderUserRows(items []authstore.AdminUser, currentID int64) string {
	if len(items) == 0 {
		return `<tr><td colspan="7" class="muted">Aucun compte.</td></tr>`
	}
	var result strings.Builder
	for _, item := range items {
		statusClass, statusLabel := "success", "Actif"
		if item.Status != "active" {
			statusClass, statusLabel = "warning", "Suspendu"
		}
		twoFactor := "Non configurée"
		if item.TwoFactorEnabled {
			twoFactor = "Activée"
		} else if item.TwoFactorRequired {
			twoFactor = "Requise"
		}
		identity := strings.TrimSpace(item.FirstName + " " + item.LastName)
		result.WriteString(`<tr><td><span class="user-actions"><a class="secondary-link compact-link" href="/users/` + strconv.FormatInt(item.ID, 10) + `/edit">Modifier</a>`)
		if item.ID == currentID {
			result.WriteString(`<span class="current-account-indicator" role="img" aria-label="Compte actuellement connecté" title="Compte actuellement connecté"></span>`)
		}
		result.WriteString(`</span></td><th scope="row">` + html.EscapeString(item.Login) + `</th><td>` + html.EscapeString(identity) + `</td><td>` + html.EscapeString(item.Email) + `</td><td>` + html.EscapeString(item.Type) + `</td><td><span class="status-badge status-badge--` + statusClass + `">` + statusLabel + `</span></td><td>` + twoFactor + `</td></tr>`)
	}
	return result.String()
}
func (a *application) renderAdminUsers(ctx context.Context, items []authstore.AdminUser, modules []authstore.AssignableModule, token string, currentID int64) string {
	if len(items) == 0 {
		return `<p class="muted">Aucun compte.</p>`
	}
	var b strings.Builder
	for _, u := range items {
		permissions, _ := a.dependencies.UserAdmin.ModulePermissions(ctx, u.ID)
		b.WriteString(`<article class="content-card"><h3>` + html.EscapeString(u.Login) + `</h3><form class="selector-form" method="post" action="/users/` + strconv.FormatInt(u.ID, 10) + `/update"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><label>Identifiant</label><input name="login" value="` + html.EscapeString(u.Login) + `"`)
		if u.Type == "root" {
			b.WriteString(` readonly`)
		}
		b.WriteString(` required><label>Prénom</label><input name="first_name" value="` + html.EscapeString(u.FirstName) + `" required><label>Nom</label><input name="last_name" value="` + html.EscapeString(u.LastName) + `" required><label>E-mail</label><input type="email" name="email" value="` + html.EscapeString(u.Email) + `" required>`)
		if u.Type != "root" {
			b.WriteString(`<label>État</label><select name="status"><option value="active"` + selected(u.Status == "active") + `>Actif</option><option value="suspended"` + selected(u.Status == "suspended") + `>Suspendu</option></select><label><input type="checkbox" name="two_factor_required" value="1"` + checked(u.TwoFactorRequired) + `> 2FA obligatoire</label><label><input type="checkbox" name="reset_totp" value="1"> Réinitialiser la double authentification</label><fieldset><legend>Droits par module</legend>` + renderPermissionFields(modules, permissions, "permission_") + `</fieldset>`)
		}
		b.WriteString(`<button class="primary-button">Enregistrer</button></form>`)
		if u.Type != "root" {
			b.WriteString(`<form class="selector-form password-generator" method="post" action="/users/` + strconv.FormatInt(u.ID, 10) + `/password" data-password-generator><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><label for="user-password-` + strconv.FormatInt(u.ID, 10) + `">Nouveau mot de passe</label><input id="user-password-` + strconv.FormatInt(u.ID, 10) + `" type="password" name="password" minlength="12" maxlength="128" autocomplete="new-password" spellcheck="false" required data-generated-password><label for="user-password-confirmation-` + strconv.FormatInt(u.ID, 10) + `">Confirmation</label><input id="user-password-confirmation-` + strconv.FormatInt(u.ID, 10) + `" type="password" name="password_confirmation" minlength="12" maxlength="128" autocomplete="new-password" spellcheck="false" required data-generated-password-confirmation><div class="password-generator__actions"><button class="secondary-button" type="button" data-generate-password>Générer un mot de passe</button><button class="secondary-button" type="button" data-toggle-password>Afficher</button><button class="secondary-button" type="button" data-copy-password disabled>Copier</button></div><p class="muted password-generator__status" aria-live="polite" data-password-status>Entre 12 et 128 caractères.</p><button class="secondary-button">Réinitialiser le mot de passe</button></form><form method="post" action="/users/` + strconv.FormatInt(u.ID, 10) + `/delete"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><button class="danger-button">Supprimer</button></form>`)
		}
		b.WriteString(`</article>`)
	}
	return b.String()
}
func renderPermissionFields(modules []authstore.AssignableModule, current map[int64]string, prefix string) string {
	var b strings.Builder
	b.WriteString(`<div class="user-module-permissions-list">`)
	category := ""
	for _, m := range modules {
		if m.Category != category {
			if category != "" {
				b.WriteString(`</div></section>`)
			}
			category = m.Category
			b.WriteString(`<section class="user-module-permissions"><h4 class="user-module-permissions__category">` + html.EscapeString(category) + `</h4><div class="user-module-permissions__items">`)
		}
		name := prefix + strconv.FormatInt(m.ID, 10)
		level := "none"
		if current != nil && current[m.ID] != "" {
			level = current[m.ID]
		}
		b.WriteString(`<fieldset class="user-module-permissions__module"><legend>` + html.EscapeString(m.Name) + `</legend><div class="permission-levels">`)
		for _, option := range []struct{ value, label string }{{"none", "Aucun"}, {"view", "Consultation"}, {"action", "Actions"}, {"modify", "Modification"}} {
			b.WriteString(`<label class="permission-choice"><input type="radio" name="` + name + `" value="` + option.value + `"` + checked(level == option.value) + `><span>` + option.label + `</span></label>`)
		}
		b.WriteString(`</div></fieldset>`)
	}
	if category != "" {
		b.WriteString(`</div></section>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
func selected(v bool) string {
	if v {
		return ` selected`
	}
	return ""
}
func checked(v bool) string {
	if v {
		return ` checked`
	}
	return ""
}

func (a *application) createUser(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	u, permissions, err := a.adminUserForm(r, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	modules, modulesErr := a.dependencies.UserAdmin.AssignableModules(ctx)
	if modulesErr != nil {
		http.Error(w, "Les modules n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	items, usersErr := a.dependencies.UserAdmin.AdminUsers(ctx)
	if usersErr != nil {
		http.Error(w, "Les utilisateurs n’ont pas pu être vérifiés.", http.StatusServiceUnavailable)
		return
	}
	if message := userIdentityConflict(items, u); message != "" {
		writeHTML(w, a.renderUserCreatePage(session.CSRFToken, modules, permissions, u, message), http.StatusConflict)
		return
	}
	hash, err := webauth.HashPassword(r.PostForm.Get("password"))
	if err != nil {
		writeHTML(w, a.renderUserCreatePage(session.CSRFToken, modules, permissions, u, "Le mot de passe doit contenir entre 12 et 128 caractères."), http.StatusBadRequest)
		return
	}
	if _, err = a.dependencies.UserAdmin.CreateAdminUser(ctx, u, hash, permissions); err != nil {
		writeHTML(w, a.renderUserCreatePage(session.CSRFToken, modules, permissions, u, "Le compte n’a pas pu être créé. Vérifiez que l’identifiant et l’adresse e-mail ne sont pas déjà utilisés."), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/users?result=created", http.StatusSeeOther)
}

func userIdentityConflict(items []authstore.AdminUser, candidate authstore.AdminUser) string {
	for _, existing := range items {
		if strings.EqualFold(existing.Email, candidate.Email) {
			return "Cette adresse e-mail est déjà utilisée par un autre utilisateur."
		}
		if strings.EqualFold(existing.Login, candidate.Login) {
			return "Cet identifiant est déjà utilisé par un autre utilisateur."
		}
	}
	return ""
}
func (a *application) updateUser(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, permissions, err := a.adminUserForm(r, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	if err = a.dependencies.UserAdmin.UpdateAdminUser(ctx, u, permissions, r.PostForm.Get("reset_totp") == "1"); err != nil {
		http.Error(w, "Le compte n’a pas pu être modifié.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/users?result=updated", http.StatusSeeOther)
}
func (a *application) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	password := r.PostForm.Get("password")
	if password != r.PostForm.Get("password_confirmation") {
		http.Error(w, "Les deux mots de passe ne correspondent pas.", http.StatusBadRequest)
		return
	}
	hash, err := webauth.HashPassword(password)
	if err != nil {
		http.Error(w, "Mot de passe invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	if err = a.dependencies.UserAdmin.ResetAdminPassword(ctx, id, hash); err != nil {
		http.Error(w, "Réinitialisation impossible.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/users?result=password", http.StatusSeeOther)
}
func (a *application) deleteUser(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	if err = a.dependencies.UserAdmin.DeleteAdminUser(ctx, id); err != nil {
		http.Error(w, "Suppression impossible.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/users?result=deleted", http.StatusSeeOther)
}
func (a *application) rootUser(w http.ResponseWriter, r *http.Request) (websession.Session, authstore.User, bool) {
	session, user, ok := a.authenticatedUser(w, r)
	if !ok {
		return session, user, false
	}
	if user.Type != "root" || a.dependencies.UserAdmin == nil {
		http.Error(w, "Accès interdit.", http.StatusForbidden)
		return session, user, false
	}
	return session, user, true
}
func (a *application) validForm(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	if err := r.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(sessionID, r.PostForm.Get("_token")) {
		http.Error(w, "La requête est invalide.", http.StatusBadRequest)
		return false
	}
	return true
}
func (a *application) adminUserForm(r *http.Request, id int64) (authstore.AdminUser, map[int64]string, error) {
	login := strings.ToLower(strings.TrimSpace(r.PostForm.Get("login")))
	first := strings.TrimSpace(r.PostForm.Get("first_name"))
	last := strings.TrimSpace(r.PostForm.Get("last_name"))
	email := strings.ToLower(strings.TrimSpace(r.PostForm.Get("email")))
	if len(login) < 3 || len(login) > 64 || !adminLoginPattern.MatchString(login) {
		return authstore.AdminUser{}, nil, errors.New("Identifiant invalide.")
	}
	if utf8.RuneCountInString(first) < 1 || utf8.RuneCountInString(first) > 100 || utf8.RuneCountInString(last) < 1 || utf8.RuneCountInString(last) > 100 {
		return authstore.AdminUser{}, nil, errors.New("Nom ou prénom invalide.")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return authstore.AdminUser{}, nil, errors.New("Adresse e-mail invalide.")
	}
	status := r.PostForm.Get("status")
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "suspended" {
		return authstore.AdminUser{}, nil, errors.New("État invalide.")
	}
	modules, err := a.dependencies.UserAdmin.AssignableModules(r.Context())
	if err != nil {
		return authstore.AdminUser{}, nil, err
	}
	permissions := map[int64]string{}
	for _, m := range modules {
		key := "permission_" + strconv.FormatInt(m.ID, 10)
		if id == 0 {
			key = strconv.FormatInt(m.ID, 10)
		}
		level := r.PostForm.Get(key)
		if level == "" {
			level = "none"
		}
		permissions[m.ID] = level
	}
	return authstore.AdminUser{ID: id, Login: login, FirstName: first, LastName: last, Email: email, Status: status, TwoFactorRequired: r.PostForm.Get("two_factor_required") == "1"}, permissions, nil
}
