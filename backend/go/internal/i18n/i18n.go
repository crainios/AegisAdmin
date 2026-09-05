package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const DefaultLanguage = "fr"

//go:embed locales/*.json
var localeFiles embed.FS

var catalogs = loadCatalogs()

func Supported(language string) bool {
	_, found := catalogs[language]
	return found
}

func FromAcceptLanguage(value string) string {
	for _, item := range strings.Split(value, ",") {
		language := strings.ToLower(strings.TrimSpace(strings.SplitN(item, ";", 2)[0]))
		if separator := strings.IndexByte(language, '-'); separator >= 0 {
			language = language[:separator]
		}
		if Supported(language) {
			return language
		}
	}
	return DefaultLanguage
}

func Text(language, key string) string {
	if translated, found := catalogs[language][key]; found {
		return translated
	}
	if translated, found := catalogs[DefaultLanguage][key]; found {
		return translated
	}
	return key
}

func FormatDateTime(language string, value time.Time) string {
	if language == "en" {
		return value.Format("01/02/2006 03:04 PM")
	}
	return value.Format("02/01/2006 15:04")
}

func FormatRFC3339(language, value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return FormatDateTime(language, parsed)
}

func Localize(template, language string) string {
	if !Supported(language) {
		language = DefaultLanguage
	}
	for key, translated := range catalogs[DefaultLanguage] {
		value := translated
		if candidate, found := catalogs[language][key]; found {
			value = candidate
		}
		template = strings.ReplaceAll(template, "{{T:"+key+"}}", value)
	}
	return strings.ReplaceAll(template, "{{LANG}}", language)
}

func loadCatalogs() map[string]map[string]string {
	result := make(map[string]map[string]string)
	for _, language := range []string{"fr", "en"} {
		content, err := localeFiles.ReadFile("locales/" + language + ".json")
		if err != nil {
			panic(fmt.Sprintf("read %s translations: %v", language, err))
		}
		catalog := make(map[string]string)
		if err := json.Unmarshal(content, &catalog); err != nil {
			panic(fmt.Sprintf("decode %s translations: %v", language, err))
		}
		result[language] = catalog
	}
	for key := range result[DefaultLanguage] {
		for _, language := range []string{"en"} {
			if _, found := result[language][key]; !found {
				panic(fmt.Sprintf("translation %q is missing from %s", key, language))
			}
		}
	}
	return result
}
