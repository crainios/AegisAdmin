package webapp

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"aegisadmin/backend/internal/authstore"
	"aegisadmin/backend/internal/buildinfo"
	"aegisadmin/backend/internal/webapache"
	"aegisadmin/backend/internal/webauth"
	"aegisadmin/backend/internal/webcertbot"
	"aegisadmin/backend/internal/webcron"
	"aegisadmin/backend/internal/webdashboard"
	"aegisadmin/backend/internal/webfail2ban"
	"aegisadmin/backend/internal/webfirewall"
	"aegisadmin/backend/internal/weblogs"
	"aegisadmin/backend/internal/webmysql"
	"aegisadmin/backend/internal/webnetwork"
	"aegisadmin/backend/internal/webphp"
	"aegisadmin/backend/internal/webservices"
	"aegisadmin/backend/internal/websession"
	"aegisadmin/backend/internal/webstorage"
	"aegisadmin/backend/internal/webtor"
	"aegisadmin/backend/internal/webupdates"
	"golang.org/x/crypto/bcrypt"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	Handler(testDependencies(t, nil)).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if body.Status != "ok" || body.Version == "" {
		t.Fatalf("unexpected health response: %#v", body)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers are missing")
	}
}

func TestApplicationJavaScriptOffersNeonTheme(t *testing.T) {
	asset, err := embeddedAssets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(asset)
	if !strings.Contains(content, `"neon"`) || !strings.Contains(content, `<option value="neon">Néon Pop</option>`) || !strings.Contains(content, `group.dataset.accent`) {
		t.Fatal("the Neon Pop theme or its navigation accents are missing")
	}
	if !strings.Contains(content, `window.location.replace`) || !strings.Contains(content, `refreshed=${Date.now()}`) {
		t.Fatal("the application update completion does not reload the updates page")
	}
}

func TestCertbotResultUsesAnAccessiblePollingModal(t *testing.T) {
	template, err := embeddedAssets.ReadFile("assets/certbot.html")
	if err != nil {
		t.Fatal(err)
	}
	script, err := embeddedAssets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(template), `data-certbot-result-modal`) || !strings.Contains(string(template), `aria-modal="true"`) || !strings.Contains(string(template), `data-certbot-result-output`) {
		t.Fatal("the Certbot result modal is incomplete")
	}
	if !strings.Contains(string(script), `/certbot/actions/${encodeURIComponent(id)}`) || !strings.Contains(string(script), `result.status === "running"`) || !strings.Contains(string(script), `window.location.replace("/certbot")`) {
		t.Fatal("the Certbot result polling or refresh behavior is missing")
	}
}

func TestProcessDescriptionsAreLimitedToKnownProcesses(t *testing.T) {
	rows := renderProcesses([]webdashboard.Process{{Name: "sshd", State: "R", StateLabel: "En cours", Description: "Serveur d’accès distant sécurisé SSH."}, {Name: "private-worker", State: "S", StateLabel: "En veille"}}, "fr")
	if !strings.Contains(rows, `title="Serveur d’accès distant sécurisé SSH."`) || !strings.Contains(rows, `aria-label="sshd : Serveur d’accès distant sécurisé SSH."`) {
		t.Fatalf("known process has no accessible description: %q", rows)
	}
	if strings.Count(rows, `process-name--described`) != 1 {
		t.Fatalf("unknown process received an approximate description: %q", rows)
	}
	if !strings.Contains(rows, "En cours · R") || !strings.Contains(rows, "En veille · S") {
		t.Fatalf("French process states are missing: %q", rows)
	}
	englishRows := renderProcesses([]webdashboard.Process{{Name: "sshd", State: "R", StateLabel: "En cours"}, {Name: "worker", State: "D", StateLabel: "Attente disque"}}, "en")
	if !strings.Contains(englishRows, "Running · R") || !strings.Contains(englishRows, "Disk wait · D") || strings.Contains(englishRows, "En cours") {
		t.Fatalf("English process states are not translated: %q", englishRows)
	}
}

func TestRootCanReadUserAccessLog(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/users/access-log?event=login_success", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Connexion réussie") || !strings.Contains(response.Body.String(), "25/08/2026 10:00") || !strings.Contains(response.Body.String(), "192.0.2.1") || !strings.Contains(response.Body.String(), `<select name="login">`) || !strings.Contains(response.Body.String(), `operator`) {
		t.Fatalf("access log=%d %q", response.Code, response.Body.String())
	}
}

func TestStorageUsesProfileLanguage(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}, language: "en"}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/storage", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<html lang="en">`) ||
		!strings.Contains(response.Body.String(), "Physical disk") || !strings.Contains(response.Body.String(), "Healthy") ||
		!strings.Contains(response.Body.String(), "Permission Modify") {
		t.Fatalf("English storage=%d %q", response.Code, response.Body.String())
	}
}

func TestAboutUsesProfileLanguage(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}, language: "en"}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/about", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `<html lang="en">`) ||
		!strings.Contains(body, "About AegisAdmin") || !strings.Contains(body, "Open-source project") ||
		!strings.Contains(body, ">Sign out<") || strings.Contains(body, "À propos d’AegisAdmin") {
		t.Fatalf("English about=%d %q", response.Code, body)
	}
}

func TestPermissionFieldsUseCategorizedRadioGroups(t *testing.T) {
	fields := renderPermissionFields([]authstore.AssignableModule{{ID: 3, Name: "Cron", Category: "Services"}}, map[int64]string{3: "action"}, "permission_")
	if !strings.Contains(fields, `user-module-permissions__category">Services`) || !strings.Contains(fields, `type="radio" name="permission_3" value="action" checked`) || strings.Contains(fields, `<select`) {
		t.Fatalf("fields=%q", fields)
	}
}

func TestUserIdentityConflictUsesFriendlyMessages(t *testing.T) {
	items := []authstore.AdminUser{{Login: "operator", Email: "operator@example.test"}}
	if message := userIdentityConflict(items, authstore.AdminUser{Login: "another", Email: "OPERATOR@example.test"}); message != "Cette adresse e-mail est déjà utilisée par un autre utilisateur." {
		t.Fatalf("email conflict=%q", message)
	}
	if message := userIdentityConflict(items, authstore.AdminUser{Login: "Operator", Email: "another@example.test"}); message != "Cet identifiant est déjà utilisé par un autre utilisateur." {
		t.Fatalf("login conflict=%q", message)
	}
	if message := userIdentityConflict(items, authstore.AdminUser{Login: "another", Email: "another@example.test"}); message != "" {
		t.Fatalf("unexpected conflict=%q", message)
	}
}

func TestRenderUserRowsPlacesActionsFirstAndMarksCurrentAccount(t *testing.T) {
	rows := renderUserRows([]authstore.AdminUser{{ID: 7, Login: "root", Status: "active"}}, 7)
	action := strings.Index(rows, `href="/users/7/edit"`)
	login := strings.Index(rows, `<th scope="row">root</th>`)
	if action < 0 || login < 0 || action > login {
		t.Fatalf("actions are not before identity: %q", rows)
	}
	if !strings.Contains(rows, `class="current-account-indicator"`) ||
		!strings.Contains(rows, `title="Compte actuellement connecté"`) ||
		strings.Contains(rows, `>Compte actuel<`) {
		t.Fatalf("current account indicator is invalid: %q", rows)
	}
}

func TestIndexAndNotFound(t *testing.T) {
	response := httptest.NewRecorder()
	handler := Handler(testDependencies(t, nil))
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("unexpected index response: %d %q", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/login", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Connexion") ||
		!strings.Contains(response.Body.String(), `href="/assets/app.css?v=`) {
		t.Fatalf("unexpected login response: %d %q", response.Code, response.Body.String())
	}
	setCookie := response.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, websession.CookieName+"=") || !strings.Contains(setCookie, "Secure") || !strings.Contains(setCookie, "HttpOnly") || !strings.Contains(setCookie, "SameSite=Strict") {
		t.Fatalf("unsafe session cookie: %q", setCookie)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status for unknown route: %d", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/login.html", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("internal template is public: %d", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/app.css?v=test", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), ".dashboard-shell") ||
		!strings.Contains(response.Body.String(), ".compact-select") ||
		!strings.Contains(response.Body.String(), ".network-selected-button") ||
		!strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("unexpected versioned stylesheet: %d %q", response.Code, response.Header().Get("Cache-Control"))
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/app.js?v=test", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "app-shell") ||
		!strings.Contains(response.Body.String(), `const csrfToken = logout?.querySelector('input[name="_token"]')?.value || "";`) ||
		!strings.Contains(response.Body.String(), `new Intl.NumberFormat(activeLanguage === "en" ? "en-US" : "fr-FR")`) ||
		strings.Contains(response.Body.String(), `header.querySelector('form[action="/logout"] input[name="_token"]')`) ||
		!strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("unexpected versioned script: %d %q", response.Code, response.Header().Get("Cache-Control"))
	}
}

func TestAuthenticatedUserCanSaveTheme(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"_token": {session.CSRFToken}, "theme": {"neon"}}
	request := httptest.NewRequest(http.MethodPost, "/go/account/theme", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || users.theme != "neon" {
		t.Fatalf("theme response=%d stored=%q", response.Code, users.theme)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "aegisadmin_theme" || cookies[0].Value != "neon" || !cookies[0].Secure || cookies[0].MaxAge != 31536000 {
		t.Fatalf("theme cookie=%#v", cookies)
	}
}

func TestAuthenticatedUserCanSaveLanguage(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}, language: "fr"}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"_token": {session.CSRFToken}, "language": {"en"}}
	request := httptest.NewRequest(http.MethodPost, "/go/account/language", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/go/account/password?result=language" || users.language != "en" {
		t.Fatalf("language response=%d stored=%q", response.Code, users.language)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "aegisadmin_language" || cookies[0].Value != "en" || !cookies[0].Secure {
		t.Fatalf("language cookie=%#v", cookies)
	}
}

