package webapp

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/webcertbot"
	"aegisadmin/backend/internal/webupdates"
)

func (a *application) certbot(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "certbot")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Certbot == nil {
		http.Error(response, "Service Certbot indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := contextWithTimeout(request, 20*time.Second)
	defer cancel()
	s, err := a.dependencies.Certbot.Snapshot(ctx)
	if err != nil {
		http.Error(response, "Les certificats TLS n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	token := html.EscapeString(session.CSRFToken)
	canAct := level == "action" || level == "modify"
	actions, header := "", ""
	if canAct {
		actions = certbotButton("renew-test", "Tester le renouvellement", "secondary-button", "", token) + certbotButton("renew", "Renouveler", "primary-button", "", token)
		header = "<th>Actions</th>"
	}
	notice := ""
	if id := request.URL.Query().Get("execution"); id != "" {
		notice = `<p class="notice notice--success">Action programmée. <a href="/certbot/actions/` + url.PathEscape(id) + `">Consulter le résultat</a>.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{"Produit", s.Info.Product + " " + s.Info.Version}, {"Installation", s.Info.Installation}, {"Exécutable", s.Info.Executable}, {"Greffons", strings.Join(s.Info.Plugins, ", ")}}), "{{STATUS}}", renderPairs([][2]string{{"Minuteur présent", yesNo(s.Status.TimerExists)}, {"Minuteur actif", yesNo(s.Status.TimerActive)}, {"Activé au démarrage", yesNo(s.Status.TimerEnabled)}, {"Dernier résultat", s.Status.LastResult}}), "{{ACTIONS}}", actions, "{{ACTION_HEADER}}", header, "{{CERTIFICATES}}", renderCertificates(s.Certificates, token, canAct)).Replace(a.certbotPage)
	writeHTML(response, page, http.StatusOK)
}

func (a *application) certbotAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "certbot")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 16*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	action := request.PathValue("action")
	args := []string{}
	name := request.PostForm.Get("certificate")
	switch action {
	case "renew-test", "renew":
	case "delete", "reinstall", "renew-replace":
		args = []string{name}
	case "issue":
		domains := strings.Fields(request.PostForm.Get("domains"))
		args = append([]string{request.PostForm.Get("email"), request.PostForm.Get("redirect")}, domains...)
	default:
		http.NotFound(response, request)
		return
	}
	ctx, cancel := contextWithTimeout(request, 20*time.Second)
	defer cancel()
	id, err := a.dependencies.Certbot.Start(ctx, action, args)
	if err != nil {
		http.Error(response, "L’action Certbot a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/certbot?execution="+url.QueryEscape(id), http.StatusSeeOther)
}
func (a *application) certbotResult(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	_, granted, ok := a.modulePermission(response, request, user, "certbot")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	result, err := a.dependencies.Certbot.Result(ctx, request.PathValue("id"))
	if err != nil {
		http.Error(response, "Résultat indisponible.", http.StatusNotFound)
		return
	}
	writeJSON(response, result, http.StatusOK)
}
func renderCertificates(items []webcertbot.Certificate, token string, actions bool) string {
	columns := 5
	if actions {
		columns++
	}
	if len(items) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">Aucun certificat détecté.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		state, class := "Expiré", "danger"
		if i.Valid {
			state, class = "Valide", "success"
			if i.DaysRemaining <= 30 {
				state, class = "À renouveler", "warning"
			}
		}
		b.WriteString(`<tr>`)
		if actions {
			b.WriteString(`<td><select class="compact-select" aria-label="Action pour ` + html.EscapeString(i.Name) + `" data-certbot-certificate-action data-certificate="` + html.EscapeString(i.Name) + `" data-csrf="` + token + `"><option value="">Actions…</option><option value="reinstall">Réinstaller</option><option value="renew-replace">Renouveler</option><option value="delete">Supprimer</option></select></td>`)
		}
		b.WriteString(`<th>` + html.EscapeString(i.Name) + `</th><td>` + html.EscapeString(strings.Join(i.Domains, ", ")) + `</td><td>` + html.EscapeString(i.KeyType) + `</td><td>` + html.EscapeString(i.Expiry) + ` (` + strconv.Itoa(i.DaysRemaining) + ` j)</td><td><span class="status-badge status-badge--` + class + `">` + state + `</span></td></tr>`)
	}
	return b.String()
}
func certbotButton(action, label, class, name, token string) string {
	return `<form method="post" action="/certbot/` + action + `"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="certificate" value="` + html.EscapeString(name) + `"><button class="` + class + `">` + label + `</button></form>`
}

func (a *application) updates(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "updates")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	ctx, cancel := contextWithTimeout(request, 40*time.Second)
	defer cancel()
	s, err := a.dependencies.Updates.Snapshot(ctx)
	if err != nil {
		http.Error(response, "Les mises à jour n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	token := html.EscapeString(session.CSRFToken)
	actions := ""
	if level == "modify" {
		actions = `<div class="section-actions"><form method="post" action="/updates/start"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button">Installer les mises à jour</button></form></div>`
	}
	composerAction := ""
	if level == "modify" && s.Composer.Available {
		composerAction = `<form method="post" action="/updates/composer-refresh"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">Vérifier maintenant</button></form>`
	}
	composerStatus := "Dernière vérification : jamais"
	if s.Composer.Refreshing {
		composerStatus = "Vérification Composer en cours…"
	} else if s.Composer.RefreshedAt != nil && *s.Composer.RefreshedAt != "" {
		composerStatus = "Dernière vérification : " + *s.Composer.RefreshedAt
	}
	composerState := ""
	if !s.Composer.Available || len(s.Composer.Sites) == 0 {
		message := s.Composer.Message
		if message == "" && s.Composer.Refreshing {
			message = "La première vérification des sites est en cours."
		}
		if message == "" {
			message = "Aucun projet Composer détecté."
		}
		composerState = `<p class="notice">` + html.EscapeString(message) + `</p>`
	}
	notice := ""
	if id := request.URL.Query().Get("job"); id != "" {
		notice = `<p class="notice notice--success">Mise à jour démarrée. <a href="/updates/jobs/` + url.PathEscape(id) + `">Suivre la progression</a>.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{SUMMARY}}", renderMetricPairs([][2]string{{"Gestionnaire", s.Info.Backend}, {"Paquets", strconv.Itoa(s.Info.UpdateCount)}, {"Sécurité", strconv.Itoa(s.Info.SecurityUpdateCount)}, {"Redémarrage", yesNo(s.Info.RebootRequired)}}), "{{ACTIONS}}", actions, "{{PACKAGES}}", renderUpdates(s.Updates), "{{FIRMWARE}}", renderFirmware(s.Firmware.Updates), "{{COMPOSER}}", renderComposer(s.Composer.Sites), "{{COMPOSER_ACTION}}", composerAction, "{{COMPOSER_STATUS}}", html.EscapeString(composerStatus), "{{COMPOSER_STATE}}", composerState, "{{COMPOSER_REFRESHING}}", strconv.FormatBool(s.Composer.Refreshing)).Replace(a.updatesPage)
	writeHTML(response, page, http.StatusOK)
}
func (a *application) startUpdates(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "updates")
	if !ok {
		return
	}
	if !granted || level != "modify" {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "Requête invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(request, 20*time.Second)
	defer cancel()
	id, err := a.dependencies.Updates.StartUpgrade(ctx)
	if err != nil {
		http.Error(response, "La mise à jour n’a pas pu démarrer.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/updates?job="+url.QueryEscape(id), http.StatusSeeOther)
}
func (a *application) updatesJob(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	_, granted, ok := a.modulePermission(response, request, user, "updates")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	job, err := a.dependencies.Updates.Job(ctx, request.PathValue("id"))
	if err != nil {
		http.Error(response, "Suivi indisponible.", http.StatusNotFound)
		return
	}
	writeJSON(response, job, http.StatusOK)
}
func (a *application) refreshComposer(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "updates")
	if !ok {
		return
	}
	if !granted || level != "modify" {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "Requête invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	if err := a.dependencies.Updates.RefreshComposer(ctx); err != nil {
		http.Error(response, "Actualisation impossible.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/updates", http.StatusSeeOther)
}
func renderUpdates(items []webupdates.Update) string {
	if len(items) == 0 {
		return `<tr><td colspan="5" class="muted">Le système est à jour.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		b.WriteString(`<tr><th>` + html.EscapeString(i.Name) + `</th><td>` + html.EscapeString(i.Architecture) + `</td><td>` + html.EscapeString(i.InstalledVersion) + `</td><td>` + html.EscapeString(i.CandidateVersion) + `</td><td>` + yesNo(i.Security) + `</td></tr>`)
	}
	return b.String()
}
func renderFirmware(items []webupdates.FirmwareUpdate) string {
	if len(items) == 0 {
		return `<tr><td colspan="5" class="muted">Aucune mise à jour de firmware.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		b.WriteString(`<tr><th>` + html.EscapeString(i.Device) + `</th><td>` + html.EscapeString(i.Vendor) + `</td><td>` + html.EscapeString(i.InstalledVersion) + `</td><td>` + html.EscapeString(i.CandidateVersion) + `</td><td>` + yesNo(i.RebootRequired) + `</td></tr>`)
	}
	return b.String()
}
func renderComposer(items []webupdates.ComposerSite) string {
	if len(items) == 0 {
		return `<tr><td colspan="4" class="muted">Aucun projet Composer détecté.</td></tr>`
	}
	var b strings.Builder
	for index, i := range items {
		statusClass, statusLabel := "success", "À jour"
		if i.Stale {
			statusClass, statusLabel = "neutral", "À revérifier"
		} else if i.Status == "outdated" {
			statusClass, statusLabel = "warning", strconv.Itoa(i.UpdateCount)+" mise(s) à jour"
		} else if i.Status == "error" {
			statusClass, statusLabel = "danger", "Erreur"
		}
		securityClass, securityLabel := "success", "Aucune alerte"
		if i.SecurityStatus == "error" {
			securityClass, securityLabel = "neutral", "Indisponible"
		} else if i.SecurityIssueCount > 0 {
			securityClass, securityLabel = "danger", strconv.Itoa(i.SecurityIssueCount)+" alerte(s)"
		}
		b.WriteString(`<tr><td>` + renderComposerDetails(i, index) + `</td><th>` + html.EscapeString(i.Domain) + `</th><td><span class="status-badge status-badge--` + statusClass + `">` + html.EscapeString(statusLabel) + `</span></td><td><span class="status-badge status-badge--` + securityClass + `">` + html.EscapeString(securityLabel) + `</span></td></tr>`)
	}
	return b.String()
}

func renderComposerDetails(site webupdates.ComposerSite, index int) string {
	if len(site.Packages) == 0 && len(site.SecurityAdvisories) == 0 && site.Message == "" {
		return `<span class="muted">—</span>`
	}
	var b strings.Builder
	id := "composer-detail-" + strconv.Itoa(index)
	b.WriteString(`<button class="secondary-button compact-link" type="button" data-composer-detail-open="` + id + `">Détails</button><template id="` + id + `"><div class="composer-details"><p><strong>Site : </strong>` + html.EscapeString(site.Domain) + `</p>`)
	if site.Message != "" {
		b.WriteString(`<p class="notice">` + html.EscapeString(site.Message) + `</p>`)
	}
	b.WriteString(`<h3>Alertes de sécurité</h3><div class="table-scroll"><table class="data-table data-table--nested"><thead><tr><th>Paquet</th><th>Alerte</th><th>Versions affectées</th><th>Référence</th></tr></thead><tbody>`)
	if len(site.SecurityAdvisories) == 0 {
		b.WriteString(`<tr><td colspan="4" class="muted">Aucune alerte.</td></tr>`)
	}
	for _, advisory := range site.SecurityAdvisories {
		link := `<span class="muted">—</span>`
		if parsed, err := url.Parse(advisory.Link); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			link = `<a href="` + html.EscapeString(parsed.String()) + `" target="_blank" rel="noopener noreferrer">` + html.EscapeString(advisory.ID) + `</a>`
		}
		b.WriteString(`<tr><th>` + html.EscapeString(advisory.Package) + `</th><td>` + html.EscapeString(advisory.Title) + `</td><td>` + html.EscapeString(advisory.AffectedVersions) + `</td><td>` + link + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></div><h3>Mises à jour disponibles</h3><div class="table-scroll"><table class="data-table data-table--nested"><thead><tr><th>Paquet</th><th>Installée</th><th>Disponible</th><th>Compatibilité</th><th>Type</th></tr></thead><tbody>`)
	if len(site.Packages) == 0 {
		b.WriteString(`<tr><td colspan="5" class="muted">Aucune mise à jour.</td></tr>`)
	}
	for _, pkg := range site.Packages {
		direct := "Indirecte"
		if pkg.Direct {
			direct = "Directe"
		}
		b.WriteString(`<tr><th>` + html.EscapeString(pkg.Name) + `</th><td>` + html.EscapeString(pkg.CurrentVersion) + `</td><td>` + html.EscapeString(pkg.LatestVersion) + `</td><td>` + html.EscapeString(pkg.Status) + `</td><td>` + direct + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></div></div></template>`)
	return b.String()
}
func writeJSON(response http.ResponseWriter, value any, status int) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
func contextWithTimeout(request *http.Request, duration time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(request.Context(), duration)
}
