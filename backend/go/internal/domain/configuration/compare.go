package configuration

import (
	"encoding/json"
	"sort"
)

func compareSnapshots(source, target Snapshot) map[string]any {
	sourceSections := indexedSections(source.Inventory)
	targetSections := indexedSections(target.Inventory)
	ids := make([]string, 0, len(sourceSections)+len(targetSections))
	seen := map[string]bool{}
	for id := range sourceSections {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range targetSections {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	items := make([]map[string]any, 0, len(ids))
	counts := map[string]int{"identical": 0, "different": 0, "missing_source": 0, "missing_target": 0}
	for _, id := range ids {
		left, leftOK := sourceSections[id]
		right, rightOK := targetSections[id]
		status := "different"
		switch {
		case !leftOK:
			status = "missing_source"
		case !rightOK:
			status = "missing_target"
		case equalJSON(left, right):
			status = "identical"
		}
		counts[status]++
		label := id
		if leftOK && stringValue(left["label"]) != "" {
			label = stringValue(left["label"])
		} else if rightOK && stringValue(right["label"]) != "" {
			label = stringValue(right["label"])
		}
		item := map[string]any{"id": id, "label": label, "status": status}
		if status != "identical" {
			item["source"] = left
			item["target"] = right
		}
		items = append(items, item)
	}
	return map[string]any{"source_id": source.ID, "target_id": target.ID, "counts": counts, "sections": items}
}

func indexedSections(inventory map[string]any) map[string]map[string]any {
	result := map[string]map[string]any{}
	switch sections := inventory["sections"].(type) {
	case []any:
		for _, raw := range sections {
			if section, ok := raw.(map[string]any); ok {
				if id := stringValue(section["id"]); id != "" {
					result[id] = section
				}
			}
		}
	case []map[string]any:
		for _, section := range sections {
			if id := stringValue(section["id"]); id != "" {
				result[id] = section
			}
		}
	}
	return result
}

func equalJSON(left, right any) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && string(a) == string(b)
}
