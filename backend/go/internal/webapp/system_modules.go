package webapp

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/i18n"
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
	language := a.languageForUser(request.Context(), user.ID)
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
	if !s.Info.Installed {
		page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", `<p class="notice">`+html.EscapeString(i18n.Text(language, "certbot.not_installed_notice"))+`</p>`, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "certbot.installation"), i18n.Text(language, "certbot.not_installed")}, {i18n.Text(language, "certbot.function"), i18n.Text(language, "certbot.function_value")}}), "{{STATUS}}", renderPairs([][2]string{{i18n.Text(language, "certbot.timer_exists"), i18n.Text(language, "common.no")}, {i18n.Text(language, "certbot.timer_active"), i18n.Text(language, "common.no")}, {i18n.Text(language, "certbot.autostart"), i18n.Text(language, "common.no")}, {i18n.Text(language, "certbot.last_result"), i18n.Text(language, "certbot.unavailable")}}), "{{ACTIONS}}", "", "{{ACTION_HEADER}}", "", "{{CERTIFICATES}}", renderCertificates(nil, token, false, language)).Replace(i18n.Localize(a.certbotPage, language))
		writeHTML(response, page, http.StatusOK)
		return
	}
	canAct := level == "action" || level == "modify"
	actions, header := "", ""
	if canAct {
		actions = certbotButton("renew-test", i18n.Text(language, "certbot.test_renewal"), "secondary-button", "", token) + certbotButton("renew", i18n.Text(language, "certbot.renew"), "primary-button", "", token)
		header = "<th>" + html.EscapeString(i18n.Text(language, "common.actions")) + "</th>"
	}
	notice := ""
	if id := request.URL.Query().Get("execution"); id != "" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "certbot.action_scheduled")) + ` <button class="secondary-button compact-link" type="button" data-certbot-result-open="` + html.EscapeString(id) + `">` + html.EscapeString(i18n.Text(language, "certbot.view_result")) + `</button></p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "certbot.product"), s.Info.Product + " " + s.Info.Version}, {i18n.Text(language, "certbot.installation"), s.Info.Installation}, {i18n.Text(language, "certbot.executable"), s.Info.Executable}, {i18n.Text(language, "certbot.plugins"), strings.Join(s.Info.Plugins, ", ")}}), "{{STATUS}}", renderPairs([][2]string{{i18n.Text(language, "certbot.timer_exists"), yesNoForLanguage(s.Status.TimerExists, language)}, {i18n.Text(language, "certbot.timer_active"), yesNoForLanguage(s.Status.TimerActive, language)}, {i18n.Text(language, "certbot.autostart"), yesNoForLanguage(s.Status.TimerEnabled, language)}, {i18n.Text(language, "certbot.last_result"), s.Status.LastResult}}), "{{ACTIONS}}", actions, "{{ACTION_HEADER}}", header, "{{CERTIFICATES}}", renderCertificates(s.Certificates, token, canAct, language)).Replace(i18n.Localize(a.certbotPage, language))
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
	if !strings.Contains(request.Header.Get("Accept"), "application/json") {
		http.Redirect(response, request, "/certbot?execution="+url.QueryEscape(request.PathValue("id")), http.StatusSeeOther)
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
func renderCertificates(items []webcertbot.Certificate, token string, actions bool, language string) string {
	columns := 5
	if actions {
		columns++
	}
	if len(items) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">` + html.EscapeString(i18n.Text(language, "certbot.empty")) + `</td></tr>`
	}
	items = append([]webcertbot.Certificate(nil), items...)
	sort.SliceStable(items, func(left, right int) bool {
		leftDomains := strings.ToLower(strings.Join(items[left].Domains, ", "))
		rightDomains := strings.ToLower(strings.Join(items[right].Domains, ", "))
		if leftDomains == rightDomains {
			return strings.ToLower(items[left].Name) < strings.ToLower(items[right].Name)
		}
		return leftDomains < rightDomains
	})
	var b strings.Builder
	for _, i := range items {
		state, class := i18n.Text(language, "certbot.expired"), "danger"
		if i.Valid {
			state, class = i18n.Text(language, "certbot.valid"), "success"
			if i.DaysRemaining <= 30 {
				state, class = i18n.Text(language, "certbot.renew_due"), "warning"
			}
		}
		b.WriteString(`<tr>`)
		if actions {
			b.WriteString(`<td><select class="compact-select" aria-label="` + html.EscapeString(i18n.Text(language, "certbot.action_for")) + ` ` + html.EscapeString(i.Name) + `" data-certbot-certificate-action data-certificate="` + html.EscapeString(i.Name) + `" data-csrf="` + token + `"><option value="">` + html.EscapeString(i18n.Text(language, "common.actions")) + `…</option><option value="reinstall">` + html.EscapeString(i18n.Text(language, "certbot.reinstall")) + `</option><option value="renew-replace">` + html.EscapeString(i18n.Text(language, "certbot.renew")) + `</option><option value="delete">` + html.EscapeString(i18n.Text(language, "common.delete")) + `</option></select></td>`)
		}
		b.WriteString(`<th>` + html.EscapeString(i.Name) + `</th><td>` + html.EscapeString(strings.Join(i.Domains, ", ")) + `</td><td>` + html.EscapeString(i.KeyType) + `</td><td>` + html.EscapeString(i.Expiry) + ` (` + strconv.Itoa(i.DaysRemaining) + ` ` + html.EscapeString(i18n.Text(language, "certbot.day_short")) + `)</td><td><span class="status-badge status-badge--` + class + `">` + state + `</span></td></tr>`)
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
	language := a.languageForUser(request.Context(), user.ID)
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
	if user.Type == "root" && s.Info.RebootRequired {
		actions += `<div class="notice notice--warning reboot-required"><h3>Redémarrage requis</h3><p>Une mise à jour installée nécessite le redémarrage du serveur.</p><form class="selector-form" method="post" action="/updates/reboot"><input type="hidden" name="_token" value="` + token + `"><label>Délai<select name="delay"><option value="0">Maintenant</option><option value="5">Dans 5 minutes</option><option value="15">Dans 15 minutes</option><option value="30">Dans 30 minutes</option><option value="60">Dans 1 heure</option></select></label><label class="checkbox-line"><input type="checkbox" name="confirmation" value="reboot" required> Je confirme le redémarrage du serveur et l’interruption des services.</label><button class="danger-button">Redémarrer le serveur</button></form></div>`
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
	if delay := request.URL.Query().Get("reboot"); delay == "now" {
		notice = `<p class="notice notice--warning">Le redémarrage immédiat du serveur a été programmé.</p>`
	} else if delay != "" {
		notice = `<p class="notice notice--warning">Le redémarrage du serveur a été programmé dans ` + html.EscapeString(delay) + ` minutes.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{SUMMARY}}", renderMetricPairs([][2]string{{"Gestionnaire", s.Info.Backend}, {"Paquets", formatIntegerForLanguage(int64(s.Info.UpdateCount), language)}, {"Sécurité", formatIntegerForLanguage(int64(s.Info.SecurityUpdateCount), language)}, {"Redémarrage", yesNoForLanguage(s.Info.RebootRequired, language)}}), "{{ACTIONS}}", actions, "{{PACKAGES}}", renderUpdates(s.Updates, language), "{{FIRMWARE}}", renderFirmware(s.Firmware.Updates, language), "{{COMPOSER}}", renderComposer(s.Composer.Sites, language), "{{COMPOSER_ACTION}}", composerAction, "{{COMPOSER_STATUS}}", html.EscapeString(composerStatus), "{{COMPOSER_STATE}}", composerState, "{{COMPOSER_REFRESHING}}", strconv.FormatBool(s.Composer.Refreshing)).Replace(a.updatesPage)
	page = localizeUpdatesHTML(page, language)
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

func (a *application) rebootServer(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if user.Type != "root" {
		http.Error(response, "Accès réservé au compte root.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) || request.PostForm.Get("confirmation") != "reboot" {
		http.Error(response, "Confirmation de redémarrage invalide.", http.StatusBadRequest)
		return
	}
	delay, err := strconv.Atoi(request.PostForm.Get("delay"))
	if err != nil || (delay != 0 && delay != 5 && delay != 15 && delay != 30 && delay != 60) {
		http.Error(response, "Délai de redémarrage invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	if err = a.dependencies.Updates.Reboot(ctx, delay); err != nil {
		a.recordAccess(request, &user, user.Login, "server_reboot_scheduled", false)
		http.Error(response, "Le redémarrage ne peut pas être programmé pendant une mise à jour active ou lorsque systemd est indisponible.", http.StatusServiceUnavailable)
		return
	}
	a.recordAccess(request, &user, user.Login, "server_reboot_scheduled", true)
	result := strconv.Itoa(delay)
	if delay == 0 {
		result = "now"
	}
	http.Redirect(response, request, "/updates?reboot="+url.QueryEscape(result), http.StatusSeeOther)
}
func (a *application) updatesJob(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := contextWithTimeout(request, 10*time.Second)
	defer cancel()
	job, err := a.dependencies.Updates.Job(ctx, request.PathValue("id"))
	if err != nil {
		http.Error(response, "Suivi indisponible.", http.StatusNotFound)
		return
	}
	// Les sessions web sont volontairement en mémoire et disparaissent quand le
	// paquet redémarre le serveur. L’identifiant aléatoire de 128 bits sert alors
	// de capacité temporaire pour que la modale puisse lire la fin de son travail.
	if _, sessionFound := a.requestSession(request); sessionFound {
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
	} else {
		started, parseErr := time.Parse(time.RFC3339, job.StartedAt)
		age := time.Since(started)
		if parseErr != nil || age < -5*time.Minute || age > 4*time.Hour {
			http.Error(response, "Suivi indisponible.", http.StatusNotFound)
			return
		}
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Referrer-Policy", "no-referrer")
	response.Header().Set("X-Robots-Tag", "noindex, nofollow")
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
func renderUpdates(items []webupdates.Update, language string) string {
	if len(items) == 0 {
		return `<tr><td colspan="5" class="muted">Le système est à jour.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		b.WriteString(`<tr><th>` + html.EscapeString(i.Name) + `</th><td>` + html.EscapeString(i.Architecture) + `</td><td>` + html.EscapeString(i.InstalledVersion) + `</td><td>` + html.EscapeString(i.CandidateVersion) + `</td><td>` + yesNoForLanguage(i.Security, language) + `</td></tr>`)
	}
	return b.String()
}
func renderFirmware(items []webupdates.FirmwareUpdate, language string) string {
	if len(items) == 0 {
		return `<tr><td colspan="5" class="muted">Aucune mise à jour de firmware.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		b.WriteString(`<tr><th>` + html.EscapeString(i.Device) + `</th><td>` + html.EscapeString(i.Vendor) + `</td><td>` + html.EscapeString(i.InstalledVersion) + `</td><td>` + html.EscapeString(i.CandidateVersion) + `</td><td>` + yesNoForLanguage(i.RebootRequired, language) + `</td></tr>`)
	}
	return b.String()
}
func renderComposer(items []webupdates.ComposerSite, language string) string {
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
		b.WriteString(`<tr><td>` + renderComposerDetails(i, index, language) + `</td><th>` + html.EscapeString(i.Domain) + `</th><td><span class="status-badge status-badge--` + statusClass + `">` + html.EscapeString(localizeUpdatesText(statusLabel, language)) + `</span></td><td><span class="status-badge status-badge--` + securityClass + `">` + html.EscapeString(localizeUpdatesText(securityLabel, language)) + `</span></td></tr>`)
	}
	return b.String()
}

func renderComposerDetails(site webupdates.ComposerSite, index int, language string) string {
	if len(site.Packages) == 0 && len(site.SecurityAdvisories) == 0 && site.Message == "" {
		return `<span class="muted">—</span>`
	}
	var b strings.Builder
	id := "composer-detail-" + strconv.Itoa(index)
	b.WriteString(`<button class="secondary-button compact-link" type="button" data-composer-detail-open="` + id + `">Détails</button><template id="` + id + `"><div class="composer-details"><p><strong>Site : </strong>` + html.EscapeString(site.Domain) + `</p>`)
	if site.Message != "" {
		b.WriteString(`<p class="notice">` + html.EscapeString(site.Message) + `</p>`)
	}
	if site.SecurityStatus == "error" && site.SecurityMessage != "" {
		b.WriteString(`<p class="notice notice--danger">` + html.EscapeString(site.SecurityMessage) + `</p>`)
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
	return localizeUpdatesHTML(b.String(), language)
}

func localizeUpdatesText(value, language string) string {
	if language != "en" {
		return value
	}
	value = strings.ReplaceAll(value, " mise(s) à jour", " update(s)")
	value = strings.ReplaceAll(value, " alerte(s)", " alert(s)")
	return strings.NewReplacer("À jour", "Up to date", "À revérifier", "Check again", "Erreur", "Error", "Aucune alerte", "No alert", "Indisponible", "Unavailable").Replace(value)
}

func localizeUpdatesHTML(value, language string) string {
	if language != "en" {
		return value
	}
	return strings.NewReplacer(
		`lang="fr"`, `lang="en"`, "Administration", "Administration", "État des mises à jour", "Update status", "Paquets disponibles", "Available packages", "Mises à jour disponibles", "Available updates", "Mises à jour", "Updates", "Paquets système, dépendances Composer et firmwares · Droit", "System packages, Composer dependencies and firmware · Permission",
		"Se déconnecter", "Sign out", "État des mises à jour", "Update status", "Gestionnaire", "Manager", "Paquets", "Packages", "Sécurité", "Security", "Redémarrage", "Reboot", "Installer les mises à jour", "Install updates",
		"Paquets disponibles", "Available packages", "Paquet", "Package", "Architecture", "Architecture", "Installée", "Installed", "Candidate", "Candidate", "Dépendances Composer", "Composer dependencies", "Vérifier maintenant", "Check now", "Détail", "Details", "Site", "Site",
		"Firmwares disponibles", "Available firmware", "Périphérique", "Device", "Fournisseur", "Vendor", "Installé", "Installed", "Fermer", "Close", "Installation des mises à jour", "Installing updates", "Initialisation…", "Initializing…", "Gestionnaire de paquets", "Package manager", "Connexion…", "Connecting…", "Connexion au suivi de la mise à jour…", "Connecting to update monitoring…", "Démarrage", "Started", "Fin", "Finished", "Code retour", "Exit code",
		"Le système est à jour.", "The system is up to date.", "Aucune mise à jour de firmware.", "No firmware update.", "Aucun projet Composer détecté.", "No Composer project detected.", "Détails", "Details", "Alertes de sécurité", "Security alerts", "Alerte", "Advisory", "Versions affectées", "Affected versions", "Référence", "Reference", "Aucune alerte.", "No alert.", "Mises à jour disponibles", "Available updates", "Disponible", "Available", "Compatibilité", "Compatibility", "Type", "Type", "Aucune mise à jour.", "No update.", "Indirecte", "Indirect", "Directe", "Direct",
		"Redémarrage requis", "Reboot required", "Une mise à jour installée nécessite le redémarrage du serveur.", "An installed update requires a server reboot.", "Délai", "Delay", "Maintenant", "Now", "Dans 5 minutes", "In 5 minutes", "Dans 15 minutes", "In 15 minutes", "Dans 30 minutes", "In 30 minutes", "Dans 1 heure", "In 1 hour", "Je confirme le redémarrage du serveur et l’interruption des services.", "I confirm the server reboot and service interruption.", "Redémarrer le serveur", "Reboot server",
		"Dernière vérification : jamais", "Last check: never", "Vérification Composer en cours…", "Composer check in progress…", "Dernière vérification :", "Last check:", "La première vérification des sites est en cours.", "The first site check is in progress.", "Le redémarrage immédiat du serveur a été programmé.", "The immediate server reboot was scheduled.", "Le redémarrage du serveur a été programmé dans", "The server reboot was scheduled in",
	).Replace(value)
}
func writeJSON(response http.ResponseWriter, value any, status int) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
func contextWithTimeout(request *http.Request, duration time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(request.Context(), duration)
}
