package i18n

import (
	"testing"
	"time"
)

func TestCatalogsAndLanguageSelection(t *testing.T) {
	if got := Text("en", "dashboard.title"); got != "Dashboard" {
		t.Fatalf("English dashboard title=%q", got)
	}
	if got := Text("unknown", "dashboard.title"); got != "Tableau de bord" {
		t.Fatalf("fallback dashboard title=%q", got)
	}
	if got := FromAcceptLanguage("de-DE, en-GB;q=0.9, fr;q=0.8"); got != "en" {
		t.Fatalf("selected language=%q", got)
	}
	localized := Localize(`<html lang="{{LANG}}"><title>{{T:login.title}}</title></html>`, "en")
	if localized != `<html lang="en"><title>Sign in</title></html>` {
		t.Fatalf("localized template=%q", localized)
	}
}

func TestDateTimeFollowsSelectedLocale(t *testing.T) {
	value := time.Date(2026, time.September, 4, 18, 35, 0, 0, time.UTC)
	if got := FormatDateTime("fr", value); got != "04/09/2026 18:35" {
		t.Fatalf("French date=%q", got)
	}
	if got := FormatDateTime("en", value); got != "09/04/2026 06:35 PM" {
		t.Fatalf("US date=%q", got)
	}
}