func TestLoginUsesConfiguredDefaultLanguage(t *testing.T) {
	dependencies := testDependencies(t, nil)
	dependencies.DefaultLanguage = "en"
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<html lang="en">`) || !strings.Contains(response.Body.String(), "Access your server administration console.") {
		t.Fatalf("English login=%d %q", response.Code, response.Body.String())
	}
}

func TestAccountAndTwoFactorUseProfileLanguage(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}, language: "en"}
	dependencies := testDependencies(t, users)
	authenticated, err := dependencies.Sessions.Create(websession.StateAuthenticated, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	accountRequest := httptest.NewRequest(http.MethodGet, "/go/account/password", nil)
	accountRequest.AddCookie(websession.Cookie(authenticated.ID))
	accountResponse := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(accountResponse, accountRequest)
	if accountResponse.Code != http.StatusOK || !strings.Contains(accountResponse.Body.String(), `<html lang="en">`) ||
		!strings.Contains(accountResponse.Body.String(), "Interface language") || !strings.Contains(accountResponse.Body.String(), "Change my password") {
		t.Fatalf("English account=%d %q", accountResponse.Code, accountResponse.Body.String())
	}

	users.user.TOTPSecret = sql.NullString{String: "secret", Valid: true}
	users.user.TOTPEnabledAt = sql.NullString{String: "2026-09-04T10:00:00Z", Valid: true}
	pending, err := dependencies.Sessions.Create(websession.StateTwoFactor, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	challengeRequest := httptest.NewRequest(http.MethodGet, "/go/two-factor", nil)
	challengeRequest.AddCookie(websession.Cookie(pending.ID))
	challengeResponse := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(challengeResponse, challengeRequest)
	if challengeResponse.Code != http.StatusOK || !strings.Contains(challengeResponse.Body.String(), "Two-factor authentication") ||
		!strings.Contains(challengeResponse.Body.String(), "Verification code") {
		t.Fatalf("English 2FA=%d %q", challengeResponse.Code, challengeResponse.Body.String())
	}
}

func TestLoginExplainsRequiredRootInitialization(t *testing.T) {
	users := &fakeLoginUsers{rootMissing: true}
	response := httptest.NewRecorder()
	Handler(testDependencies(t, users)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/login", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Initialisation obligatoire") || !strings.Contains(response.Body.String(), "sudo aegisadmin initialize") {
		t.Fatalf("unexpected uninitialized login response: %d %q", response.Code, response.Body.String())
	}
}

func TestRequestAddressUsesLocalReverseProxyHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:43120"
	request.Header.Set("X-Forwarded-For", "198.51.100.8, 192.0.2.24")
	if got := requestAddress(request); got != "192.0.2.24" {
		t.Fatalf("requestAddress() = %q, want %q", got, "192.0.2.24")
	}
}

func TestRequestAddressIgnoresProxyHeaderFromRemotePeer(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.42:43120"
	request.Header.Set("X-Forwarded-For", "198.51.100.8")
	if got := requestAddress(request); got != "203.0.113.42" {
		t.Fatalf("requestAddress() = %q, want direct peer address", got)
	}
}

func TestSystemRedirectsToGoDashboard(t *testing.T) {
	response := httptest.NewRecorder()
	Handler(testDependencies(t, nil)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/system", nil))
	if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != "/go/dashboard" {
		t.Fatalf("unexpected system redirect: %d %q", response.Code, response.Header().Get("Location"))
	}
}

func TestReadiness(t *testing.T) {
	response := httptest.NewRecorder()
	Handler(testDependencies(t, nil)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"backend":"ok"`) || !strings.Contains(response.Body.String(), `"database":"ok"`) {
		t.Fatalf("unexpected ready response: %d %q", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	dependencies := testDependencies(t, nil)
	dependencies.Readiness = ReadinessChecks{}
	Handler(dependencies).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"backend":"unavailable"`) {
		t.Fatalf("unexpected unavailable response: %d %q", response.Code, response.Body.String())
	}
}

func TestServerTLSMinimum(t *testing.T) {
	server := Server("127.0.0.1:0", Handler(testDependencies(t, nil)))
	if server.TLSConfig == nil || server.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected TLS configuration: %#v", server.TLSConfig)
	}
}

func TestLoginFlowRotatesSession(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("valid password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := authstore.User{ID: 1, Login: "root", PasswordHash: string(hash), Type: "root", Status: "active", AuthVersion: 2}
	users := &fakeLoginUsers{user: user}
	handler := Handler(testDependencies(t, users))

	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, httptest.NewRequest(http.MethodGet, "/login", nil))
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %#v", cookies)
	}
	match := regexp.MustCompile(`name="_token" value="([^"]+)"`).FindStringSubmatch(loginResponse.Body.String())
	if len(match) != 2 {
		t.Fatal("CSRF token missing from login form")
	}
	form := url.Values{"_token": {match[1]}, "login": {"root"}, "password": {"valid password"}}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(cookies[0])
	request.RemoteAddr = "192.0.2.10:12345"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/go/dashboard" {
		t.Fatalf("login response = %d, location = %q", response.Code, response.Header().Get("Location"))
	}
	responseCookies := response.Result().Cookies()
	var rotatedSession *http.Cookie
	var restoredTheme *http.Cookie
	for _, cookie := range responseCookies {
		switch cookie.Name {
		case websession.CookieName:
			rotatedSession = cookie
		case "aegisadmin_theme":
			restoredTheme = cookie
		}
	}
	if rotatedSession == nil || rotatedSession.Value == cookies[0].Value {
		t.Fatalf("session was not rotated: %#v", responseCookies)
	}
	if restoredTheme == nil || restoredTheme.Value != "dark" || !restoredTheme.Secure {
		t.Fatalf("profile theme was not restored: %#v", responseCookies)
	}

	dashboardRequest := httptest.NewRequest(http.MethodGet, "/go/dashboard", nil)
	dashboardRequest.AddCookie(rotatedSession)
	dashboardResponse := httptest.NewRecorder()
	handler.ServeHTTP(dashboardResponse, dashboardRequest)
	if dashboardResponse.Code != http.StatusOK || !strings.Contains(dashboardResponse.Body.String(), "Tableau de bord") ||
		!strings.Contains(dashboardResponse.Body.String(), `/assets/app.js?v=`) ||
		strings.Contains(dashboardResponse.Body.String(), "Modules accessibles") ||
		strings.Contains(dashboardResponse.Body.String(), `class="server-summary"`) ||
		strings.Contains(dashboardResponse.Body.String(), "Disponibilité") ||
		!strings.Contains(dashboardResponse.Body.String(), "serveur-test") ||
		!strings.Contains(dashboardResponse.Body.String(), `data-dashboard-uptime-seconds="183840"`) ||
		!strings.Contains(dashboardResponse.Body.String(), "23,0 %") {
		t.Fatalf("dashboard response = %d %q", dashboardResponse.Code, dashboardResponse.Body.String())
	}
	logoutToken := regexp.MustCompile(`name="_token" value="([^"]+)"`).FindStringSubmatch(dashboardResponse.Body.String())
	if len(logoutToken) != 2 {
		t.Fatal("logout CSRF token is missing")
	}
	accountRequest := httptest.NewRequest(http.MethodGet, "/go/account/password", nil)
	accountRequest.AddCookie(rotatedSession)
	accountResponse := httptest.NewRecorder()
	handler.ServeHTTP(accountResponse, accountRequest)
	if accountResponse.Code != http.StatusOK || !strings.Contains(accountResponse.Body.String(), "Changer mon mot de passe") ||
		!strings.Contains(accountResponse.Body.String(), `name="language"`) {
		t.Fatalf("account password response = %d %q", accountResponse.Code, accountResponse.Body.String())
	}
	storageRequest := httptest.NewRequest(http.MethodGet, "/storage", nil)
	storageRequest.AddCookie(rotatedSession)
	storageResponse := httptest.NewRecorder()
	handler.ServeHTTP(storageResponse, storageRequest)
	if storageResponse.Code != http.StatusOK || !strings.Contains(storageResponse.Body.String(), "Stockage") ||
		!strings.Contains(storageResponse.Body.String(), "/dev/root") ||
		!strings.Contains(storageResponse.Body.String(), `data-sort-default-column="0"`) ||
		!strings.Contains(storageResponse.Body.String(), `data-sort-column="8"`) ||
		!strings.Contains(storageResponse.Body.String(), `data-sort-value="107374182400"`) ||
		!strings.Contains(storageResponse.Body.String(), `/assets/app.js?v=`) ||
		!strings.Contains(storageResponse.Body.String(), "Droit Modification") {
		t.Fatalf("storage response = %d %q", storageResponse.Code, storageResponse.Body.String())
	}
	logoutForm := url.Values{"_token": {logoutToken[1]}}
	logoutRequest := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(logoutForm.Encode()))
	logoutRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	logoutRequest.AddCookie(rotatedSession)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusSeeOther || logoutResponse.Header().Get("Location") != "/login" ||
		logoutResponse.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("unexpected logout response: %d %#v", logoutResponse.Code, logoutResponse.Result().Cookies())
	}
	afterLogout := httptest.NewRequest(http.MethodGet, "/go/dashboard", nil)
	afterLogout.AddCookie(rotatedSession)
	afterLogoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(afterLogoutResponse, afterLogout)
	if afterLogoutResponse.Code != http.StatusSeeOther || afterLogoutResponse.Header().Get("Location") != "/login" {
		t.Fatalf("destroyed session remains usable: %d", afterLogoutResponse.Code)
	}
}

