package webapp

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *application) configuration(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if a.dependencies.Configuration == nil {
		http.Error(w, "Service de configuration indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := contextWithTimeout(r, 60*time.Second)
	defer cancel()
	inventory, err := a.dependencies.Configuration.Call(ctx, "inventory", nil)
	if err != nil {
		http.Error(w, "L’inventaire n’a pas pu être collecté.", http.StatusServiceUnavailable)
		return
	}
	snapshots, err := a.dependencies.Configuration.Call(ctx, "snapshots", nil)
	if err != nil {
		http.Error(w, "Les snapshots n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	notice := ""
	if r.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">L’opération sur les snapshots est terminée.</p>`
	}
	available, total := inventoryCounts(inventory)
	sourceOptions, targetOptions, comparisonDisabled := renderSnapshotCompareOptions(snapshots)
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{NOTICE}}", notice, "{{SUMMARY}}", renderMetricPairs([][2]string{{"Sections disponibles", strconv.Itoa(available)}, {"Sections totales", strconv.Itoa(total)}, {"Snapshots", strconv.Itoa(intValue(snapshots["count"]))}}), "{{INVENTORY}}", renderInventory(inventory), "{{SNAPSHOTS}}", renderSnapshots(snapshots, session.CSRFToken), "{{SNAPSHOT_SOURCE_OPTIONS}}", sourceOptions, "{{SNAPSHOT_TARGET_OPTIONS}}", targetOptions, "{{COMPARE_DISABLED}}", comparisonDisabled).Replace(a.configurationPage)
	writeHTML(w, page, http.StatusOK)
}

func renderSnapshotCompareOptions(data map[string]any) (string, string, string) {
	items, _ := data["snapshots"].([]any)
	valid := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		item := mapValue(raw)
		isValid, _ := item["valid"].(bool)
		if isValid && stringValueAny(item["id"]) != "" {
			valid = append(valid, item)
		}
	}
	if len(valid) < 2 {
		message := `<option value="">Au moins deux snapshots sont nécessaires</option>`
		return message, message, ` disabled`
	}
	render := func(selected int) string {
		var b strings.Builder
		for index, item := range valid {
			id, name, created := stringValueAny(item["id"]), stringValueAny(item["name"]), stringValueAny(item["created_at"])
			if name == "" {
				name = id
			}
			b.WriteString(`<option value="` + html.EscapeString(id) + `"`)
			if index == selected {
				b.WriteString(` selected`)
			}
			b.WriteString(`>` + html.EscapeString(name))
			if created != "" {
				b.WriteString(` · ` + html.EscapeString(created))
			}
			b.WriteString(`</option>`)
		}
		return b.String()
	}
	return render(1), render(0), ""
}
func inventoryCounts(data map[string]any) (int, int) {
	sections, _ := data["sections"].([]any)
	available := 0
	for _, raw := range sections {
		item, _ := raw.(map[string]any)
		if value, _ := item["available"].(bool); value {
			available++
		}
	}
	return available, len(sections)
}
func renderInventory(data map[string]any) string {
	sections, _ := data["sections"].([]any)
	if len(sections) == 0 {
		return `<p class="muted">Aucune section disponible.</p>`
	}
	var b strings.Builder
	for _, raw := range sections {
		item, _ := raw.(map[string]any)
		label, _ := item["label"].(string)
		message, _ := item["message"].(string)
		available, _ := item["available"].(bool)
		b.WriteString(`<article class="content-card"><h3>` + html.EscapeString(label) + `</h3>`)
		if !available {
			b.WriteString(`<p class="notice">` + html.EscapeString(message) + `</p>`)
		} else {
			encoded, _ := json.MarshalIndent(item["data"], "", "  ")
			b.WriteString(`<pre>` + html.EscapeString(string(encoded)) + `</pre>`)
		}
		b.WriteString(`</article>`)
	}
	return b.String()
}
func renderSnapshots(data map[string]any, token string) string {
	items, _ := data["snapshots"].([]any)
	if len(items) == 0 {
		return `<p class="muted">Aucun snapshot.</p>`
	}
	var b strings.Builder
	b.WriteString(`<div class="table-scroll"><table class="data-table snapshot-table"><thead><tr><th>Actions</th><th>Nom</th><th>Date</th><th>Source</th><th>Sections</th><th>Taille</th><th>Intégrité</th></tr></thead><tbody>`)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		id, _ := item["id"].(string)
		name, _ := item["name"].(string)
		created, _ := item["created_at"].(string)
		source, _ := item["source"].(string)
		valid, _ := item["valid"].(bool)
		if name == "" {
			name = id
		}
		sections := strconv.Itoa(intValue(item["available_sections"])) + " / " + strconv.Itoa(intValue(item["total_sections"]))
		size := formatByteCount(int64(intValue(item["size"])))
		integrityClass, integrityLabel := "danger", "Invalide"
		if valid {
			integrityClass, integrityLabel = "success", "Valide"
		}
		b.WriteString(`<tr><td><div class="snapshot-table__actions"><a class="secondary-link compact-link" href="/configuration/snapshots/` + html.EscapeString(id) + `">Ouvrir</a><a class="secondary-link compact-link" href="/configuration/snapshots/` + html.EscapeString(id) + `/export">Exporter</a><form method="post" action="/configuration/snapshots/` + html.EscapeString(id) + `/delete"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input type="hidden" name="confirmation" value="` + html.EscapeString(id) + `"><button class="danger-button compact-link">Supprimer</button></form></div></td><td><form class="snapshot-table__rename" method="post" action="/configuration/snapshots/` + html.EscapeString(id) + `/name"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input name="name" value="` + html.EscapeString(name) + `" maxlength="100" aria-label="Nom du snapshot" required><button class="secondary-button compact-link">Renommer</button></form></td><td>` + html.EscapeString(created) + `</td><td>` + html.EscapeString(source) + `</td><td>` + sections + `</td><td>` + html.EscapeString(size) + `</td><td><span class="status-badge status-badge--` + integrityClass + `">` + integrityLabel + `</span></td></tr>`)
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}
func (a *application) configurationPost(w http.ResponseWriter, r *http.Request, command string, args func() []string) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	if !a.validForm(w, r, session.ID) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 60*time.Second)
	defer cancel()
	if _, err := a.dependencies.Configuration.Call(ctx, command, args()); err != nil {
		http.Error(w, "L’opération de configuration a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, "/configuration?result="+command, http.StatusSeeOther)
}
func (a *application) createConfigurationSnapshot(w http.ResponseWriter, r *http.Request) {
	a.configurationPost(w, r, "snapshot-create", func() []string { return []string{strings.TrimSpace(r.PostForm.Get("name"))} })
}
func (a *application) importConfigurationSnapshot(w http.ResponseWriter, r *http.Request) {
	session, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 6*1024*1024)
	if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, r.FormValue("_token")) {
		http.Error(w, "Import invalide.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	file, header, err := r.FormFile("snapshot")
	if err != nil || name == "" || len(name) > 100 || header.Size > 5*1024*1024 {
		http.Error(w, "Snapshot invalide.", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 5*1024*1024+1))
	if err != nil || len(content) > 5*1024*1024 {
		http.Error(w, "Snapshot illisible.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	if _, err = a.dependencies.Configuration.Call(ctx, "snapshot-import", []string{name, string(content)}); err != nil {
		http.Error(w, "Le snapshot importé est invalide ou existe déjà.", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/configuration?result=imported", http.StatusSeeOther)
}
func (a *application) compareConfigurationSnapshots(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	source, target := r.URL.Query().Get("source"), r.URL.Query().Get("target")
	if source == "" || target == "" || source == target {
		http.Error(w, "Comparaison invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	data, err := a.dependencies.Configuration.Call(ctx, "snapshot-compare", []string{source, target})
	if err != nil {
		http.Error(w, "Comparaison indisponible.", http.StatusNotFound)
		return
	}
	comparison := mapValue(data["comparison"])
	raw, _ := json.MarshalIndent(comparison, "", "  ")
	page := strings.NewReplacer(
		"{{SOURCE}}", html.EscapeString(stringValueAny(comparison["source_id"])),
		"{{TARGET}}", html.EscapeString(stringValueAny(comparison["target_id"])),
		"{{SUMMARY}}", renderComparisonSummary(mapValue(comparison["counts"])),
		"{{SECTIONS}}", renderComparisonSections(comparison["sections"]),
		"{{RAW}}", html.EscapeString(string(raw)),
	).Replace(a.configurationComparePage)
	writeHTML(w, page, http.StatusOK)
}
func (a *application) renameConfigurationSnapshot(w http.ResponseWriter, r *http.Request) {
	a.configurationPost(w, r, "snapshot-name", func() []string { return []string{r.PathValue("id"), strings.TrimSpace(r.PostForm.Get("name"))} })
}
func (a *application) deleteConfigurationSnapshot(w http.ResponseWriter, r *http.Request) {
	a.configurationPost(w, r, "snapshot-delete", func() []string { return []string{r.PathValue("id"), r.PostForm.Get("confirmation")} })
}
func (a *application) configurationSnapshot(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	data, err := a.dependencies.Configuration.Call(ctx, "snapshot", []string{r.PathValue("id")})
	if err != nil {
		http.Error(w, "Snapshot introuvable.", http.StatusNotFound)
		return
	}
	snapshot := mapValue(data["snapshot"])
	inventory := mapValue(snapshot["inventory"])
	raw, _ := json.MarshalIndent(snapshot, "", "  ")
	page := strings.NewReplacer(
		"{{ID}}", html.EscapeString(stringValueAny(snapshot["id"])),
		"{{CREATED}}", html.EscapeString(stringValueAny(snapshot["created_at"])),
		"{{SCHEMA}}", html.EscapeString(stringValueAny(snapshot["inventory_schema"])),
		"{{FINGERPRINT}}", html.EscapeString(stringValueAny(snapshot["inventory_sha256"])),
		"{{SECTIONS}}", renderSnapshotSections(inventory["sections"]),
		"{{RAW}}", html.EscapeString(string(raw)),
	).Replace(a.configurationSnapshotPage)
	writeHTML(w, page, http.StatusOK)
}

func renderSnapshotSections(value any) string {
	sections := sliceValue(value)
	if len(sections) == 0 {
		return `<p class="muted">Aucune section disponible.</p>`
	}
	var navigation, content strings.Builder
	navigation.WriteString(`<nav class="snapshot-navigation" aria-label="Sections du snapshot">`)
	for index, raw := range sections {
		section := mapValue(raw)
		label := stringValueAny(section["label"])
		if label == "" {
			label = stringValueAny(section["id"])
		}
		anchor := "snapshot-section-" + strconv.Itoa(index)
		navigation.WriteString(`<a href="#` + anchor + `">` + html.EscapeString(label) + `</a>`)
		content.WriteString(`<section class="content-card snapshot-section" id="` + anchor + `"><h2>` + html.EscapeString(label) + `</h2>`)
		if available, _ := section["available"].(bool); !available {
			content.WriteString(`<p class="notice">` + html.EscapeString(stringValueAny(section["message"])) + `</p>`)
		} else {
			content.WriteString(renderStructuredValue(section["data"]))
		}
		content.WriteString(`</section>`)
	}
	navigation.WriteString(`</nav>`)
	return navigation.String() + content.String()
}

func renderComparisonSummary(counts map[string]any) string {
	return renderMetricPairs([][2]string{{"Identiques", strconv.Itoa(intValue(counts["identical"]))}, {"Différentes", strconv.Itoa(intValue(counts["different"]))}, {"Absentes de la source", strconv.Itoa(intValue(counts["missing_source"]))}, {"Absentes de la cible", strconv.Itoa(intValue(counts["missing_target"]))}})
}

func renderComparisonSections(value any) string {
	sections := sliceValue(value)
	if len(sections) == 0 {
		return `<p class="muted">Aucune section à comparer.</p>`
	}
	labels := map[string]string{"identical": "Identique", "different": "Modifiée", "missing_source": "Ajoutée", "missing_target": "Supprimée"}
	classes := map[string]string{"identical": "success", "different": "warning", "missing_source": "success", "missing_target": "danger"}
	var b strings.Builder
	differences := 0
	for _, raw := range sections {
		section := mapValue(raw)
		status := stringValueAny(section["status"])
		if status == "identical" {
			continue
		}
		differences++
		label := stringValueAny(section["label"])
		b.WriteString(`<section class="content-card snapshot-section"><header class="section-heading section-heading--flush"><h2>` + html.EscapeString(label) + `</h2><span class="status-badge status-badge--` + classes[status] + `">` + labels[status] + `</span></header>`)
		b.WriteString(renderStructuredDifferences(section["source"], section["target"]))
		b.WriteString(`</section>`)
	}
	if differences == 0 {
		return `<p class="notice notice--success">Aucune différence entre ces deux snapshots.</p>`
	}
	return b.String()
}

func renderStructuredDifferences(source, target any) string {
	var rows strings.Builder
	renderDifferenceRows(&rows, nil, normalizedJSONValue(source), normalizedJSONValue(target))
	if rows.Len() == 0 {
		return `<p class="muted">Aucune valeur différente dans cette section.</p>`
	}
	return `<div class="table-scroll"><table class="data-table snapshot-difference-table"><thead><tr><th>Élément</th><th>Snapshot source</th><th>Snapshot cible</th></tr></thead><tbody>` + rows.String() + `</tbody></table></div>`
}

func renderDifferenceRows(rows *strings.Builder, path []string, source, target any) {
	if structuredValuesEqual(source, target) {
		return
	}
	sourceMap, sourceIsMap := source.(map[string]any)
	targetMap, targetIsMap := target.(map[string]any)
	if sourceIsMap || targetIsMap {
		keys, seen := make([]string, 0, len(sourceMap)+len(targetMap)), map[string]bool{}
		for key := range sourceMap {
			seen[key] = true
			keys = append(keys, key)
		}
		for key := range targetMap {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			writeDifferenceRow(rows, path, source, target)
		}
		for _, key := range keys {
			renderDifferenceRows(rows, appendPath(path, configurationLabel(key)), sourceMap[key], targetMap[key])
		}
		return
	}
	sourceItems, sourceIsSlice := source.([]any)
	targetItems, targetIsSlice := target.([]any)
	if sourceIsSlice || targetIsSlice {
		if sourceByID, targetByID, keys, ok := indexedApacheVirtualHosts(sourceItems, targetItems); ok {
			for _, key := range keys {
				renderDifferenceRows(rows, appendPath(path, apacheVirtualHostIdentityLabel(sourceByID[key], targetByID[key])), sourceByID[key], targetByID[key])
			}
			return
		}
		if sourceByID, targetByID, keys, ok := indexedListenerDifferenceItems(sourceItems, targetItems); ok {
			for _, key := range keys {
				renderDifferenceRows(rows, appendPath(path, listenerDifferenceIdentityLabel(sourceByID[key], targetByID[key])), sourceByID[key], targetByID[key])
			}
			return
		}
		if sourceByID, targetByID, keys, ok := indexedFirewallDifferenceItems(sourceItems, targetItems); ok {
			for _, key := range keys {
				renderDifferenceRows(rows, appendPath(path, firewallDifferenceIdentityLabel(sourceByID[key], targetByID[key])), sourceByID[key], targetByID[key])
			}
			return
		}
		if sourceByID, targetByID, keys, ok := indexedCronDifferenceItems(sourceItems, targetItems); ok {
			for _, key := range keys {
				renderDifferenceRows(rows, appendPath(path, cronDifferenceIdentityLabel(key, sourceByID[key], targetByID[key])), sourceByID[key], targetByID[key])
			}
			return
		}
		if identity, sourceByID, targetByID, keys, ok := indexedDifferenceItems(sourceItems, targetItems); ok {
			for _, key := range keys {
				renderDifferenceRows(rows, appendPath(path, differenceIdentityLabel(identity, key)), sourceByID[key], targetByID[key])
			}
			return
		}
		if sourceSet, targetSet, keys, ok := scalarDifferenceSets(sourceItems, targetItems); ok {
			for _, key := range keys {
				left, leftExists := sourceSet[key]
				right, rightExists := targetSet[key]
				if leftExists && rightExists {
					continue
				}
				labelValue := left
				if !leftExists {
					labelValue = right
				}
				renderDifferenceRows(rows, appendPath(path, "Valeur "+stringValueAny(labelValue)), left, right)
			}
			return
		}
		length := len(sourceItems)
		if len(targetItems) > length {
			length = len(targetItems)
		}
		if length == 0 {
			writeDifferenceRow(rows, path, source, target)
		}
		for index := 0; index < length; index++ {
			var left, right any
			if index < len(sourceItems) {
				left = sourceItems[index]
			}
			if index < len(targetItems) {
				right = targetItems[index]
			}
			renderDifferenceRows(rows, appendPath(path, "Élément "+strconv.Itoa(index+1)), left, right)
		}
		return
	}
	writeDifferenceRow(rows, path, source, target)
}

func indexedApacheVirtualHosts(source, target []any) (map[string]any, map[string]any, []string, bool) {
	index := func(items []any) (map[string]any, bool) {
		result := make(map[string]any, len(items))
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, false
			}
			configFile, serverName, port := stringValueAny(item["config_file"]), stringValueAny(item["server_name"]), stringValueAny(item["port"])
			if configFile == "" {
				return nil, false
			}
			if serverName == "" {
				serverName = "(sans ServerName)"
			}
			if port == "" {
				port = "(port inconnu)"
			}
			key := configFile + "\x00" + strings.ToLower(serverName) + "\x00" + port
			if _, duplicate := result[key]; duplicate {
				line := stringValueAny(item["config_line"])
				if line == "" {
					return nil, false
				}
				key += "\x00ligne:" + line
				if _, duplicate = result[key]; duplicate {
					return nil, false
				}
			}
			comparable := make(map[string]any, len(item))
			for field, value := range item {
				comparable[field] = value
			}
			delete(comparable, "config_line")
			result[key] = comparable
		}
		return result, true
	}
	left, leftOK := index(source)
	right, rightOK := index(target)
	if !leftOK || !rightOK || len(left)+len(right) == 0 {
		return nil, nil, nil, false
	}
	keys, seen := make([]string, 0, len(left)+len(right)), map[string]bool{}
	for key := range left {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range right {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return left, right, keys, true
}

func apacheVirtualHostIdentityLabel(source, target any) string {
	item := mapValue(source)
	if item == nil {
		item = mapValue(target)
	}
	serverName, port := stringValueAny(item["server_name"]), stringValueAny(item["port"])
	if serverName == "" {
		serverName = "sans ServerName"
	}
	if port == "" {
		port = "inconnu"
	}
	return "VirtualHost " + filepath.Base(stringValueAny(item["config_file"])) + " · " + serverName + " · port " + port
}

func indexedListenerDifferenceItems(source, target []any) (map[string]any, map[string]any, []string, bool) {
	index := func(items []any) (map[string]any, bool) {
		result := make(map[string]any, len(items))
		for _, raw := range items {
			item, ok := normalizedListener(raw)
			if !ok {
				return nil, false
			}
			key := stringValueAny(item["protocol"]) + "\x00" + stringValueAny(item["address"]) + "\x00" + stringValueAny(item["port"])
			if _, duplicate := result[key]; duplicate {
				return nil, false
			}
			result[key] = item
		}
		return result, true
	}
	left, leftOK := index(source)
	right, rightOK := index(target)
	if !leftOK || !rightOK || len(left)+len(right) == 0 {
		return nil, nil, nil, false
	}
	keys, seen := make([]string, 0, len(left)+len(right)), map[string]bool{}
	for key := range left {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range right {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return left, right, keys, true
}

func normalizedListener(raw any) (map[string]any, bool) {
	if item, ok := raw.(map[string]any); ok {
		for _, field := range []string{"protocol", "state", "address", "port"} {
			if stringValueAny(item[field]) == "" {
				return nil, false
			}
		}
		return map[string]any{"protocol": strings.ToLower(stringValueAny(item["protocol"])), "state": strings.ToUpper(stringValueAny(item["state"])), "address": stringValueAny(item["address"]), "port": stringValueAny(item["port"])}, true
	}
	line, ok := raw.(string)
	if !ok {
		return nil, false
	}
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return nil, false
	}
	endpoint := fields[4]
	index := strings.LastIndex(endpoint, ":")
	if index < 0 {
		return nil, false
	}
	address, port := endpoint[:index], endpoint[index+1:]
	address = strings.TrimPrefix(strings.TrimSuffix(address, "]"), "[")
	if address == "" || port == "" {
		return nil, false
	}
	return map[string]any{"protocol": strings.ToLower(fields[0]), "state": strings.ToUpper(fields[1]), "address": address, "port": port}, true
}

func listenerDifferenceIdentityLabel(source, target any) string {
	item := mapValue(source)
	if item == nil {
		item = mapValue(target)
	}
	address := stringValueAny(item["address"])
	if strings.Contains(address, ":") {
		address = "[" + address + "]"
	}
	return "Écoute " + stringValueAny(item["protocol"]) + " " + address + ":" + stringValueAny(item["port"])
}

func indexedFirewallDifferenceItems(source, target []any) (map[string]any, map[string]any, []string, bool) {
	fields := []string{"action", "direction", "protocol", "ports", "source", "destination", "family"}
	index := func(items []any) (map[string]any, bool) {
		result := make(map[string]any, len(items))
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, false
			}
			identity := make(map[string]any, len(fields))
			for _, field := range fields {
				value, exists := item[field]
				if !exists {
					return nil, false
				}
				if field == "ports" {
					ports := sliceValue(value)
					normalized := make([]string, 0, len(ports))
					for _, port := range ports {
						normalized = append(normalized, stringValueAny(port))
					}
					sort.Strings(normalized)
					value = normalized
				}
				identity[field] = value
			}
			encoded, err := json.Marshal(identity)
			if err != nil {
				return nil, false
			}
			key := string(encoded)
			if _, duplicate := result[key]; duplicate {
				return nil, false
			}
			comparable := make(map[string]any, len(item))
			for field, value := range item {
				comparable[field] = value
			}
			delete(comparable, "id")
			result[key] = comparable
		}
		return result, true
	}
	left, leftOK := index(source)
	right, rightOK := index(target)
	if !leftOK || !rightOK || len(left)+len(right) == 0 {
		return nil, nil, nil, false
	}
	keys, seen := make([]string, 0, len(left)+len(right)), map[string]bool{}
	for key := range left {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range right {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return left, right, keys, true
}

func firewallDifferenceIdentityLabel(source, target any) string {
	item := mapValue(source)
	if item == nil {
		item = mapValue(target)
	}
	ports := sliceValue(item["ports"])
	portLabels := make([]string, 0, len(ports))
	for _, port := range ports {
		portLabels = append(portLabels, stringValueAny(port))
	}
	port := strings.Join(portLabels, ",")
	if port == "" {
		port = "tous ports"
	}
	return "Règle " + stringValueAny(item["action"]) + " " + stringValueAny(item["direction"]) + " · " + stringValueAny(item["protocol"]) + " " + port + " · " + stringValueAny(item["source"])
}

func indexedCronDifferenceItems(source, target []any) (map[string]any, map[string]any, []string, bool) {
	index := func(items []any) (map[string]any, bool) {
		result := make(map[string]any, len(items))
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, false
			}
			user, sourceName, command := stringValueAny(item["user"]), stringValueAny(item["source"]), stringValueAny(item["command"])
			if user == "" || sourceName == "" || command == "" || stringValueAny(item["schedule"]) == "" {
				return nil, false
			}
			key := "task:" + user + "\x00" + sourceName + "\x00" + command
			if _, duplicate := result[key]; duplicate {
				return nil, false
			}
			comparable := make(map[string]any, len(item))
			for field, value := range item {
				comparable[field] = value
			}
			delete(comparable, "line")
			delete(comparable, "id")
			result[key] = comparable
		}
		return result, true
	}
	left, leftOK := index(source)
	right, rightOK := index(target)
	if !leftOK || !rightOK || len(left)+len(right) == 0 {
		return nil, nil, nil, false
	}
	keys, seen := make([]string, 0, len(left)+len(right)), map[string]bool{}
	for key := range left {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range right {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return left, right, keys, true
}

func cronDifferenceIdentityLabel(key string, source, target any) string {
	item := mapValue(source)
	if item == nil {
		item = mapValue(target)
	}
	user, command := stringValueAny(item["user"]), stringValueAny(item["command"])
	if len([]rune(command)) > 72 {
		command = string([]rune(command)[:69]) + "…"
	}
	if user != "" && command != "" {
		return "Tâche " + user + " · " + command
	}
	return "Tâche " + key
}

func indexedDifferenceItems(source, target []any) (string, map[string]any, map[string]any, []string, bool) {
	candidates := []string{"name", "filename", "config_file", "path", "server_name", "id", "login", "unit", "service", "domain", "package", "source", "mount", "address", "device", "port", "interface"}
	for _, candidate := range candidates {
		left, leftOK := indexDifferenceItems(source, candidate)
		right, rightOK := indexDifferenceItems(target, candidate)
		if !leftOK || !rightOK || len(left)+len(right) == 0 {
			continue
		}
		keys, seen := make([]string, 0, len(left)+len(right)), map[string]bool{}
		for key := range left {
			seen[key] = true
			keys = append(keys, key)
		}
		for key := range right {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		return candidate, left, right, keys, true
	}
	return "", nil, nil, nil, false
}

func differenceIdentityLabel(identity, key string) string {
	if identity == "config_file" || identity == "filename" || identity == "path" {
		return "Fichier " + filepath.Base(key)
	}
	return configurationLabel(identity) + " " + key
}

func indexDifferenceItems(items []any, identity string) (map[string]any, bool) {
	indexed := make(map[string]any, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		value, exists := item[identity]
		if !exists {
			return nil, false
		}
		key := stringValueAny(value)
		if key == "" {
			return nil, false
		}
		if _, duplicate := indexed[key]; duplicate {
			return nil, false
		}
		indexed[key] = item
	}
	return indexed, true
}

func scalarDifferenceSets(source, target []any) (map[string]any, map[string]any, []string, bool) {
	index := func(items []any) (map[string]any, bool) {
		result := make(map[string]any, len(items))
		for _, item := range items {
			switch item.(type) {
			case map[string]any, []any:
				return nil, false
			}
			encoded, err := json.Marshal(item)
			if err != nil {
				return nil, false
			}
			result[string(encoded)] = item
		}
		return result, true
	}
	left, leftOK := index(source)
	right, rightOK := index(target)
	if !leftOK || !rightOK {
		return nil, nil, nil, false
	}
	keys, seen := make([]string, 0, len(left)+len(right)), map[string]bool{}
	for key := range left {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range right {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return left, right, keys, true
}

func writeDifferenceRow(rows *strings.Builder, path []string, source, target any) {
	label := strings.Join(path, " › ")
	if label == "" {
		label = "Valeur"
	}
	rows.WriteString(`<tr><th>` + html.EscapeString(label) + `</th><td>` + renderDifferenceValue(source) + `</td><td>` + renderDifferenceValue(target) + `</td></tr>`)
}

func renderDifferenceValue(value any) string {
	if value == nil {
		return `<span class="muted">Absent</span>`
	}
	if boolean, ok := value.(bool); ok {
		return yesNo(boolean)
	}
	if object, ok := value.(map[string]any); ok && len(object) == 0 {
		return `<span class="muted">Objet vide</span>`
	}
	if items, ok := value.([]any); ok && len(items) == 0 {
		return `<span class="muted">Liste vide</span>`
	}
	return `<span class="snapshot-scalar">` + html.EscapeString(stringValueAny(value)) + `</span>`
}

func normalizedJSONValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if decoder.Decode(&normalized) != nil {
		return value
	}
	return normalized
}
func structuredValuesEqual(source, target any) bool {
	left, leftErr := json.Marshal(source)
	right, rightErr := json.Marshal(target)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}
func appendPath(path []string, value string) []string {
	result := make([]string, len(path), len(path)+1)
	copy(result, path)
	return append(result, value)
}

func renderStructuredValue(value any) string {
	if value == nil {
		return `<span class="muted">Absent</span>`
	}
	if object := mapValue(value); object != nil {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(`<div class="table-scroll"><table class="data-table data-table--nested"><tbody>`)
		for _, key := range keys {
			b.WriteString(`<tr><th>` + html.EscapeString(configurationLabel(key)) + `</th><td>` + renderStructuredValue(object[key]) + `</td></tr>`)
		}
		b.WriteString(`</tbody></table></div>`)
		return b.String()
	}
	if items := sliceValue(value); items != nil {
		if len(items) == 0 {
			return `<span class="muted">Aucun élément</span>`
		}
		var b strings.Builder
		b.WriteString(`<ol class="snapshot-value-list">`)
		for _, item := range items {
			b.WriteString(`<li>` + renderStructuredValue(item) + `</li>`)
		}
		b.WriteString(`</ol>`)
		return b.String()
	}
	if boolean, ok := value.(bool); ok {
		return yesNo(boolean)
	}
	return `<span class="snapshot-scalar">` + html.EscapeString(stringValueAny(value)) + `</span>`
}

func mapValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result map[string]any
	if json.Unmarshal(encoded, &result) != nil {
		return nil
	}
	return result
}
func sliceValue(value any) []any {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]any); ok {
		return typed
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result []any
	if json.Unmarshal(encoded, &result) != nil {
		return nil
	}
	return result
}
func stringValueAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
func configurationLabel(key string) string {
	labels := map[string]string{"id": "Identifiant", "label": "Libellé", "available": "Disponible", "message": "Message", "data": "Données", "created_at": "Créé le", "collected_at": "Collecté le", "read_only": "Lecture seule", "status": "État", "enabled": "Activé", "active": "Actif", "exists": "Présent", "version": "Version", "name": "Nom", "type": "Type", "path": "Chemin", "size": "Taille", "source": "Source", "target": "Cible", "config_file": "Fichier de configuration", "server_name": "Nom du serveur", "document_root": "Racine du site", "sha256": "Empreinte SHA-256", "config_line": "Ligne de configuration"}
	if label := labels[key]; label != "" {
		return label
	}
	label := strings.ReplaceAll(key, "_", " ")
	if label == "" {
		return "Valeur"
	}
	return strings.ToUpper(label[:1]) + label[1:]
}
func (a *application) exportConfigurationSnapshot(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.rootUser(w, r)
	if !ok {
		return
	}
	ctx, cancel := contextWithTimeout(r, 20*time.Second)
	defer cancel()
	data, err := a.dependencies.Configuration.Call(ctx, "snapshot-export", []string{r.PathValue("id")})
	if err != nil {
		http.Error(w, "Export indisponible.", http.StatusNotFound)
		return
	}
	content, _ := data["content"].(string)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="aegisadmin-configuration-`+r.PathValue("id")+`.json"`)
	_, _ = w.Write([]byte(content))
}
func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}
