package webapp

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/authstore"
)

func (a *application) modules(w http.ResponseWriter, r *http.Request) {
	session, current, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if a.dependencies.NavigationAdmin == nil {
		http.Error(w, "Service des modules indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	items, err := a.dependencies.NavigationAdmin.AdminNavigation(ctx)
	if err != nil {
		http.Error(w, "Les modules n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	notice := ""
	if r.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">La navigation a été mise à jour.</p>`
	}
	language := a.languageForUser(ctx, current.ID)
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{NOTICE}}", notice, "{{CATEGORIES}}", renderAdminNavigation(items, session.CSRFToken, language)).Replace(a.modulesPage)
	page = localizeModulesHTML(page, language)
	writeHTML(w, page, http.StatusOK)
}
func renderAdminNavigation(items []authstore.AdminCategory, token, language string) string {
	t := func(fr, en string) string {
		if language == "en" {
			return en
		}
		return fr
	}
	var b strings.Builder
	for _, c := range items {
		id := strconv.FormatInt(c.ID, 10)
		b.WriteString(`<section class="content-card modules-page__category" data-navigation-category data-category-id="` + id + `"><header class="modules-page__category-header"><button class="drag-handle" type="button" draggable="true" data-navigation-category-handle aria-label="` + t("Déplacer la catégorie", "Move category") + `">☰</button><form class="selector-form" method="post" action="/modules/categories/` + id + `/rename"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input name="name" value="` + html.EscapeString(c.Name) + `" minlength="2" maxlength="64" required><button class="secondary-button">` + t("Renommer", "Rename") + `</button></form><div class="header-actions fallback-move-actions">` + moveForm("/modules/categories/"+id+"/move", -1, t("Monter", "Move up"), token) + moveForm("/modules/categories/"+id+"/move", 1, t("Descendre", "Move down"), token) + `</div></header>`)
		if len(c.Modules) == 0 {
			b.WriteString(`<div class="table-scroll"><table class="data-table"><tbody data-navigation-module-list><tr data-navigation-empty-row><td class="muted">` + t("Déposez un module dans cette catégorie.", "Drop a module into this category.") + `</td></tr></tbody></table></div><form method="post" action="/modules/categories/` + id + `/delete"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><button class="danger-button">` + t("Supprimer la catégorie", "Delete category") + `</button></form>`)
		} else {
			b.WriteString(`<div class="table-scroll"><table class="data-table modules-table"><thead><tr><th>` + t("Ordre", "Order") + `</th><th>Module</th><th>Route</th><th>` + t("Accès", "Access") + `</th><th>` + t("État", "Status") + `</th></tr></thead><tbody data-navigation-module-list>`)
			for _, m := range c.Modules {
				mid := strconv.FormatInt(m.ID, 10)
				state := t("Désactivé", "Disabled")
				if m.Enabled {
					state = t("Activé", "Enabled")
				}
				b.WriteString(`<tr class="modules-page__module" draggable="true" data-navigation-module data-module-id="` + mid + `"><td class="modules-order"><button class="drag-handle" type="button" data-navigation-module-handle aria-label="` + t("Déplacer le module", "Move module") + `">☰</button><span class="fallback-move-actions">` + moveForm("/modules/"+mid+"/move", -1, "↑", token) + moveForm("/modules/"+mid+"/move", 1, "↓", token) + `</span></td><th><div class="module-name-line"><span>` + html.EscapeString(m.Icon+" "+m.Name) + `</span>`)
				if !m.Essential {
					b.WriteString(`<form class="inline-form" method="post" action="/modules/` + mid + `/toggle"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input type="hidden" name="enabled" value="` + strconv.FormatBool(!m.Enabled) + `"><button class="secondary-button module-toggle-button">` + map[bool]string{true: t("Désactiver", "Disable"), false: t("Activer", "Enable")}[m.Enabled] + `</button></form>`)
				} else {
					b.WriteString(`<small class="muted">` + t("essentiel", "essential") + `</small>`)
				}
				b.WriteString(`</div></th><td><code>` + html.EscapeString(m.Route) + `</code></td><td>` + html.EscapeString(m.AccessPolicy) + `</td><td>` + state + `</td></tr>`)
			}
			b.WriteString(`</tbody></table></div>`)
		}
		b.WriteString(`</section>`)
	}
	return b.String()
}
func moveForm(action string, direction int, label, token string) string {
	return `<form method="post" action="` + action + `"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input type="hidden" name="direction" value="` + strconv.Itoa(direction) + `"><button class="secondary-button">` + label + `</button></form>`
}

func localizeModulesHTML(value, language string) string {
	if language != "en" {
		return value
	}
	return strings.NewReplacer(`lang="fr"`, `lang="en"`, "Organisez le menu, ses catégories et les modules disponibles.", "Organize the menu, its categories and available modules.", "Se déconnecter", "Sign out", "La navigation a été mise à jour.", "Navigation was updated.", "Organisation du menu", "Menu organization", "Faites glisser les poignées pour déplacer une catégorie ou un module. Un module peut passer d’une catégorie à une autre.", "Drag the handles to move a category or module. A module can be moved between categories.", "Nouvelle catégorie", "New category", "Ajouter", "Add").Replace(value)
}
func (a *application) navigationAction(w http.ResponseWriter, r *http.Request, operation func(int64) error) {
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
	if err = operation(id); err != nil {
		http.Error(w, "La navigation n’a pas pu être modifiée.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/modules?result=updated", http.StatusSeeOther)
}
func (a *application) createCategory(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	if len(name) < 2 || len(name) > 64 {
		http.Error(w, "Nom invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	if err := a.dependencies.NavigationAdmin.CreateCategory(ctx, name); err != nil {
		http.Error(w, "Création impossible.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/modules?result=created", http.StatusSeeOther)
}
func (a *application) renameCategory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	a.navigationAction(w, r, func(id int64) error {
		name := strings.TrimSpace(r.PostForm.Get("name"))
		if len(name) < 2 || len(name) > 64 {
			return fmt.Errorf("invalid name")
		}
		return a.dependencies.NavigationAdmin.RenameCategory(ctx, id, name)
	})
}
func (a *application) deleteCategory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	a.navigationAction(w, r, func(id int64) error { return a.dependencies.NavigationAdmin.DeleteCategory(ctx, id) })
}
func (a *application) toggleModule(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	a.navigationAction(w, r, func(id int64) error {
		return a.dependencies.NavigationAdmin.ToggleModule(ctx, id, r.PostForm.Get("enabled") == "true")
	})
}
func (a *application) moveCategory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	a.navigationAction(w, r, func(id int64) error {
		direction, _ := strconv.Atoi(r.PostForm.Get("direction"))
		return a.dependencies.NavigationAdmin.MoveCategory(ctx, id, direction)
	})
}
func (a *application) moveModule(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	a.navigationAction(w, r, func(id int64) error {
		direction, _ := strconv.Atoi(r.PostForm.Get("direction"))
		return a.dependencies.NavigationAdmin.MoveModule(ctx, id, direction)
	})
}

func (a *application) reorderNavigation(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	var payload struct {
		Token               string             `json:"_token"`
		CategoryIDs         []int64            `json:"categoryIds"`
		ModuleIDsByCategory map[string][]int64 `json:"moduleIdsByCategory"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, payload.Token) {
		writeJSON(w, map[string]any{"success": false, "message": "La requête est invalide."}, http.StatusBadRequest)
		return
	}
	modules := make(map[int64][]int64, len(payload.ModuleIDsByCategory))
	for key, ids := range payload.ModuleIDsByCategory {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, map[string]any{"success": false, "message": "Une catégorie est invalide."}, http.StatusBadRequest)
			return
		}
		modules[id] = ids
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	if err := a.dependencies.NavigationAdmin.ReorderNavigation(ctx, payload.CategoryIDs, modules); err != nil {
		writeJSON(w, map[string]any{"success": false, "message": "L’organisation du menu n’a pas pu être enregistrée."}, http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"success": true, "message": "L’organisation du menu a été enregistrée."}, http.StatusOK)
}