func TestStorageRejectsDirectAccessWithoutPermission(t *testing.T) {
	users := &fakeLoginUsers{denyStorage: true, user: authstore.User{
		ID: 9, Login: "reader", Type: "user", Status: "active", AuthVersion: 1,
	}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/storage", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("storage response code = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestServicesConsultationHidesActionsAndRejectsRestart(t *testing.T) {
	users := &fakeLoginUsers{servicesPermission: "view", language: "en", user: authstore.User{
		ID: 10, Login: "reader", Type: "user", Status: "active", AuthVersion: 1,
	}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/services", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "apache2") ||
		!strings.Contains(response.Body.String(), "Authorized services") || !strings.Contains(response.Body.String(), "Automatic startup") ||
		strings.Contains(response.Body.String(), "Restart") || strings.Contains(response.Body.String(), "Services autorisés") {
		t.Fatalf("consultation response = %d %q", response.Code, response.Body.String())
	}

	form := url.Values{"_token": {session.CSRFToken}, "service": {"apache2"}}
	restart := httptest.NewRequest(http.MethodPost, "/services/restart", strings.NewReader(form.Encode()))
	restart.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	restart.AddCookie(websession.Cookie(session.ID))
	restartResponse := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(restartResponse, restart)
	if restartResponse.Code != http.StatusForbidden || users.restartedService != "" {
		t.Fatalf("restart response = %d, restarted = %q", restartResponse.Code, users.restartedService)
	}
}

func TestServicesActionCanRestart(t *testing.T) {
	users := &fakeLoginUsers{servicesPermission: "action", user: authstore.User{
		ID: 11, Login: "operator", Type: "user", Status: "active", AuthVersion: 1,
	}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"_token": {session.CSRFToken}, "service": {"apache2"}}
	request := httptest.NewRequest(http.MethodPost, "/services/restart", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/services?result=restarted" || users.restartedService != "apache2" {
		t.Fatalf("restart response = %d %q, restarted = %q", response.Code, response.Header().Get("Location"), users.restartedService)
	}
}

func TestNetworkRequiresPermissionAndRendersDetails(t *testing.T) {
	users := &fakeLoginUsers{networkPermission: "view", language: "en", user: authstore.User{ID: 12, Login: "reader", Type: "user", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/network?interface=eth0", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "192.0.2.2/24") || !strings.Contains(response.Body.String(), "00:11:22:33:44:55") ||
		!strings.Contains(response.Body.String(), `class="network-selected-button" aria-current="true">Selected</span>`) ||
		!strings.Contains(response.Body.String(), "Detected interfaces") || strings.Contains(response.Body.String(), "Sélectionnée") {
		t.Fatalf("network response = %d %q", response.Code, response.Body.String())
	}

	users.networkPermission = ""
	denied := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("network denied response = %d", denied.Code)
	}
}

func TestLogsRequiresPermissionAndEscapesContent(t *testing.T) {
	users := &fakeLoginUsers{logsPermission: "view", language: "en", user: authstore.User{ID: 13, Login: "reader", Type: "user", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/logs?source=apache2%2Ferror.log&level=error&keyword=test&lines=750", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "2026-08-17 erreur de test") ||
		!strings.Contains(response.Body.String(), "apache2/error.log") ||
		!strings.Contains(response.Body.String(), `name="level"`) ||
		!strings.Contains(response.Body.String(), `name="lines" type="number" min="1" max="5000" step="1" value="750"`) ||
		!strings.Contains(response.Body.String(), `value="test"`) ||
		!strings.Contains(response.Body.String(), `href="/logs"`) ||
		!strings.Contains(response.Body.String(), "Selection and search") ||
		!strings.Contains(response.Body.String(), "1 line displayed out of 1 total") ||
		strings.Contains(response.Body.String(), "Sélection et recherche") {
		t.Fatalf("logs response = %d %q", response.Code, response.Body.String())
	}
	users.logsPermission = ""
	denied := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("logs denied response = %d", denied.Code)
	}
}

func TestLogsRejectsInvalidLineCount(t *testing.T) {
	users := &fakeLoginUsers{logsPermission: "view", user: authstore.User{ID: 13, Login: "reader", Type: "user", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"0", "5001", "12.5", "-1"} {
		request := httptest.NewRequest(http.MethodGet, "/logs?lines="+url.QueryEscape(value), nil)
		request.AddCookie(websession.Cookie(session.ID))
		response := httptest.NewRecorder()
		Handler(dependencies).ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("lines %q response = %d", value, response.Code)
		}
	}
}

func TestFilterLogLinesByLevelAndKeyword(t *testing.T) {
	lines := []string{"INFO service démarré", "WARNING disque presque plein", "ERROR Échec de connexion", "ligne sans niveau"}
	filtered := filterLogLines(lines, "error", "connexion")
	if len(filtered) != 1 || filtered[0] != lines[2] {
		t.Fatalf("filtered logs = %#v", filtered)
	}
	if warning := filterLogLines(lines, "warning", ""); len(warning) != 1 || warning[0] != lines[1] {
		t.Fatalf("warning logs = %#v", warning)
	}
}

func TestSnapshotPresentationStructuresAndEscapesData(t *testing.T) {
	sections := []any{map[string]any{"id": "system", "label": "Système", "available": true, "data": map[string]any{"hostname": "serveur<script>", "active": true, "addresses": []any{"192.0.2.1"}}}}
	rendered := renderSnapshotSections(sections, "fr")
	if !strings.Contains(rendered, `href="#snapshot-section-0"`) ||
		!strings.Contains(rendered, "serveur&lt;script&gt;") ||
		!strings.Contains(rendered, "Actif") ||
		strings.Contains(rendered, "serveur<script>") {
		t.Fatalf("snapshot rendering = %q", rendered)
	}
	comparison := renderComparisonSections([]any{map[string]any{"label": "PHP", "status": "different", "source": map[string]any{"version": "8.4"}, "target": map[string]any{"version": "8.5"}}}, "fr")
	if !strings.Contains(comparison, "Modifiée") || !strings.Contains(comparison, "Snapshot source") || !strings.Contains(comparison, "8.5") {
		t.Fatalf("comparison rendering = %q", comparison)
	}
	identical := renderComparisonSections([]any{map[string]any{"label": "PHP", "status": "identical"}}, "fr")
	if !strings.Contains(identical, "Aucune différence") || strings.Contains(identical, "<h2>PHP</h2>") {
		t.Fatalf("identical sections should be hidden: %q", identical)
	}
}

func TestRenderSnapshotsUsesOneTableRowPerSnapshot(t *testing.T) {
	rendered := renderSnapshots(map[string]any{"snapshots": []any{map[string]any{"id": "snapshot-id", "name": "Référence", "created_at": "2026-08-25T12:00:00Z", "source": "local", "valid": true, "available_sections": 13, "total_sections": 14, "size": 2048}}}, "token", "fr")
	if !strings.Contains(rendered, `<table class="data-table snapshot-table">`) ||
		!strings.Contains(rendered, `<th>Actions</th><th>Nom</th>`) ||
		strings.Count(rendered, `<tr>`) != 2 ||
		!strings.Contains(rendered, "13 / 14") || !strings.Contains(rendered, "2,0 Kio") {
		t.Fatalf("snapshot table = %q", rendered)
	}
}

func TestStructuredDifferencesHideUnchangedValues(t *testing.T) {
	rendered := renderStructuredDifferences(
		map[string]any{"hostname": "server", "service": map[string]any{"active": true, "version": "1.0"}},
		map[string]any{"hostname": "server", "service": map[string]any{"active": true, "version": "2.0"}},
		"fr",
	)
	if !strings.Contains(rendered, "Service › Version") || !strings.Contains(rendered, "1.0") || !strings.Contains(rendered, "2.0") ||
		strings.Contains(rendered, "Hostname") || strings.Contains(rendered, "Actif") {
		t.Fatalf("structured differences = %q", rendered)
	}
}

func TestStructuredDifferencesMatchListsByIdentityAndIgnoreOrder(t *testing.T) {
	source := map[string]any{"items": []any{
		map[string]any{"name": "apt", "version": "3.2.0"},
		map[string]any{"name": "appstream", "version": "0.25build1"},
	}}
	target := map[string]any{"items": []any{
		map[string]any{"name": "appstream", "version": "0.25build1"},
		map[string]any{"name": "apt", "version": "3.2.1"},
	}}
	rendered := renderStructuredDifferences(source, target, "fr")
	if !strings.Contains(rendered, "Nom apt › Version") || !strings.Contains(rendered, "3.2.0") || !strings.Contains(rendered, "3.2.1") ||
		strings.Contains(rendered, "appstream") || strings.Contains(rendered, "Élément 1") {
		t.Fatalf("identity-aware differences = %q", rendered)
	}
	if reordered := renderStructuredDifferences([]any{"b", "a"}, []any{"a", "b"}, "fr"); !strings.Contains(reordered, "Aucune valeur différente") {
		t.Fatalf("scalar order should be ignored: %q", reordered)
	}
}

func TestStructuredDifferencesMatchApacheVirtualHostsByConfigFile(t *testing.T) {
	source := []any{
		map[string]any{"config_file": "/etc/apache2/sites-enabled/default.conf", "server_name": "", "port": 80, "config_id": "default-hash", "document_root": "/var/www/html"},
		map[string]any{"config_file": "/etc/apache2/sites-enabled/shared.conf", "server_name": "app.example", "port": 80, "config_id": "app-hash", "document_root": "/var/www/app"},
		map[string]any{"config_file": "/etc/apache2/sites-enabled/shared.conf", "server_name": "blog.example", "port": 80, "config_id": "blog-hash", "document_root": "/var/www/blog"},
	}
	target := []any{
		map[string]any{"config_file": "/etc/apache2/sites-enabled/shared.conf", "server_name": "blog.example", "port": 80, "config_id": "blog-hash", "document_root": "/var/www/blog"},
		map[string]any{"config_file": "/etc/apache2/sites-enabled/default.conf", "server_name": "", "port": 80, "config_id": "default-hash", "document_root": "/var/www/html"},
		map[string]any{"config_file": "/etc/apache2/sites-enabled/shared.conf", "server_name": "app.example", "port": 80, "config_id": "app-hash", "document_root": "/srv/www/app"},
	}
	rendered := renderStructuredDifferences(source, target, "fr")
	if !strings.Contains(rendered, "VirtualHost shared.conf · app.example · port 80 › Racine du site") ||
		!strings.Contains(rendered, "/var/www/app") || !strings.Contains(rendered, "/srv/www/app") ||
		strings.Contains(rendered, "blog.example") || strings.Contains(rendered, "default.conf") || strings.Contains(rendered, "Élément 1") {
		t.Fatalf("Apache configuration differences = %q", rendered)
	}
}

func TestStructuredDifferencesMatchCronJobsWithoutLineNoise(t *testing.T) {
	source := []any{map[string]any{"id": "readonly-old", "source": "/etc/crontab", "user": "root", "schedule": "@daily", "command": "/usr/local/bin/backup", "line": 12, "managed": false}}
	target := []any{map[string]any{"id": "readonly-new", "source": "/etc/crontab", "user": "root", "schedule": "@hourly", "command": "/usr/local/bin/backup", "line": 18, "managed": false}}
	rendered := renderStructuredDifferences(source, target, "fr")
	if !strings.Contains(rendered, "Tâche root · /usr/local/bin/backup › Schedule") ||
		!strings.Contains(rendered, "@daily") || !strings.Contains(rendered, "@hourly") ||
		strings.Contains(rendered, "readonly-old") || strings.Contains(rendered, "Line") {
		t.Fatalf("Cron differences = %q", rendered)
	}
}

func TestStructuredDifferencesRecognizeCronMigrationToManagedTask(t *testing.T) {
	source := []any{map[string]any{"id": "legacy-hash", "source": "/var/spool/cron/crontabs/www-data", "user": "www-data", "schedule": "45 1 * * *", "command": "curl https://example.com/maintenance.php", "line": 3, "managed": false, "enabled": true, "editable": true}}
	target := []any{map[string]any{"id": "7391c994-dc56-42a2-b2af-edc1fd35ad9d", "source": "/var/spool/cron/crontabs/www-data", "user": "www-data", "schedule": "45 1 * * *", "command": "curl https://example.com/maintenance.php", "line": 4, "managed": true, "enabled": true, "editable": true}}
	rendered := renderStructuredDifferences(source, target, "fr")
	if strings.Count(rendered, "Tâche www-data · curl https://example.com/maintenance.php") != 1 ||
		!strings.Contains(rendered, "Managed") || !strings.Contains(rendered, "Non") || !strings.Contains(rendered, "Oui") ||
		strings.Contains(rendered, "7391c994") || strings.Contains(rendered, "Command") || strings.Contains(rendered, "Absent") {
		t.Fatalf("Cron managed migration = %q", rendered)
	}
}

func TestStructuredDifferencesIgnoreFirewallRenumbering(t *testing.T) {
	rule22 := map[string]any{"id": 1, "action": "allow", "direction": "in", "protocol": "tcp", "ports": []any{"22"}, "source": "any", "destination": "any", "family": "ipv4"}
	rule443 := map[string]any{"id": 2, "action": "allow", "direction": "in", "protocol": "tcp", "ports": []any{"443"}, "source": "any", "destination": "any", "family": "ipv4"}
	reordered22 := map[string]any{"id": 2, "action": "allow", "direction": "in", "protocol": "tcp", "ports": []any{"22"}, "source": "any", "destination": "any", "family": "ipv4"}
	reordered443 := map[string]any{"id": 1, "action": "allow", "direction": "in", "protocol": "tcp", "ports": []any{"443"}, "source": "any", "destination": "any", "family": "ipv4"}
	rendered := renderStructuredDifferences([]any{rule22, rule443}, []any{reordered443, reordered22}, "fr")
	if !strings.Contains(rendered, "Aucune valeur différente") || strings.Contains(rendered, "Identifiant") {
		t.Fatalf("firewall renumbering = %q", rendered)
	}
}

func TestStructuredDifferencesNormalizeLegacyListeners(t *testing.T) {
	source := []any{"tcp LISTEN 0 5 127.0.0.1:555 0.0.0.0:*", "tcp LISTEN 0 511 *:8443 *:*"}
	target := []any{map[string]any{"protocol": "tcp", "state": "LISTEN", "address": "*", "port": "8443"}, map[string]any{"protocol": "tcp", "state": "LISTEN", "address": "127.0.0.1", "port": "555"}}
	rendered := renderStructuredDifferences(source, target, "fr")
	if !strings.Contains(rendered, "Aucune valeur différente") || strings.Contains(rendered, "Valeur tcp") {
		t.Fatalf("listener normalization = %q", rendered)
	}
	added := renderStructuredDifferences(source, append(target, map[string]any{"protocol": "tcp", "state": "LISTEN", "address": "127.0.0.1", "port": "9080"}), "fr")
	if !strings.Contains(added, "Écoute tcp 127.0.0.1:9080") || strings.Contains(added, "127.0.0.1:555") {
		t.Fatalf("listener addition = %q", added)
	}
}

func TestSnapshotComparisonOptionsUseNamesAndDistinctDefaults(t *testing.T) {
	data := map[string]any{"snapshots": []any{
		map[string]any{"id": "new", "name": "Après mise à jour", "created_at": "2026-08-25T12:00:00Z", "valid": true},
		map[string]any{"id": "old", "name": "Avant mise à jour", "created_at": "2026-08-24T12:00:00Z", "valid": true},
	}}
	source, target, disabled := renderSnapshotCompareOptions(data, "fr")
	if disabled != "" || !strings.Contains(source, `value="old" selected`) ||
		!strings.Contains(target, `value="new" selected`) ||
		!strings.Contains(source, "Avant mise à jour") {
		t.Fatalf("source=%q target=%q disabled=%q", source, target, disabled)
	}
	_, _, disabled = renderSnapshotCompareOptions(map[string]any{"snapshots": []any{map[string]any{"id": "only", "valid": true}}}, "fr")
	if disabled != " disabled" {
		t.Fatalf("single snapshot comparison should be disabled: %q", disabled)
	}
}

func TestAboutUsesApplicationVersionAndIsAvailableToUser(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 14, Login: "reader", Type: "user", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/about", nil)
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "À propos d’AegisAdmin") || !strings.Contains(response.Body.String(), buildinfo.Version) || !strings.Contains(response.Body.String(), "GNU AGPL-3.0") {
		t.Fatalf("about response = %d %q", response.Code, response.Body.String())
	}
}

func TestPHPConsultationHidesRestartAndActionAllowsIt(t *testing.T) {
	users := &fakeLoginUsers{phpPermission: "view", language: "en", user: authstore.User{ID: 15, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 15, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/php", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "8.5.9") ||
		!strings.Contains(response.Body.String(), "PHP CLI and PHP-FPM monitoring") ||
		!strings.Contains(response.Body.String(), "Automatic startup") ||
		strings.Contains(response.Body.String(), "Restart") || strings.Contains(response.Body.String(), "Démarrage automatique") {
		t.Fatalf("PHP consultation = %d %q", response.Code, response.Body.String())
	}
	users.phpPermission = "action"
	form := url.Values{"_token": {session.CSRFToken}, "runtime": {"fpm-8.5"}}
	post := httptest.NewRequest(http.MethodPost, "/php/restart", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	restarted := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(restarted, post)
	if restarted.Code != http.StatusSeeOther || users.restartedPHP != "fpm-8.5" {
		t.Fatalf("PHP restart = %d %q", restarted.Code, users.restartedPHP)
	}
}

func TestMySQLConsultationHidesRestartAndActionAllowsIt(t *testing.T) {
	users := &fakeLoginUsers{mysqlPermission: "view", language: "en", user: authstore.User{ID: 16, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	d := testDependencies(t, users)
	session, err := d.Sessions.Create(websession.StateAuthenticated, 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/mysql", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(d).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "MariaDB 11.8") ||
		!strings.Contains(response.Body.String(), "Database server monitoring") ||
		!strings.Contains(response.Body.String(), "Active connections") ||
		!strings.Contains(response.Body.String(), "1.0 KiB") ||
		strings.Contains(response.Body.String(), "Restart MariaDB") || strings.Contains(response.Body.String(), "Connexions actives") {
		t.Fatalf("mysql consultation=%d %q", response.Code, response.Body.String())
	}
	users.mysqlPermission = "action"
	form := url.Values{"_token": {session.CSRFToken}}
	post := httptest.NewRequest(http.MethodPost, "/mysql/restart", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	restarted := httptest.NewRecorder()
	Handler(d).ServeHTTP(restarted, post)
	if restarted.Code != http.StatusSeeOther || !users.mysqlRestarted {
		t.Fatalf("mysql restart=%d %v", restarted.Code, users.mysqlRestarted)
	}
}

func TestTorConsultationHidesActionsAndActionAllowsReload(t *testing.T) {
	users := &fakeLoginUsers{torPermission: "view", language: "en", user: authstore.User{ID: 17, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	d := testDependencies(t, users)
	session, err := d.Sessions.Create(websession.StateAuthenticated, 17, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/tor", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(d).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Tor 0.4.8") ||
		!strings.Contains(response.Body.String(), "Tor instance and Onion services monitoring") ||
		!strings.Contains(response.Body.String(), "The Tor configuration is valid.") ||
		!strings.Contains(response.Body.String(), "1.0 KiB") ||
		!strings.Contains(response.Body.String(), "Onion service") ||
		strings.Contains(response.Body.String(), "Reload Tor") || strings.Contains(response.Body.String(), "Service Onion") {
		t.Fatalf("tor consultation=%d %q", response.Code, response.Body.String())
	}
	users.torPermission = "action"
	form := url.Values{"_token": {session.CSRFToken}}
	post := httptest.NewRequest(http.MethodPost, "/tor/reload", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	done := httptest.NewRecorder()
	Handler(d).ServeHTTP(done, post)
	if done.Code != http.StatusSeeOther || users.torAction != "reload" {
		t.Fatalf("tor action=%d %q", done.Code, users.torAction)
	}
}

func TestApacheConsultationHidesActionsAndActionAllowsReload(t *testing.T) {
	users := &fakeLoginUsers{apachePermission: "view", language: "en", user: authstore.User{ID: 18, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	d := testDependencies(t, users)
	session, err := d.Sessions.Create(websession.StateAuthenticated, 18, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/apache", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(d).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "2.4.65") ||
		!strings.Contains(response.Body.String(), "HTTP server monitoring") ||
		!strings.Contains(response.Body.String(), "Loaded VirtualHosts") ||
		!strings.Contains(response.Body.String(), "Read only") ||
		strings.Contains(response.Body.String(), "Reload Apache") || strings.Contains(response.Body.String(), "Sites actifs") {
		t.Fatalf("apache consultation=%d %q", response.Code, response.Body.String())
	}
	users.apachePermission = "action"
	form := url.Values{"_token": {session.CSRFToken}}
	post := httptest.NewRequest(http.MethodPost, "/apache/reload", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	done := httptest.NewRecorder()
	Handler(d).ServeHTTP(done, post)
	if done.Code != http.StatusSeeOther || users.apacheAction != "reload" {
		t.Fatalf("apache action=%d %q", done.Code, users.apacheAction)
	}
	users.apachePermission = "modify"
	modifyPage := httptest.NewRecorder()
	Handler(d).ServeHTTP(modifyPage, get)
	if modifyPage.Code != http.StatusOK || !strings.Contains(modifyPage.Body.String(), "Add a site") ||
		!strings.Contains(modifyPage.Body.String(), "Add an Apache site") || strings.Contains(modifyPage.Body.String(), "Ajouter un site") {
		t.Fatalf("Apache modify page=%d %q", modifyPage.Code, modifyPage.Body.String())
	}
	configID := strings.Repeat("a", 64)
	configRequest := httptest.NewRequest(http.MethodGet, "/apache/sites/"+configID, nil)
	configRequest.AddCookie(websession.Cookie(session.ID))
	configResponse := httptest.NewRecorder()
	Handler(d).ServeHTTP(configResponse, configRequest)
	if configResponse.Code != http.StatusOK || !strings.Contains(configResponse.Body.String(), `"filename":"site.conf"`) {
		t.Fatalf("apache config=%d %q", configResponse.Code, configResponse.Body.String())
	}
	modifyForm := url.Values{"_token": {session.CSRFToken}, "config_id": {configID}}
	modifyRequest := httptest.NewRequest(http.MethodPost, "/apache/disable", strings.NewReader(modifyForm.Encode()))
	modifyRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	modifyRequest.AddCookie(websession.Cookie(session.ID))
	modifyResponse := httptest.NewRecorder()
	Handler(d).ServeHTTP(modifyResponse, modifyRequest)
	if modifyResponse.Code != http.StatusSeeOther || users.apacheAction != "disable" {
		t.Fatalf("apache modification=%d %q", modifyResponse.Code, users.apacheAction)
	}
}

func TestCertbotIssueValuesNormalizeDomainsAndExplainInvalidValues(t *testing.T) {
	domains, err := certbotIssueValues(" admin@example.org ", "Example.ORG, www.example.org example.org.")
	if err != nil || len(domains) != 2 || domains[0] != "example.org" || domains[1] != "www.example.org" {
		t.Fatalf("domains=%#v err=%v", domains, err)
	}
	if _, err = certbotIssueValues("admin@example.org", "server-local"); err == nil || !strings.Contains(err.Error(), "server-local") {
		t.Fatalf("invalid domain error=%v", err)
	}
	if _, err = certbotIssueValues("admin", "example.org"); err == nil || !strings.Contains(err.Error(), "e-mail") {
		t.Fatalf("invalid email error=%v", err)
	}
}

func TestRenderCertificatesPlacesActionMenuFirst(t *testing.T) {
	row := renderCertificates([]webcertbot.Certificate{{Name: "example.org", Domains: []string{"example.org"}, Valid: true, DaysRemaining: 60}}, "token", true, "fr")
	action := strings.Index(row, `data-certbot-certificate-action`)
	name := strings.Index(row, `<th>example.org</th>`)
	if action < 0 || name < 0 || action > name || strings.Contains(row, `danger-button`) {
		t.Fatalf("certificate row=%q", row)
	}
}

func TestRenderCertificatesSortsByDomains(t *testing.T) {
	rows := renderCertificates([]webcertbot.Certificate{
		{Name: "zeta", Domains: []string{"zeta.example"}, Valid: true},
		{Name: "alpha", Domains: []string{"alpha.example"}, Valid: true},
	}, "token", false, "fr")
	if alpha, zeta := strings.Index(rows, "alpha.example"), strings.Index(rows, "zeta.example"); alpha < 0 || zeta < 0 || alpha > zeta {
		t.Fatalf("certificate rows=%q", rows)
	}
}

func TestCronLibraryAndCertificatesUseEnglish(t *testing.T) {
	library := renderBackupLibrary("token", `<option>root</option>`, "en")
	if !strings.Contains(library, "Cron task library") || strings.Contains(library, "Bibliothèque de tâches Cron") {
		t.Fatalf("cron library=%q", library)
	}
	rows := renderCertificates([]webcertbot.Certificate{{Name: "example.org", Domains: []string{"example.org"}, Valid: true, DaysRemaining: 60}}, "token", true, "en")
	if !strings.Contains(rows, "Valid") || !strings.Contains(rows, "60 d") || strings.Contains(rows, "Réinstaller") {
		t.Fatalf("certificate rows=%q", rows)
	}
}

func TestFail2banRightsSeparateActionsAndModification(t *testing.T) {
	users := &fakeLoginUsers{fail2banPermission: "view", user: authstore.User{ID: 19, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	d := testDependencies(t, users)
	session, err := d.Sessions.Create(websession.StateAuthenticated, 19, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/fail2ban", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(d).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "192.0.2.4") || strings.Contains(response.Body.String(), "Débannir") || strings.Contains(response.Body.String(), "Bannir une adresse") || strings.Contains(response.Body.String(), "Recharger") {
		t.Fatalf("fail2ban view=%d %q", response.Code, response.Body.String())
	}
	users.fail2banPermission = "action"
	response = httptest.NewRecorder()
	Handler(d).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "192.0.2.4") || !strings.Contains(response.Body.String(), "Recharger") || strings.Contains(response.Body.String(), "Débannir") {
		t.Fatalf("fail2ban action=%d %q", response.Code, response.Body.String())
	}
	form := url.Values{"_token": {session.CSRFToken}, "jail": {"sshd"}, "address": {"192.0.2.4"}}
	post := httptest.NewRequest(http.MethodPost, "/fail2ban/unban", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	denied := httptest.NewRecorder()
	Handler(d).ServeHTTP(denied, post)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unban action right=%d", denied.Code)
	}
	users.fail2banPermission = "modify"
	allowed := httptest.NewRecorder()
	Handler(d).ServeHTTP(allowed, post)
	if allowed.Code != http.StatusSeeOther || users.fail2banAction != "unban" {
		t.Fatalf("unban modify=%d %q", allowed.Code, users.fail2banAction)
	}
}

func TestFirewallRightsSeparateActionsAndModification(t *testing.T) {
	users := &fakeLoginUsers{firewallPermission: "action", user: authstore.User{ID: 20, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	d := testDependencies(t, users)
	session, err := d.Sessions.Create(websession.StateAuthenticated, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/firewall", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(d).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Recharger le pare-feu") || strings.Contains(response.Body.String(), "Ajouter une règle") {
		t.Fatalf("firewall action=%d %q", response.Code, response.Body.String())
	}
	stateForm := url.Values{"_token": {session.CSRFToken}}
	stateRequest := httptest.NewRequest(http.MethodPost, "/firewall/disable", strings.NewReader(stateForm.Encode()))
	stateRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	stateRequest.AddCookie(websession.Cookie(session.ID))
	stateResponse := httptest.NewRecorder()
	Handler(d).ServeHTTP(stateResponse, stateRequest)
	if stateResponse.Code != http.StatusSeeOther || users.firewallAction != "disable" {
		t.Fatalf("firewall disable=%d %q", stateResponse.Code, users.firewallAction)
	}
	form := url.Values{"_token": {session.CSRFToken}, "id": {"1"}}
	post := httptest.NewRequest(http.MethodPost, "/firewall/delete", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	denied := httptest.NewRecorder()
	Handler(d).ServeHTTP(denied, post)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("delete action=%d", denied.Code)
	}
	users.firewallPermission = "modify"
	allowed := httptest.NewRecorder()
	Handler(d).ServeHTTP(allowed, post)
	if allowed.Code != http.StatusSeeOther || users.firewallAction != "delete" {
		t.Fatalf("delete modify=%d %q", allowed.Code, users.firewallAction)
	}
}

func TestRenderFirewallRulesSortsPortsAndPlacesActionFirst(t *testing.T) {
	rows := renderFirewallRulesWithServices([]webfirewall.Rule{{ID: 1, Protocol: "tcp", Ports: []string{"9080"}}, {ID: 2, Protocol: "tcp", Ports: []string{"22"}}}, "token", true, map[string]string{"22/tcp": "ssh"}, "fr")
	port22, port9080 := strings.Index(rows, ">22 (ssh)</td>"), strings.Index(rows, ">9080</td>")
	deleteAction, firstID := strings.Index(rows, `action="/firewall/delete"`), strings.Index(rows, `<th>2</th>`)
	if port22 < 0 || port9080 < 0 || port22 > port9080 || deleteAction < 0 || deleteAction > firstID {
		t.Fatalf("firewall rows=%q", rows)
	}
}

func TestFirewallServicesAndPortFormatting(t *testing.T) {
	services := parseFirewallServices("http 80/tcp www\ndomain 53/udp # DNS\ninvalid value\n")
	if services["80/tcp"] != "http" || services["53/udp"] != "domain" {
		t.Fatalf("services=%#v", services)
	}
	formatted := formatFirewallPorts([]string{"80,443", "1000:1010"}, "tcp", map[string]string{"80/tcp": "http", "443/tcp": "https"})
	if formatted != "80 (http), 443 (https), 1000:1010" {
		t.Fatalf("formatted ports=%q", formatted)
	}
}

func TestFirewallProfileIsDisplayedWhenPortsAreEmpty(t *testing.T) {
	rule := webfirewall.Rule{ID: 1, Protocol: "any", Destination: "Apache Full"}
	if target := formatFirewallRuleTarget(rule, map[string]string{}); target != "Apache Full" {
		t.Fatalf("profile target=%q", target)
	}
	rows := renderFirewallRulesWithServices([]webfirewall.Rule{rule}, "token", false, map[string]string{}, "fr")
	if !strings.Contains(rows, ">Apache Full</td>") {
		t.Fatalf("firewall profile row=%q", rows)
	}
}

func TestFail2banAndFirewallRowsUseEnglish(t *testing.T) {
	selector := renderJailSelector([]string{"sshd"}, "sshd", "en")
	_, actions := renderJail(webfail2ban.Snapshot{Selected: "sshd", Jail: &webfail2ban.Jail{CurrentlyBanned: 1, BannedIPs: []string{"192.0.2.4"}}}, "token", true, "en")
	if !strings.Contains(selector, ">Show</button>") || !strings.Contains(actions, ">Unban</button>") || strings.Contains(actions, "Débannir") {
		t.Fatalf("selector=%q actions=%q", selector, actions)
	}
	rows := renderFirewallRulesWithServices([]webfirewall.Rule{{ID: 1, Protocol: "tcp", Ports: []string{"443"}}}, "token", true, map[string]string{"443/tcp": "https"}, "en")
	if !strings.Contains(rows, ">Delete</button>") || strings.Contains(rows, ">Supprimer</button>") {
		t.Fatalf("firewall rows=%q", rows)
	}
}

func TestUpdatesUseEnglish(t *testing.T) {
	page := localizeUpdatesHTML(`<html lang="fr"><h1>Mises à jour</h1><h2>Paquets disponibles</h2><p>Sécurité</p><p>Détails</p><p>Installation des mises à jour</p>`, "en")
	if !strings.Contains(page, `lang="en"`) || !strings.Contains(page, "Available packages") || !strings.Contains(page, "Security") || !strings.Contains(page, ">Details<") || strings.Contains(page, "Detailss") || strings.Contains(page, "Mises à jour") {
		t.Fatalf("updates page=%q", page)
	}
	rows := renderUpdates([]webupdates.Update{{Name: "example", Security: true}}, "en")
	if !strings.Contains(rows, ">Yes</td>") {
		t.Fatalf("updates rows=%q", rows)
	}
}

func TestImmediateRebootReturnsReconnectPageBeforeShutdown(t *testing.T) {
	users := &fakeLoginUsers{user: authstore.User{ID: 1, Login: "root", Type: "root", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	updates := &fakeUpdatesProvider{}
	dependencies.Updates = updates
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, users.user.ID, users.user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"_token": {session.CSRFToken}, "confirmation": {"reboot"}, "delay": {"0"}}
	request := httptest.NewRequest(http.MethodPost, "/updates/reboot", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !updates.rebootCalled || updates.rebootDelay != 0 ||
		!strings.Contains(response.Body.String(), `data-reboot-active="true"`) ||
		!strings.Contains(response.Body.String(), "attendra automatiquement le retour du serveur") {
		t.Fatalf("immediate reboot response=%d delay=%d body=%q", response.Code, updates.rebootDelay, response.Body.String())
	}
}

type fakeUpdatesProvider struct {
	rebootCalled bool
	rebootDelay  int
}

func (*fakeUpdatesProvider) Snapshot(context.Context) (webupdates.Snapshot, error) {
	return webupdates.Snapshot{}, nil
}
func (*fakeUpdatesProvider) Summary(context.Context) (webupdates.Summary, error) {
	return webupdates.Summary{}, nil
}
func (*fakeUpdatesProvider) StartUpgrade(context.Context) (string, error) { return "", nil }
func (*fakeUpdatesProvider) Job(context.Context, string) (webupdates.Job, error) {
	return webupdates.Job{}, nil
}
func (*fakeUpdatesProvider) RefreshComposer(context.Context) error { return nil }
func (f *fakeUpdatesProvider) Reboot(_ context.Context, delay int) error {
	f.rebootCalled = true
	f.rebootDelay = delay
	return nil
}

func TestUsersUseEnglish(t *testing.T) {
	page := localizeUsersHTML(`<html lang="fr"><h1>Utilisateurs</h1><p>Comptes enregistrés</p><button>Nouvel utilisateur</button><h2>Sécurité</h2><span>Consultation</span>`, "en")
	for _, expected := range []string{`lang="en"`, "Users", "Registered accounts", "New user", "Security", "View"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("missing %q in %q", expected, page)
		}
	}
}

func TestModulesAndSettingsUseEnglish(t *testing.T) {
	modules := localizeModulesHTML(`<html lang="fr"><h2>Organisation du menu</h2><input placeholder="Nouvelle catégorie"><button>Ajouter</button>`, "en")
	if !strings.Contains(modules, "Menu organization") || !strings.Contains(modules, "New category") || strings.Contains(modules, "Organisation du menu") {
		t.Fatalf("modules=%q", modules)
	}
	navigation := renderAdminNavigation([]authstore.AdminCategory{{ID: 1, Name: "Sécurité", Modules: []authstore.AdminModule{}}}, "token", "en")
	if !strings.Contains(navigation, `value="Sécurité"`) || !strings.Contains(navigation, "Delete category") {
		t.Fatalf("navigation=%q", navigation)
	}
	settings := localizeSettingsHTML(`<html lang="fr"><h1>Paramètres</h1><h2>Serveur de messagerie SMTP</h2><span>Mot de passe enregistré</span><button>Restaurer la sauvegarde</button>`, "en")
	if !strings.Contains(settings, "SMTP mail server") || !strings.Contains(settings, "Password saved") || !strings.Contains(settings, "Restore backup") || strings.Contains(settings, "Paramètres") {
		t.Fatalf("settings=%q", settings)
	}
}

func TestConfigurationUsesEnglishAndPreservesSnapshotName(t *testing.T) {
	template := localizeConfigurationTemplate(`<html lang="fr"><h1>Configuration serveur</h1><h2>Comparer des snapshots</h2>`, "en")
	if !strings.Contains(template, "Server configuration") || !strings.Contains(template, "Compare snapshots") || strings.Contains(template, `lang="fr"`) {
		t.Fatalf("configuration template=%q", template)
	}
	rows := renderSnapshots(map[string]any{"snapshots": []any{map[string]any{"id": "snapshot-id", "name": "Référence France", "created_at": "2026-08-25T12:00:00Z", "source": "local", "valid": true, "available_sections": 14, "total_sections": 14, "size": 2048}}}, "token", "en")
	if !strings.Contains(rows, `value="Référence France"`) || !strings.Contains(rows, ">Open</a>") || !strings.Contains(rows, ">Valid</span>") {
		t.Fatalf("configuration rows=%q", rows)
	}
}

func TestCronRightsSeparateActionsAndModification(t *testing.T) {
	users := &fakeLoginUsers{cronPermission: "action", user: authstore.User{ID: 21, Login: "operator", Type: "user", Status: "active", AuthVersion: 1}}
	dependencies := testDependencies(t, users)
	session, err := dependencies.Sessions.Create(websession.StateAuthenticated, 21, 1)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/cron", nil)
	get.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(response, get)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Exécuter") || strings.Contains(response.Body.String(), "Créer une tâche") || strings.Contains(response.Body.String(), "Supprimer") {
		t.Fatalf("cron action=%d %q", response.Code, response.Body.String())
	}
	form := url.Values{"_token": {session.CSRFToken}, "user": {"deploy"}, "task_id": {"legacy-1234567890abcdef1234567890abcdef"}}
	post := httptest.NewRequest(http.MethodPost, "/cron/delete", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	denied := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(denied, post)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("delete action=%d", denied.Code)
	}
	users.cronPermission = "modify"
	post = httptest.NewRequest(http.MethodPost, "/cron/delete", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(websession.Cookie(session.ID))
	allowed := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(allowed, post)
	if allowed.Code != http.StatusSeeOther || users.cronAction != "delete" {
		t.Fatalf("delete modify=%d %q", allowed.Code, users.cronAction)
	}
	resultRequest := httptest.NewRequest(http.MethodGet, "/cron/executions/1234567890abcdef1234567890abcdef", nil)
	resultRequest.AddCookie(websession.Cookie(session.ID))
	resultResponse := httptest.NewRecorder()
	Handler(dependencies).ServeHTTP(resultResponse, resultRequest)
	if resultResponse.Code != http.StatusOK || !strings.Contains(resultResponse.Body.String(), `"stdout":"ok"`) {
		t.Fatalf("cron result=%d %q", resultResponse.Code, resultResponse.Body.String())
	}
}

func TestCronStateIsFirstAndMySQLQueriesUseFrenchThousands(t *testing.T) {
	row := renderCronJobs([]webcron.Job{{User: "deploy", Enabled: true}}, "token", "view", "fr")
	if state, user := strings.Index(row, "status-badge"), strings.Index(row, "<th>deploy</th>"); state < 0 || user < 0 || state > user {
		t.Fatalf("cron row=%q", row)
	}
	if formatted := formatFrenchInteger(1234567); formatted != "1\u202f234\u202f567" {
		t.Fatalf("formatted=%q", formatted)
	}
	metrics := renderMySQLMetrics(map[string]int64{"queries": 1234567}, "fr")
	if !strings.Contains(metrics, "1\u202f234\u202f567") {
		t.Fatalf("metrics=%q", metrics)
	}
	if english := renderMySQLMetrics(map[string]int64{"queries": 1234567}, "en"); !strings.Contains(english, "1,234,567") {
		t.Fatalf("English metrics=%q", english)
	}
}

func TestDashboardCardsAndCronRowsUseEnglish(t *testing.T) {
	cards := localizeDashboardCards([]webdashboard.Card{{ID: "memory", Title: "Mémoire", Value: "42,5 %", Subtitle: "1,0 Gio disponibles · 2,0 Gio utilisés sur 3,0 Gio"}}, "en")
	if cards[0].Title != "Memory" || cards[0].Value != "42.5 %" || !strings.Contains(cards[0].Subtitle, "1.0 GiB available") {
		t.Fatalf("English dashboard card=%#v", cards[0])
	}
	row := renderCronJobs([]webcron.Job{{User: "deploy", Enabled: false, Editable: false}}, "token", "action", "en")
	if !strings.Contains(row, "Suspended") || !strings.Contains(row, "Read only") || strings.Contains(row, "Suspendue") {
		t.Fatalf("English Cron row=%q", row)
	}
}

func TestTwoFactorFlowRotatesPendingSession(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("valid password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := authstore.User{
		ID: 2, Login: "admin", PasswordHash: string(hash), Status: "active", AuthVersion: 3,
		TOTPSecret:    sql.NullString{String: "SECRET", Valid: true},
		TOTPEnabledAt: sql.NullString{String: "2026-08-17T00:00:00Z", Valid: true},
	}
	users := &fakeLoginUsers{user: user}
	dependencies := testDependencies(t, users)
	dependencies.TwoFactor = fixedTwoFactorVerifier(true)
	handler := Handler(dependencies)

	loginCookie, loginToken := beginLogin(t, handler)
	form := url.Values{"_token": {loginToken}, "login": {"admin"}, "password": {"valid password"}}
	loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(loginCookie)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Header().Get("Location") != "/go/two-factor" {
		t.Fatalf("unexpected second-factor location: %q", loginResponse.Header().Get("Location"))
	}
	pendingCookie := loginResponse.Result().Cookies()[0]

	challengeRequest := httptest.NewRequest(http.MethodGet, "/go/two-factor", nil)
	challengeRequest.AddCookie(pendingCookie)
	challengeResponse := httptest.NewRecorder()
	handler.ServeHTTP(challengeResponse, challengeRequest)
	match := regexp.MustCompile(`name="_token" value="([^"]+)"`).FindStringSubmatch(challengeResponse.Body.String())
	if challengeResponse.Code != http.StatusOK || len(match) != 2 {
		t.Fatalf("unexpected challenge response: %d %q", challengeResponse.Code, challengeResponse.Body.String())
	}

	verifyForm := url.Values{"_token": {match[1]}, "code": {"123456"}}
	verifyRequest := httptest.NewRequest(http.MethodPost, "/go/two-factor", strings.NewReader(verifyForm.Encode()))
	verifyRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	verifyRequest.AddCookie(pendingCookie)
	verifyResponse := httptest.NewRecorder()
	handler.ServeHTTP(verifyResponse, verifyRequest)
	if verifyResponse.Code != http.StatusSeeOther || verifyResponse.Header().Get("Location") != "/go/dashboard" {
		t.Fatalf("unexpected verification response: %d %q", verifyResponse.Code, verifyResponse.Header().Get("Location"))
	}
	authenticatedCookie := verifyResponse.Result().Cookies()[0]
	if authenticatedCookie.Value == pendingCookie.Value {
		t.Fatal("pending session was not rotated")
	}
}

func TestTwoFactorRejectsInvalidCode(t *testing.T) {
	user := authstore.User{
		ID: 2, Login: "admin", Status: "active", AuthVersion: 3,
		TOTPSecret:    sql.NullString{String: "SECRET", Valid: true},
		TOTPEnabledAt: sql.NullString{String: "2026-08-17T00:00:00Z", Valid: true},
	}
	users := &fakeLoginUsers{user: user}
	dependencies := testDependencies(t, users)
	dependencies.TwoFactor = fixedTwoFactorVerifier(false)
	session, err := dependencies.Sessions.Create(websession.StateTwoFactor, user.ID, user.AuthVersion)
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(dependencies)
	form := url.Values{"_token": {session.CSRFToken}, "code": {"000000"}}
	request := httptest.NewRequest(http.MethodPost, "/go/two-factor", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(websession.Cookie(session.ID))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "code de vérification") {
		t.Fatalf("unexpected invalid-code response: %d %q", response.Code, response.Body.String())
	}
}

func TestRequiredTwoFactorEnrollment(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("valid password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := &fakeLoginUsers{user: authstore.User{
		ID: 3, Login: "operator", PasswordHash: string(hash), Status: "active",
		AuthVersion: 7, TwoFactorRequired: true,
	}}
	dependencies := testDependencies(t, users)
	dependencies.TwoFactor = fixedTwoFactorVerifier(true)
	handler := Handler(dependencies)

	loginCookie, loginToken := beginLogin(t, handler)
	loginForm := url.Values{"_token": {loginToken}, "login": {"operator"}, "password": {"valid password"}}
	loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(loginForm.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(loginCookie)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Header().Get("Location") != "/go/two-factor/setup" {
		t.Fatalf("unexpected enrollment location: %q", loginResponse.Header().Get("Location"))
	}
	pendingCookie := loginResponse.Result().Cookies()[0]

	setupRequest := httptest.NewRequest(http.MethodGet, "/go/two-factor/setup", nil)
	setupRequest.AddCookie(pendingCookie)
	setupResponse := httptest.NewRecorder()
	handler.ServeHTTP(setupResponse, setupRequest)
	match := regexp.MustCompile(`name="_token" value="([^"]+)"`).FindStringSubmatch(setupResponse.Body.String())
	if setupResponse.Code != http.StatusOK || len(match) != 2 ||
		!strings.Contains(setupResponse.Body.String(), "data:image/png;base64,") ||
		!users.user.TOTPSecret.Valid {
		t.Fatalf("unexpected setup response: %d %q", setupResponse.Code, setupResponse.Body.String())
	}

	confirmForm := url.Values{"_token": {match[1]}, "code": {"123456"}}
	confirmRequest := httptest.NewRequest(http.MethodPost, "/go/two-factor/setup", strings.NewReader(confirmForm.Encode()))
	confirmRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	confirmRequest.AddCookie(pendingCookie)
	confirmResponse := httptest.NewRecorder()
	handler.ServeHTTP(confirmResponse, confirmRequest)
	if confirmResponse.Code != http.StatusSeeOther || confirmResponse.Header().Get("Location") != "/go/dashboard" ||
		!users.user.TOTPEnabledAt.Valid || users.user.AuthVersion != 8 {
		t.Fatalf("unexpected confirmation: %d %q user=%#v", confirmResponse.Code, confirmResponse.Header().Get("Location"), users.user)
	}
}

func TestRequiredPasswordChange(t *testing.T) {
	const currentPassword = "Mot de passe initial valide"
	const newPassword = "Nouveau mot de passe valide"
	hash, err := bcrypt.GenerateFromPassword([]byte(currentPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := &fakeLoginUsers{user: authstore.User{
		ID: 4, Login: "operator", PasswordHash: string(hash), Status: "active",
		AuthVersion: 11, MustChangePassword: true,
	}}
	dependencies := testDependencies(t, users)
	handler := Handler(dependencies)

	loginCookie, loginToken := beginLogin(t, handler)
	loginForm := url.Values{"_token": {loginToken}, "login": {"operator"}, "password": {currentPassword}}
	loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(loginForm.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(loginCookie)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Header().Get("Location") != "/go/account/password" {
		t.Fatalf("unexpected password location: %q", loginResponse.Header().Get("Location"))
	}
	pendingCookie := loginResponse.Result().Cookies()[0]

	formRequest := httptest.NewRequest(http.MethodGet, "/go/account/password", nil)
	formRequest.AddCookie(pendingCookie)
	formResponse := httptest.NewRecorder()
	handler.ServeHTTP(formResponse, formRequest)
	match := regexp.MustCompile(`name="_token" value="([^"]+)"`).FindStringSubmatch(formResponse.Body.String())
	if formResponse.Code != http.StatusOK || len(match) != 2 {
		t.Fatalf("unexpected password form: %d %q", formResponse.Code, formResponse.Body.String())
	}

	changeForm := url.Values{
		"_token": {match[1]}, "current_password": {currentPassword},
		"new_password": {newPassword}, "new_password_confirmation": {newPassword},
	}
	changeRequest := httptest.NewRequest(http.MethodPost, "/go/account/password", strings.NewReader(changeForm.Encode()))
	changeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	changeRequest.AddCookie(pendingCookie)
	changeResponse := httptest.NewRecorder()
	handler.ServeHTTP(changeResponse, changeRequest)
	if changeResponse.Code != http.StatusSeeOther || changeResponse.Header().Get("Location") != "/go/dashboard" ||
		users.user.MustChangePassword || users.user.AuthVersion != 12 ||
		!webauth.VerifyPassword(newPassword, users.user.PasswordHash) {
		t.Fatalf("unexpected password change: %d %q user=%#v", changeResponse.Code, changeResponse.Header().Get("Location"), users.user)
	}
	if changeResponse.Result().Cookies()[0].Value == pendingCookie.Value {
		t.Fatal("password session was not rotated")
	}
}

func beginLogin(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/login", nil))
	match := regexp.MustCompile(`name="_token" value="([^"]+)"`).FindStringSubmatch(response.Body.String())
	if len(match) != 2 || len(response.Result().Cookies()) != 1 {
		t.Fatal("login session was not initialized")
	}
	return response.Result().Cookies()[0], match[1]
}

type fixedTwoFactorVerifier bool

func (v fixedTwoFactorVerifier) Verify(_, _ string) bool { return bool(v) }

func testSessions(t *testing.T) *websession.Manager {
	t.Helper()
	manager, err := websession.New(websession.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func testDependencies(t *testing.T, users *fakeLoginUsers) Dependencies {
	t.Helper()
	dependencies := Dependencies{
		Readiness: readyChecks(), Sessions: testSessions(t), LoginLimiter: webauth.NewLoginLimiter(),
		TwoFactor: webauth.NewTOTPVerifier(), TwoFactorLimiter: webauth.NewLoginLimiter(),
		PasswordLimiter: webauth.NewLoginLimiter(),
	}
	if users != nil {
		dependencies.Authenticator = webauth.New(users)
		dependencies.Users = users
		dependencies.AccessLog = users
		dependencies.Enrollment = users
		dependencies.Passwords = users
		dependencies.Navigation = users
		dependencies.Dashboard = users
		dependencies.Authorization = users
		dependencies.Storage = users
		dependencies.Services = users
		dependencies.Network = users
		dependencies.Logs = users
		dependencies.PHP = users
		dependencies.MySQL = users
		dependencies.Tor = users
		dependencies.Apache = users
		dependencies.Fail2ban = users
		dependencies.Firewall = users
		dependencies.Cron = users
	}
	return dependencies
}

type fakeLoginUsers struct {
	user               authstore.User
	theme              string
	language           string
	rootMissing        bool
	denyStorage        bool
	servicesPermission string
	networkPermission  string
	logsPermission     string
	phpPermission      string
	restartedPHP       string
	mysqlPermission    string
	mysqlRestarted     bool
	torPermission      string
	torAction          string
	apachePermission   string
	apacheAction       string
	fail2banPermission string
	fail2banAction     string
	firewallPermission string
	firewallAction     string
	accessEvents       []string
	cronPermission     string
	cronAction         string
	restartedService   string
}

func (f *fakeLoginUsers) RecordAccess(_ context.Context, _ *int64, _, event string, _ bool, _, _ string) error {
	f.accessEvents = append(f.accessEvents, event)
	return nil
}
func (f *fakeLoginUsers) AccessLog(context.Context, authstore.AccessLogFilter) (authstore.AccessLogResult, error) {
	return authstore.AccessLogResult{Items: []authstore.AccessLogEntry{{Login: "root", Event: "login_success", Success: true, IPAddress: "192.0.2.1", OccurredAt: "2026-08-25T10:00:00Z"}}, Total: 1, Page: 1, PerPage: 50}, nil
}
func (f *fakeLoginUsers) AccessLogUsers(context.Context) ([]string, error) {
	return []string{"root", "operator"}, nil
}

func (f *fakeLoginUsers) FindByLogin(_ context.Context, login string) (authstore.User, bool, error) {
	return f.user, strings.EqualFold(login, f.user.Login), nil
}
func (f *fakeLoginUsers) FindRoot(context.Context) (authstore.User, bool, error) {
	return f.user, !f.rootMissing, nil
}
func (f *fakeLoginUsers) ThemeForUser(context.Context, int64) (string, error) {
	if f.theme == "" {
		return "dark", nil
	}
	return f.theme, nil
}
func (f *fakeLoginUsers) SetTheme(_ context.Context, _ int64, theme string) error {
	f.theme = theme
	return nil
}
func (f *fakeLoginUsers) LanguageForUser(context.Context, int64) (string, error) {
	return f.language, nil
}
func (f *fakeLoginUsers) SetLanguage(_ context.Context, _ int64, language string) error {
	f.language = language
	return nil
}
func (f *fakeLoginUsers) FindByID(_ context.Context, id int64) (authstore.User, bool, error) {
	return f.user, id == f.user.ID, nil
}

func (f *fakeLoginUsers) PrepareTOTP(_ context.Context, id, authVersion int64, secret string) (authstore.User, error) {
	if id != f.user.ID || authVersion != f.user.AuthVersion {
		return authstore.User{}, context.Canceled
	}
	if !f.user.TOTPSecret.Valid {
		f.user.TOTPSecret = sql.NullString{String: secret, Valid: true}
	}
	return f.user, nil
}

func (f *fakeLoginUsers) EnableTOTP(_ context.Context, id, authVersion int64, secret string) (authstore.User, error) {
	if id != f.user.ID || authVersion != f.user.AuthVersion || secret != f.user.TOTPSecret.String {
		return authstore.User{}, context.Canceled
	}
	f.user.TOTPEnabledAt = sql.NullString{String: "2026-08-17T00:00:00Z", Valid: true}
	f.user.AuthVersion++
	return f.user, nil
}

func (f *fakeLoginUsers) DisableTOTP(_ context.Context, id, authVersion int64) (authstore.User, error) {
	if id != f.user.ID || authVersion != f.user.AuthVersion || f.user.TwoFactorRequired {
		return authstore.User{}, errors.New("invalid TOTP disable")
	}
	f.user.TOTPSecret = sql.NullString{}
	f.user.TOTPEnabledAt = sql.NullString{}
	f.user.AuthVersion++
	return f.user, nil
}

func (f *fakeLoginUsers) ChangePassword(_ context.Context, id, authVersion int64, currentHash, newHash string) (authstore.User, error) {
	if id != f.user.ID || authVersion != f.user.AuthVersion || currentHash != f.user.PasswordHash {
		return authstore.User{}, context.Canceled
	}
	f.user.PasswordHash = newHash
	f.user.MustChangePassword = false
	f.user.AuthVersion++
	return f.user, nil
}

func (f *fakeLoginUsers) Menu(_ context.Context, user authstore.User) ([]authstore.NavigationCategory, error) {
	if user.ID != f.user.ID {
		return nil, context.Canceled
	}
	return []authstore.NavigationCategory{{
		ID: 1, Name: "Supervision", Modules: []authstore.NavigationModule{
			{ID: 1, Key: "dashboard", Name: "Dashboard", Icon: "D", PermissionLevel: "view"},
			{ID: 2, Key: "storage", Name: "Stockage", Icon: "S", PermissionLevel: "action"},
			{ID: 3, Key: "services", Name: "Services", Icon: "V", PermissionLevel: "action"},
			{ID: 4, Key: "network", Name: "Réseau", Icon: "R", PermissionLevel: "view"},
			{ID: 5, Key: "logs", Name: "Journaux", Icon: "L", PermissionLevel: "view"},
			{ID: 6, Key: "about", Name: "À propos de…", Icon: "A", PermissionLevel: "view"},
			{ID: 7, Key: "php", Name: "PHP", Icon: "P", PermissionLevel: "action"},
			{ID: 8, Key: "mysql", Name: "MySQL", Icon: "M", PermissionLevel: "action"},
			{ID: 9, Key: "tor", Name: "Tor", Icon: "T", PermissionLevel: "action"},
			{ID: 10, Key: "apache", Name: "Apache", Icon: "H", PermissionLevel: "action"},
			{ID: 11, Key: "fail2ban", Name: "Fail2ban", Icon: "F", PermissionLevel: "action"},
			{ID: 12, Key: "firewall", Name: "Pare-feu", Icon: "W", PermissionLevel: "action"},
			{ID: 13, Key: "cron", Name: "Cron", Icon: "C", PermissionLevel: "action"},
		},
	}}, nil
}

func (f *fakeLoginUsers) PermissionForModule(_ context.Context, user authstore.User, module string) (string, bool, error) {
	if user.ID != f.user.ID {
		return "", false, context.Canceled
	}
	if module == "storage" && f.denyStorage {
		return "", false, nil
	}
	if user.Type == "root" {
		return "modify", true, nil
	}
	if module == "about" {
		return "view", true, nil
	}
	if module == "services" {
		if f.servicesPermission == "" {
			return "", false, nil
		}
		return f.servicesPermission, true, nil
	}
	if module == "network" {
		if f.networkPermission == "" {
			return "", false, nil
		}
		return f.networkPermission, true, nil
	}
	if module == "logs" {
		if f.logsPermission == "" {
			return "", false, nil
		}
		return f.logsPermission, true, nil
	}
	if module == "php" {
		if f.phpPermission == "" {
			return "", false, nil
		}
		return f.phpPermission, true, nil
	}
	if module == "mysql" {
		if f.mysqlPermission == "" {
			return "", false, nil
		}
		return f.mysqlPermission, true, nil
	}
	if module == "tor" {
		if f.torPermission == "" {
			return "", false, nil
		}
		return f.torPermission, true, nil
	}
	if module == "apache" {
		if f.apachePermission == "" {
			return "", false, nil
		}
		return f.apachePermission, true, nil
	}
	if module == "fail2ban" {
		if f.fail2banPermission == "" {
			return "", false, nil
		}
		return f.fail2banPermission, true, nil
	}
	if module == "firewall" {
		if f.firewallPermission == "" {
			return "", false, nil
		}
		return f.firewallPermission, true, nil
	}
	if module == "cron" {
		if f.cronPermission == "" {
			return "", false, nil
		}
		return f.cronPermission, true, nil
	}
	if module != "storage" {
		return "", false, nil
	}
	return "action", true, nil
}

func (f *fakeLoginUsers) Snapshot(context.Context) (webdashboard.Snapshot, error) {
	return webdashboard.Snapshot{
		Hostname: "serveur-test", System: "Debian test", Kernel: "6.12 · amd64", Uptime: "2 j 3 h 4 min", UptimeSeconds: 183840,
		Information: []webdashboard.Information{
			{Label: "Nom d’hôte", Value: "serveur-test"},
			{Label: "Version", Value: "13"},
			{Label: "Noyau", Value: "6.12"},
			{Label: "Durée de fonctionnement", Value: "2 j 3 h 4 min"},
		},
		Cards: []webdashboard.Card{
			{Title: "Processeur", Value: "23,0 %", Subtitle: "4 cœurs", Status: "success"},
			{Title: "Mémoire", Value: "81,0 %", Subtitle: "6,5 Gio utilisés", Status: "warning"},
		},
	}, nil
}

func (f *fakeLoginUsers) Resources(context.Context) ([]webdashboard.Card, error) {
	return []webdashboard.Card{{ID: "cpu", Title: "Processeur", Value: "24,0 %", Subtitle: "4 cœurs", Status: "success"}}, nil
}

func (f *fakeLoginUsers) Supervision(context.Context) ([]webdashboard.Card, error) {
	return []webdashboard.Card{{ID: "storage", Title: "Stockage", Value: "1 volume", Subtitle: "État normal", Status: "success", URL: "/storage"}}, nil
}

func (f *fakeLoginUsers) Processes(context.Context) (webdashboard.ProcessSnapshot, error) {
	return webdashboard.ProcessSnapshot{Total: 1, Returned: 1, Limit: 500, Processes: []webdashboard.Process{{PID: "42", Name: "test", User: "root", State: "R", StateLabel: "En cours", Status: "success", CPU: "1,0 %", MemoryPercent: "2,0 %", Memory: "3,0 Mio", Elapsed: "1 min"}}}, nil
}

func (f *fakeLoginUsers) StorageSnapshot(context.Context) (webstorage.Snapshot, error) {
	return webstorage.Snapshot{
		Mounts: []webstorage.Mount{{
			Mount: "/", Source: "/dev/root", PhysicalDevice: "/dev/sda", Filesystem: "ext4", Percent: 42,
			Size: 107374182400, Used: 45097156608, Available: 62277025792,
			SizeLabel: "100 Gio", UsedLabel: "42 Gio", AvailableLabel: "58 Gio", Status: "success", StatusLabel: "Normal",
		}},
		Summary: webstorage.Summary{Total: 1, Normal: 1},
	}, nil
}

func (f *fakeLoginUsers) ServicesSnapshot(context.Context) (webservices.Snapshot, error) {
	return webservices.Snapshot{
		Services: []webservices.Service{{ID: "apache2", Exists: true, Active: true, Enabled: true, State: "running", Status: "success", StatusLabel: "Actif"}},
		Summary:  webservices.Summary{Total: 1, Installed: 1, Active: 1},
	}, nil
}

func (f *fakeLoginUsers) RestartService(_ context.Context, service string) error {
	f.restartedService = service
	return nil
}

func (f *fakeLoginUsers) NetworkSnapshot(context.Context, string) (webnetwork.Snapshot, error) {
	mac, mtu := "00:11:22:33:44:55", 1500
	item := webnetwork.Interface{ID: "eth0", Name: "eth0", Type: "ethernet", State: "up", TypeLabel: "Ethernet", Status: "success", StatusLabel: "Active", MAC: &mac, MTU: &mtu, IPv4: []webnetwork.Address{{Address: "192.0.2.2", Prefix: 24}}}
	return webnetwork.Snapshot{Interfaces: []webnetwork.Interface{item}, Selected: &item, Summary: webnetwork.Summary{Total: 1, Up: 1}}, nil
}

func (f *fakeLoginUsers) LogsSnapshot(context.Context, string, int) (weblogs.Snapshot, error) {
	return weblogs.Snapshot{Sources: []string{"apache2/error.log"}, Selected: "apache2/error.log", Lines: []string{"2026-08-17 erreur de test"}, Total: 1}, nil
}

func (f *fakeLoginUsers) LogSources(context.Context) ([]string, error) {
	return []string{"apache2/access.log", "apache2/error.log"}, nil
}

func (f *fakeLoginUsers) PHPSnapshot(context.Context) (webphp.Snapshot, error) {
	return webphp.Snapshot{CLI: webphp.CLI{Version: "8.5.9", SAPI: "cli", IniFile: "/etc/php.ini", ScanDir: "/etc/php.d"}, Instances: []webphp.Instance{{ID: "fpm-8.5", Version: "8.5", Service: "php8.5-fpm", Exists: true, Active: true, Enabled: true, State: "running", Status: "success", StatusLabel: "Active"}}}, nil
}
func (f *fakeLoginUsers) RestartPHP(_ context.Context, runtime string) error {
	f.restartedPHP = runtime
	return nil
}
func (f *fakeLoginUsers) MySQLSnapshot(context.Context) (webmysql.Snapshot, error) {
	unit := "mariadb.service"
	return webmysql.Snapshot{Server: webmysql.Server{Product: "MariaDB", Version: "11.8", Hostname: "db", Port: 3306, Socket: "/run/mysql.sock", DataDirectory: "/var/lib/mysql", DefaultStorageEngine: "InnoDB", Service: webmysql.Service{Unit: &unit, Exists: true, Active: true, Enabled: true, State: "running"}}, Metrics: map[string]int64{"threads_connected": 3, "threads_running": 1, "queries": 42, "slow_queries": 0}, Databases: []webmysql.Database{{Name: "app", Kind: "user", Tables: 5, Size: 1024}}}, nil
}
func (f *fakeLoginUsers) MySQLMetrics(context.Context) (map[string]int64, error) {
	return map[string]int64{"threads_connected": 3, "threads_running": 1, "queries": 42, "slow_queries": 0}, nil
}
func (f *fakeLoginUsers) RestartMySQL(context.Context) error { f.mysqlRestarted = true; return nil }

func (f *fakeLoginUsers) TorSnapshot(context.Context) (webtor.Snapshot, error) {
	bootstrap := 100
	host := "example.onion"
	return webtor.Snapshot{Installed: true, Info: webtor.Info{Product: "Tor", Version: "0.4.8", ConfigFile: "/etc/tor/torrc", Service: "tor@default", Unit: "tor@default.service"}, Status: webtor.Status{Exists: true, Active: true, Enabled: true, State: "running", MainPID: 12, Memory: 1024, Tasks: 4, Bootstrap: &bootstrap}, ConfigurationValid: true, ConfigurationMessage: "Configuration valide", Services: []webtor.Onion{{ID: "site", Hostname: &host}}}, nil
}
func (f *fakeLoginUsers) TorAction(_ context.Context, action string) error {
	f.torAction = action
	return nil
}
func (f *fakeLoginUsers) ApacheSnapshot(context.Context) (webapache.Snapshot, error) {
	root := "/var/www/site"
	return webapache.Snapshot{Version: "2.4.65", Built: "2026", ConfigValid: true, ConfigMessage: "Syntax OK", VHosts: []webapache.VHost{{ServerName: "example.fr", Port: 443, ConfigFile: "site.conf", DocumentRoot: &root}}, Sites: []webapache.Site{{Filename: "site.conf", ConfigID: strings.Repeat("a", 64), Enabled: true, ServerNames: []string{"example.fr"}, Ports: []int{443}}}, Modules: []webapache.Module{{Name: "ssl", Type: "shared"}}}, nil
}
func (f *fakeLoginUsers) ApacheAction(_ context.Context, action string) error {
	f.apacheAction = action
	return nil
}
func (f *fakeLoginUsers) ApacheSite(_ context.Context, id string) (webapache.SiteConfig, error) {
	return webapache.SiteConfig{Filename: "site.conf", ConfigID: id, Enabled: true, Content: "<VirtualHost *:80>\n</VirtualHost>\n"}, nil
}
func (f *fakeLoginUsers) ApacheSiteAction(_ context.Context, action, _, _, _ string) error {
	f.apacheAction = action
	return nil
}
func (f *fakeLoginUsers) Fail2banSnapshot(context.Context, string) (webfail2ban.Snapshot, error) {
	return webfail2ban.Snapshot{Installed: true, Info: webfail2ban.Info{Product: "Fail2ban", Version: "1.1"}, Status: webfail2ban.Status{Exists: true, Active: true, Enabled: true, State: "running", Jails: []string{"sshd"}}, ConfigValid: true, ConfigMessage: "Configuration valide", Selected: "sshd", Jail: &webfail2ban.Jail{CurrentlyBanned: 1, BannedIPs: []string{"192.0.2.4"}}}, nil
}
func (f *fakeLoginUsers) Action(_ context.Context, action, jail, address string) error {
	f.fail2banAction = action
	return nil
}
func (f *fakeLoginUsers) FirewallSnapshot(context.Context) (webfirewall.Snapshot, error) {
	version := "0.36"
	ipv6 := true
	return webfirewall.Snapshot{Info: webfirewall.Info{Product: "UFW", Version: &version, Active: true, IPv6: &ipv6, DefaultIncoming: "deny", DefaultOutgoing: "allow"}, Rules: []webfirewall.Rule{{ID: 1, Action: "allow", Direction: "in", Protocol: "tcp", Ports: []string{"22"}, Source: "any", Family: "ipv4"}}}, nil
}
func (f *fakeLoginUsers) Reload(context.Context) error { f.firewallAction = "reload"; return nil }
func (f *fakeLoginUsers) SetEnabled(_ context.Context, enabled bool) error {
	if enabled {
		f.firewallAction = "enable"
	} else {
		f.firewallAction = "disable"
	}
	return nil
}
func (f *fakeLoginUsers) Add(context.Context, string, string, string, string) error {
	f.firewallAction = "add"
	return nil
}
func (f *fakeLoginUsers) Delete(context.Context, int) error { f.firewallAction = "delete"; return nil }
func (f *fakeLoginUsers) CronSnapshot(context.Context) (webcron.Snapshot, error) {
	return webcron.Snapshot{Info: webcron.Info{Product: "Cron", Version: "3.0", Service: "cron", Unit: "cron.service"}, Status: webcron.Status{Exists: true, Active: true, Enabled: true, State: "running"}, Users: []webcron.User{{Name: "deploy"}}, Jobs: []webcron.Job{{ID: "legacy-1234567890abcdef1234567890abcdef", User: "deploy", Schedule: "@daily", Command: "/bin/true", Source: "/var/spool/cron/deploy", Enabled: true, Editable: true}}}, nil
}
func (f *fakeLoginUsers) CronAction(_ context.Context, action, user, id, schedule, command string) (string, error) {
	f.cronAction = action
	if action == "run" {
		return "1234567890abcdef1234567890abcdef", nil
	}
	return "", nil
}

func (f *fakeLoginUsers) CronBackupCreate(_ context.Context, _ webcron.BackupRequest) error {
	return nil
}
func (f *fakeLoginUsers) CronBackupAction(_ context.Context, _, _ string) (string, error) {
	return "0123456789abcdef0123456789abcdef", nil
}
func (f *fakeLoginUsers) CronResult(_ context.Context, id string) (webcron.ExecutionResult, error) {
	exit := 0
	return webcron.ExecutionResult{ExecutionID: id, Status: "finished", ExitCode: &exit, DurationMS: 12, Stdout: "ok"}, nil
}

func readyChecks() ReadinessChecks {
	ready := func() error { return nil }
	return ReadinessChecks{Backend: ready, Database: ready}
}
