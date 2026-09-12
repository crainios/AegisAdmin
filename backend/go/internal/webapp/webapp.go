package webapp

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/authstore"
	"aegisadmin/backend/internal/buildinfo"
	"aegisadmin/backend/internal/i18n"
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
	"aegisadmin/backend/internal/websettings"
	"aegisadmin/backend/internal/webstorage"
	"aegisadmin/backend/internal/webtor"
	"aegisadmin/backend/internal/webupdates"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed assets
var embeddedAssets embed.FS

type healthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Backend  string `json:"backend,omitempty"`
	Database string `json:"database,omitempty"`
}

type ReadinessCheck func() error

type ReadinessChecks struct {
	Backend  ReadinessCheck
	Database ReadinessCheck
}

type UserLookup interface {
	FindByID(context.Context, int64) (authstore.User, bool, error)
	FindRoot(context.Context) (authstore.User, bool, error)
	ThemeForUser(context.Context, int64) (string, error)
	SetTheme(context.Context, int64, string) error
	LanguageForUser(context.Context, int64) (string, error)
	SetLanguage(context.Context, int64, string) error
}
type AccessLogStore interface {
	RecordAccess(context.Context, *int64, string, string, bool, string, string) error
	AccessLog(context.Context, authstore.AccessLogFilter) (authstore.AccessLogResult, error)
	AccessLogUsers(context.Context) ([]string, error)
}

type TwoFactorVerifier interface {
	Verify(secret, code string) bool
}

type TwoFactorEnrollmentStore interface {
	PrepareTOTP(context.Context, int64, int64, string) (authstore.User, error)
	EnableTOTP(context.Context, int64, int64, string) (authstore.User, error)
	DisableTOTP(context.Context, int64, int64) (authstore.User, error)
}

type PasswordStore interface {
	ChangePassword(context.Context, int64, int64, string, string) (authstore.User, error)
}

type NavigationStore interface {
	Menu(context.Context, authstore.User) ([]authstore.NavigationCategory, error)
}

type DashboardProvider interface {
	Snapshot(context.Context) (webdashboard.Snapshot, error)
	Resources(context.Context) ([]webdashboard.Card, error)
	Supervision(context.Context) ([]webdashboard.Card, error)
	Processes(context.Context) (webdashboard.ProcessSnapshot, error)
}

type ModuleAuthorizer interface {
	PermissionForModule(context.Context, authstore.User, string) (string, bool, error)
}

type StorageProvider interface {
	StorageSnapshot(context.Context) (webstorage.Snapshot, error)
}

type ServicesProvider interface {
	ServicesSnapshot(context.Context) (webservices.Snapshot, error)
	RestartService(context.Context, string) error
}

type NetworkProvider interface {
	NetworkSnapshot(context.Context, string) (webnetwork.Snapshot, error)
}

type LogsProvider interface {
	LogSources(context.Context) ([]string, error)
	LogsSnapshot(context.Context, string, int) (weblogs.Snapshot, error)
}
type PHPProvider interface {
	PHPSnapshot(context.Context) (webphp.Snapshot, error)
	RestartPHP(context.Context, string) error
}
type MySQLProvider interface {
	MySQLSnapshot(context.Context) (webmysql.Snapshot, error)
	MySQLMetrics(context.Context) (map[string]int64, error)
	RestartMySQL(context.Context) error
}
type TorProvider interface {
	TorSnapshot(context.Context) (webtor.Snapshot, error)
	TorAction(context.Context, string) error
}
type ApacheProvider interface {
	ApacheSnapshot(context.Context) (webapache.Snapshot, error)
	ApacheAction(context.Context, string) error
	ApacheSite(context.Context, string) (webapache.SiteConfig, error)
	ApacheSiteAction(context.Context, string, string, string, string) error
}
type Fail2banProvider interface {
	Fail2banSnapshot(context.Context, string) (webfail2ban.Snapshot, error)
	Action(context.Context, string, string, string) error
}
type FirewallProvider interface {
	FirewallSnapshot(context.Context) (webfirewall.Snapshot, error)
	Reload(context.Context) error
	SetEnabled(context.Context, bool) error
	Add(context.Context, string, string, string, string) error
	Delete(context.Context, int) error
}
type CronProvider interface {
	CronSnapshot(context.Context) (webcron.Snapshot, error)
	CronAction(context.Context, string, string, string, string, string) (string, error)
	CronResult(context.Context, string) (webcron.ExecutionResult, error)
	CronBackupCreate(context.Context, webcron.BackupRequest) error
	CronBackupAction(context.Context, string, string) (string, error)
}
type CertbotProvider interface {
	Snapshot(context.Context) (webcertbot.Snapshot, error)
	Start(context.Context, string, []string) (string, error)
	Result(context.Context, string) (webcertbot.ActionResult, error)
}
type UpdatesProvider interface {
	Snapshot(context.Context) (webupdates.Snapshot, error)
	Summary(context.Context) (webupdates.Summary, error)
	StartUpgrade(context.Context) (string, error)
	Job(context.Context, string) (webupdates.Job, error)
	RefreshComposer(context.Context) error
	Reboot(context.Context, int) error
}
type UserAdministrationStore interface {
	AdminUsers(context.Context) ([]authstore.AdminUser, error)
	AssignableModules(context.Context) ([]authstore.AssignableModule, error)
	ModulePermissions(context.Context, int64) (map[int64]string, error)
	CreateAdminUser(context.Context, authstore.AdminUser, string, map[int64]string) (int64, error)
	UpdateAdminUser(context.Context, authstore.AdminUser, map[int64]string, bool) error
	ResetAdminPassword(context.Context, int64, string) error
	DeleteAdminUser(context.Context, int64) error
}
type NavigationAdministrationStore interface {
	AdminNavigation(context.Context) ([]authstore.AdminCategory, error)
	CreateCategory(context.Context, string) error
	RenameCategory(context.Context, int64, string) error
	DeleteCategory(context.Context, int64) error
	ToggleModule(context.Context, int64, bool) error
	MoveCategory(context.Context, int64, int) error
	MoveModule(context.Context, int64, int) error
	ReorderNavigation(context.Context, []int64, map[int64][]int64) error
}
type SettingsStore interface {
	Settings(context.Context) (authstore.ApplicationSettings, error)
	UpdateSettings(context.Context, authstore.ApplicationSettings) error
	SMTPSettings(context.Context) (authstore.SMTPSettings, error)
	UpdateSMTPSettings(context.Context, authstore.SMTPSettings, *string) error
	BackupDatabase(context.Context) ([]byte, error)
	RestoreDatabase(context.Context, []byte) error
}
type AdminAccessProvider interface {
	AdminAccess(context.Context) (websettings.AdminAccess, error)
	UpdateAdminAccess(context.Context, websettings.AdminAccess) (websettings.AdminAccess, error)
}
type ConfigurationProvider interface {
	Call(context.Context, string, []string) (map[string]any, error)
}

type Dependencies struct {
	DefaultLanguage  string
	Readiness        ReadinessChecks
	Sessions         *websession.Manager
	Authenticator    *webauth.Authenticator
	Users            UserLookup
	AccessLog        AccessLogStore
	LoginLimiter     *webauth.LoginLimiter
	TwoFactor        TwoFactorVerifier
	TwoFactorLimiter *webauth.LoginLimiter
	Enrollment       TwoFactorEnrollmentStore
	Passwords        PasswordStore
	PasswordLimiter  *webauth.LoginLimiter
	Navigation       NavigationStore
	Dashboard        DashboardProvider
	Authorization    ModuleAuthorizer
	Storage          StorageProvider
	Services         ServicesProvider
	Network          NetworkProvider
	Logs             LogsProvider
	PHP              PHPProvider
	MySQL            MySQLProvider
	Tor              TorProvider
	Apache           ApacheProvider
	Fail2ban         Fail2banProvider
	Firewall         FirewallProvider
	Cron             CronProvider
	Certbot          CertbotProvider
	Updates          UpdatesProvider
	UserAdmin        UserAdministrationStore
	NavigationAdmin  NavigationAdministrationStore
	Settings         SettingsStore
	AdminAccess      AdminAccessProvider
	Configuration    ConfigurationProvider
}

type application struct {
	loginPage                 string
	statePage                 string
	twoFactorPage             string
	twoFactorSetupPage        string
	passwordPage              string
	accountPasswordPage       string
	dashboardPage             string
	processesPage             string
	storagePage               string
	servicesPage              string
	networkPage               string
	logsPage                  string
	aboutPage                 string
	phpPage                   string
	mysqlPage                 string
	torPage                   string
	apachePage                string
	fail2banPage              string
	firewallPage              string
	cronPage                  string
	certbotPage               string
	updatesPage               string
	rebootWaitPage            string
	usersPage                 string
	userCreatePage            string
	userEditPage              string
	userAccessLogPage         string
	modulesPage               string
	settingsPage              string
	configurationPage         string
	configurationSnapshotPage string
	configurationComparePage  string
	dependencies              Dependencies
}

func Handler(dependencies Dependencies) http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	app := newApplication(assets, dependencies)
	stylesheet, err := fs.ReadFile(assets, "app.css")
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /assets/app.css", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/css; charset=utf-8")
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = response.Write(stylesheet)
	})
	javascript, err := fs.ReadFile(assets, "app.js")
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /assets/app.js", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = response.Write(javascript)
	})
	mux.HandleFunc("GET /assets/aegisadmin-mark.png", projectAsset(
		"image/png",
		"/usr/share/aegisadmin/web/public/assets/images/branding/aegisadmin-mark-512.png",
		"/var/www/Aegisadmin/public/assets/images/branding/aegisadmin-mark-512.png",
	))
	mux.HandleFunc("GET /assets/themes.css", projectAsset(
		"text/css; charset=utf-8",
		"/usr/share/aegisadmin/web/public/assets/css/themes.css",
		"/var/www/Aegisadmin/public/assets/css/themes.css",
	))
	mux.HandleFunc("GET /assets/auth-background.webp", projectAsset(
		"image/webp",
		"/usr/share/aegisadmin/web/public/assets/images/branding/auth-background.webp",
		"/var/www/Aegisadmin/public/assets/images/branding/auth-background.webp",
	))
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", ready(dependencies.Readiness))
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("GET /login", app.login)
	mux.HandleFunc("POST /login", app.authenticate)
	mux.HandleFunc("GET /go/dashboard", app.dashboard)
	mux.HandleFunc("GET /system", func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/go/dashboard", http.StatusPermanentRedirect)
	})
	mux.HandleFunc("GET /go/dashboard/resources", app.dashboardResources)
	mux.HandleFunc("GET /go/dashboard/supervision", app.dashboardSupervision)
	mux.HandleFunc("GET /updates/summary", app.updatesSummary)
	mux.HandleFunc("GET /processes", app.processes)
	mux.HandleFunc("GET /go/navigation", app.navigationMenu)
	mux.HandleFunc("GET /storage", app.storage)
	mux.HandleFunc("GET /services", app.services)
	mux.HandleFunc("POST /services/restart", app.restartService)
	mux.HandleFunc("GET /network", app.network)
	mux.HandleFunc("GET /logs", app.logs)
	mux.HandleFunc("GET /about", app.about)
	mux.HandleFunc("GET /php", app.php)
	mux.HandleFunc("POST /php/restart", app.restartPHP)
	mux.HandleFunc("GET /mysql", app.mysql)
	mux.HandleFunc("GET /mysql/metrics", app.mysqlMetrics)
	mux.HandleFunc("POST /mysql/restart", app.restartMySQL)
	mux.HandleFunc("GET /tor", app.tor)
	mux.HandleFunc("POST /tor/{action}", app.torAction)
	mux.HandleFunc("GET /apache", app.apache)
	mux.HandleFunc("GET /apache/sites/{id}", app.apacheSite)
	mux.HandleFunc("POST /apache/{action}", app.apacheAction)
	mux.HandleFunc("GET /fail2ban", app.fail2ban)
	mux.HandleFunc("POST /fail2ban/{action}", app.fail2banAction)
	mux.HandleFunc("GET /firewall", app.firewall)
	mux.HandleFunc("POST /firewall/{action}", app.firewallAction)
	mux.HandleFunc("GET /cron", app.cron)
	mux.HandleFunc("POST /cron/{action}", app.cronAction)
	mux.HandleFunc("POST /cron/backup/create", app.cronBackupCreate)
	mux.HandleFunc("POST /cron/backup/{action}", app.cronBackupAction)
	mux.HandleFunc("GET /cron/executions/{id}", app.cronResult)
	mux.HandleFunc("GET /certbot", app.certbot)
	mux.HandleFunc("GET /certbot/", redirectCanonical("/certbot"))
	mux.HandleFunc("POST /certbot/{action}", app.certbotAction)
	mux.HandleFunc("GET /certbot/actions/{id}", app.certbotResult)
	mux.HandleFunc("GET /updates", app.updates)
	mux.HandleFunc("GET /updates/", redirectCanonical("/updates"))
	mux.HandleFunc("POST /updates/start", app.startUpdates)
	mux.HandleFunc("POST /updates/reboot", app.rebootServer)
	mux.HandleFunc("GET /updates/jobs/{id}", app.updatesJob)
	mux.HandleFunc("POST /updates/composer-refresh", app.refreshComposer)
	mux.HandleFunc("GET /users", app.users)
	mux.HandleFunc("GET /users/access-log", app.userAccessLog)
	mux.HandleFunc("GET /users/create", app.newUser)
	mux.HandleFunc("GET /users/{id}/edit", app.editUser)
	mux.HandleFunc("POST /users/create", app.createUser)
	mux.HandleFunc("POST /users/{id}/update", app.updateUser)
	mux.HandleFunc("POST /users/{id}/password", app.resetUserPassword)
	mux.HandleFunc("POST /users/{id}/delete", app.deleteUser)
	mux.HandleFunc("GET /modules", app.modules)
	mux.HandleFunc("POST /modules/categories/create", app.createCategory)
	mux.HandleFunc("POST /modules/categories/{id}/rename", app.renameCategory)
	mux.HandleFunc("POST /modules/categories/{id}/delete", app.deleteCategory)
	mux.HandleFunc("POST /modules/categories/{id}/move", app.moveCategory)
	mux.HandleFunc("POST /modules/{id}/toggle", app.toggleModule)
	mux.HandleFunc("POST /modules/{id}/move", app.moveModule)
	mux.HandleFunc("POST /modules/reorder", app.reorderNavigation)
	mux.HandleFunc("GET /setting", app.settings)
	mux.HandleFunc("POST /setting", app.updateSettings)
	mux.HandleFunc("POST /setting/smtp", app.updateSMTPSettings)
	mux.HandleFunc("POST /setting/admin-access", app.updateAdminAccess)
	mux.HandleFunc("POST /setting/database/backup", app.backupDatabase)
	mux.HandleFunc("POST /setting/database/restore", app.restoreDatabase)
	mux.HandleFunc("GET /configuration", app.configuration)
	mux.HandleFunc("POST /configuration/snapshots", app.createConfigurationSnapshot)
	mux.HandleFunc("POST /configuration/snapshots/import", app.importConfigurationSnapshot)
	mux.HandleFunc("GET /configuration/compare", app.compareConfigurationSnapshots)
	mux.HandleFunc("POST /configuration/snapshots/{id}/name", app.renameConfigurationSnapshot)
	mux.HandleFunc("POST /configuration/snapshots/{id}/delete", app.deleteConfigurationSnapshot)
	mux.HandleFunc("GET /configuration/snapshots/{id}", app.configurationSnapshot)
	mux.HandleFunc("GET /configuration/snapshots/{id}/export", app.exportConfigurationSnapshot)
	mux.HandleFunc("POST /logout", app.logout)
	mux.HandleFunc("GET /go/account/password", app.password)
	mux.HandleFunc("POST /go/account/password", app.changePassword)
	mux.HandleFunc("POST /go/account/theme", app.changeTheme)
	mux.HandleFunc("POST /go/account/language", app.changeLanguage)
	mux.HandleFunc("GET /go/account/two-factor/setup", app.accountTwoFactorSetup)
	mux.HandleFunc("POST /go/account/two-factor/setup", app.confirmAccountTwoFactorSetup)
	mux.HandleFunc("POST /go/account/two-factor/disable", app.disableAccountTwoFactor)
	mux.HandleFunc("GET /go/two-factor", app.twoFactor)
	mux.HandleFunc("POST /go/two-factor", app.verifyTwoFactor)
	mux.HandleFunc("GET /go/two-factor/setup", app.twoFactorSetup)
	mux.HandleFunc("POST /go/two-factor/setup", app.confirmTwoFactorSetup)
	return securityHeaders(mux)
}

func newApplication(assets fs.FS, dependencies Dependencies) *application {
	read := func(name string) string {
		content, err := fs.ReadFile(assets, name)
		if err != nil {
			panic(err)
		}
		return strings.ReplaceAll(string(content), "{{VERSION}}", html.EscapeString(buildinfo.Version))
	}
	return &application{
		loginPage: read("login.html"), statePage: read("state.html"),
		twoFactorPage: read("two-factor.html"), twoFactorSetupPage: read("two-factor-setup.html"),
		passwordPage:              read("password.html"),
		accountPasswordPage:       read("account-password.html"),
		dashboardPage:             read("dashboard.html"),
		processesPage:             read("processes.html"),
		storagePage:               read("storage.html"),
		servicesPage:              read("services.html"),
		networkPage:               read("network.html"),
		logsPage:                  read("logs.html"),
		aboutPage:                 read("about.html"),
		phpPage:                   read("php.html"),
		mysqlPage:                 read("mysql.html"),
		torPage:                   read("tor.html"),
		apachePage:                read("apache.html"),
		fail2banPage:              read("fail2ban.html"),
		firewallPage:              read("firewall.html"),
		cronPage:                  read("cron.html"),
		certbotPage:               read("certbot.html"),
		updatesPage:               read("updates.html"),
		rebootWaitPage:            read("reboot-wait.html"),
		usersPage:                 read("users.html"),
		userCreatePage:            read("user-create.html"),
		userEditPage:              read("user-edit.html"),
		userAccessLogPage:         read("user-access-log.html"),
		modulesPage:               read("modules.html"),
		settingsPage:              read("settings.html"),
		configurationPage:         read("configuration.html"),
		configurationSnapshotPage: read("configuration-snapshot.html"),
		configurationComparePage:  read("configuration-compare.html"),
		dependencies:              dependencies,
	}
}

func (a *application) home(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(response, request)
		return
	}
	http.Redirect(response, request, "/login", http.StatusSeeOther)
}

func (a *application) login(response http.ResponseWriter, request *http.Request) {
	if a.dependencies.Sessions == nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	session, found := a.requestSession(request)
	if found && session.State == websession.StateAuthenticated {
		http.Redirect(response, request, "/go/dashboard", http.StatusSeeOther)
		return
	}
	if !found {
		var err error
		session, err = a.dependencies.Sessions.Create(websession.StateAnonymous, 0, 0)
		if err != nil {
			http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
			return
		}
		http.SetCookie(response, websession.Cookie(session.ID))
	}
	a.renderLogin(request, response, session.CSRFToken, false, http.StatusOK)
}

func (a *application) authenticate(response http.ResponseWriter, request *http.Request) {
	if !a.available() {
		http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
		return
	}
	session, found := a.requestSession(request)
	if !found || session.State != websession.StateAnonymous {
		http.SetCookie(response, websession.ExpiredCookie())
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		a.renderLogin(request, response, session.CSRFToken, true, http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderLogin(request, response, session.CSRFToken, true, http.StatusBadRequest)
		return
	}
	login, password := request.PostForm.Get("login"), request.PostForm.Get("password")
	address := requestAddress(request)
	if len(login) < 3 || len(login) > 64 || password == "" || len(password) > 1024 ||
		!a.dependencies.LoginLimiter.Allowed(address, login) {
		a.dependencies.LoginLimiter.Failure(address, login)
		a.recordAccess(request, nil, login, "login_failure", false)
		a.renderLogin(request, response, session.CSRFToken, true, http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	result, err := a.dependencies.Authenticator.Authenticate(ctx, login, password)
	if err != nil {
		http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
		return
	}
	if result.Step == webauth.StepInvalid {
		a.dependencies.LoginLimiter.Failure(address, login)
		a.recordAccess(request, nil, login, "login_failure", false)
		a.renderLogin(request, response, session.CSRFToken, true, http.StatusUnauthorized)
		return
	}
	a.dependencies.LoginLimiter.Success(login)
	a.recordAccess(request, &result.User, result.User.Login, "login_success", true)
	state, location := nextState(result.Step)
	rotated, err := a.dependencies.Sessions.Rotate(session.ID, state, result.User.ID, result.User.AuthVersion)
	if err != nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.Cookie(rotated.ID))
	if theme, themeErr := a.dependencies.Users.ThemeForUser(request.Context(), result.User.ID); themeErr == nil {
		http.SetCookie(response, themeCookie(theme))
	}
	if language, languageErr := a.dependencies.Users.LanguageForUser(request.Context(), result.User.ID); languageErr == nil {
		if !i18n.Supported(language) {
			language = a.defaultLanguage(request.Context())
			_ = a.dependencies.Users.SetLanguage(request.Context(), result.User.ID, language)
		}
		http.SetCookie(response, languageCookie(language))
	}
	http.Redirect(response, request, location, http.StatusSeeOther)
}

func (a *application) changeTheme(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	theme := request.PostForm.Get("theme")
	if theme != "dark" && theme != "light" && theme != "bootstrap" && theme != "neon" {
		http.Error(response, "Le thème demandé est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	if err := a.dependencies.Users.SetTheme(ctx, user.ID, theme); err != nil {
		http.Error(response, "Le thème n’a pas pu être enregistré.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, themeCookie(theme))
	response.WriteHeader(http.StatusNoContent)
}

func themeCookie(theme string) *http.Cookie {
	return &http.Cookie{Name: "aegisadmin_theme", Value: theme, Path: "/", MaxAge: 31536000, Secure: true, HttpOnly: false, SameSite: http.SameSiteStrictMode}
}

func (a *application) changeLanguage(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	language := request.PostForm.Get("language")
	if !i18n.Supported(language) {
		http.Error(response, "La langue demandée est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	if err := a.dependencies.Users.SetLanguage(ctx, user.ID, language); err != nil {
		http.Error(response, "La langue n’a pas pu être enregistrée.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, languageCookie(language))
	http.Redirect(response, request, "/go/account/password?result=language", http.StatusSeeOther)
}

func languageCookie(language string) *http.Cookie {
	return &http.Cookie{Name: "aegisadmin_language", Value: language, Path: "/", MaxAge: 31536000, Secure: true, HttpOnly: false, SameSite: http.SameSiteStrictMode}
}

func (a *application) dashboard(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Dashboard == nil {
		http.Error(response, "Service du tableau de bord indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.Dashboard.Snapshot(ctx)
	if err != nil {
		http.Error(response, "Les indicateurs du serveur n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(request.Context(), user.ID)
	page := strings.NewReplacer(
		"{{USER}}", html.EscapeString(user.Login),
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{SYSTEM_INFORMATION}}", renderSystemInformation(snapshot.Information, snapshot.UptimeSeconds, language),
		"{{RESOURCES}}", renderOverview(localizeDashboardCards(filterDashboardCards(snapshot.Cards, false), language)),
		"{{SUPERVISION}}", renderOverview(localizeDashboardCards(filterDashboardCards(snapshot.Cards, true), language)),
	).Replace(i18n.Localize(a.dashboardPage, language))
	writeHTML(response, page, http.StatusOK)
}

func (a *application) dashboardResources(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Dashboard == nil {
		http.Error(response, "Service du tableau de bord indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	cards, err := a.dependencies.Dashboard.Resources(ctx)
	if err != nil {
		http.Error(response, "Les ressources système n’ont pas pu être actualisées.", http.StatusServiceUnavailable)
		return
	}
	writeJSON(response, localizeDashboardCards(cards, a.languageForUser(ctx, user.ID)), http.StatusOK)
}

func (a *application) dashboardSupervision(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Dashboard == nil {
		http.Error(response, "Service du tableau de bord indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	cards, err := a.dependencies.Dashboard.Supervision(ctx)
	if err != nil {
		http.Error(response, "La supervision n’a pas pu être actualisée.", http.StatusServiceUnavailable)
		return
	}
	writeJSON(response, localizeDashboardCards(cards, a.languageForUser(ctx, user.ID)), http.StatusOK)
}

func (a *application) updatesSummary(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Updates == nil {
		http.Error(response, "Service des mises à jour indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	summary, err := a.dependencies.Updates.Summary(ctx)
	language := a.languageForUser(ctx, user.ID)
	if err != nil {
		failure := webupdates.Summary{Success: false, Message: "L’état des mises à jour est indisponible.", Status: "neutral", StatusLabel: "Indisponible", Value: "État indisponible", Subtitle: "La vérification n’a pas pu être effectuée.", URL: "/updates"}
		writeJSON(response, localizeUpdatesSummary(failure, language), http.StatusServiceUnavailable)
		return
	}
	writeJSON(response, localizeUpdatesSummary(summary, language), http.StatusOK)
}

func localizeUpdatesSummary(summary webupdates.Summary, language string) webupdates.Summary {
	if language != "en" {
		return summary
	}
	labels := map[string]string{"Sécurité": "Security", "Mises à jour disponibles": "Updates available", "Redémarrage requis": "Reboot required", "État partiel": "Partial status", "À jour": "Up to date", "Indisponible": "Unavailable"}
	if value := labels[summary.StatusLabel]; value != "" {
		summary.StatusLabel = value
	}
	if summary.UpdatesAvailable {
		word := "updates"
		if summary.UpdateCount == 1 {
			word = "update"
		}
		summary.Value = fmt.Sprintf("%d %s", summary.UpdateCount, word)
		summary.Subtitle = fmt.Sprintf("%d packages · %d firmware · %d security", summary.APTUpdateCount, summary.FirmwareUpdateCount, summary.SecurityUpdateCount)
	} else if summary.RebootRequired {
		summary.Value, summary.Subtitle = "Reboot required", "No new update available."
	} else if !summary.Complete {
		summary.Value, summary.Subtitle = "Packages up to date", "Firmware monitoring unavailable."
	} else {
		summary.Value, summary.Subtitle = "System up to date", "No package or firmware update."
	}
	if !summary.Success {
		summary.Message, summary.Value, summary.Subtitle = "Update status is unavailable.", "Status unavailable", "The check could not be completed."
	}
	return summary
}

func (a *application) processes(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	_, granted, ok := a.modulePermission(response, request, user, "processes")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Dashboard == nil {
		http.Error(response, "Service des processus indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 8*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.Dashboard.Processes(ctx)
	if err != nil {
		http.Error(response, "Les processus n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	rows := renderProcesses(snapshot.Processes, language)
	notice := ""
	if snapshot.Truncated {
		notice = `<p class="notice">` + html.EscapeString(fmt.Sprintf(i18n.Text(language, "processes.truncated"), strconv.FormatInt(snapshot.Returned, 10), strconv.FormatInt(snapshot.Total, 10))) + `</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{TOTAL}}", strconv.FormatInt(snapshot.Total, 10), "{{RETURNED}}", strconv.FormatInt(snapshot.Returned, 10), "{{LIMIT}}", strconv.FormatInt(snapshot.Limit, 10), "{{NOTICE}}", notice, "{{PROCESSES}}", rows).Replace(i18n.Localize(a.processesPage, language))
	writeHTML(response, page, http.StatusOK)
}

func renderProcesses(processes []webdashboard.Process, language string) string {
	if len(processes) == 0 {
		return `<tr><td colspan="8" class="muted">` + html.EscapeString(i18n.Text(language, "processes.empty")) + `</td></tr>`
	}
	var result strings.Builder
	for _, process := range processes {
		status := process.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		name := html.EscapeString(process.Name)
		if description := process.Description; description != "" {
			label := html.EscapeString(process.Name + " : " + description)
			name = `<span class="process-name process-name--described" tabindex="0" title="` + html.EscapeString(description) + `" aria-label="` + label + `">` + name + `<span class="process-name__help" aria-hidden="true">?</span></span>`
		}
		stateLabel := processStateLabel(process.State, process.StateLabel, language)
		result.WriteString(`<tr><td data-sort-value="` + strconv.FormatInt(process.PIDValue, 10) + `">` + html.EscapeString(process.PID) + `</td><th data-sort-value="` + html.EscapeString(strings.ToLower(process.Name)) + `">` + name + `</th><td data-sort-value="` + html.EscapeString(strings.ToLower(process.User)) + `">` + html.EscapeString(process.User) + `</td><td data-sort-value="` + html.EscapeString(strings.ToLower(stateLabel+" "+process.State)) + `"><span class="status-badge status-badge--` + status + `">` + html.EscapeString(stateLabel+" · "+process.State) + `</span></td><td data-sort-value="` + strconv.FormatFloat(process.CPUPercentValue, 'f', -1, 64) + `">` + html.EscapeString(process.CPU) + `</td><td data-sort-value="` + strconv.FormatFloat(process.MemoryPercentValue, 'f', -1, 64) + `">` + html.EscapeString(process.MemoryPercent) + `</td><td data-sort-value="` + strconv.FormatInt(process.MemoryBytes, 10) + `">` + html.EscapeString(process.Memory) + `</td><td data-sort-value="` + strconv.FormatInt(process.ElapsedSeconds, 10) + `">` + html.EscapeString(process.Elapsed) + `</td></tr>`)
	}
	return result.String()
}

func processStateLabel(state, fallback, language string) string {
	if state == "" {
		return i18n.Text(language, "processes.state.unknown")
	}
	keys := map[byte]string{
		'R': "running", 'S': "sleeping", 'D': "disk_wait", 'T': "stopped", 't': "stopped",
		'Z': "zombie", 'X': "dead", 'x': "dead", 'K': "wakekill", 'I': "idle",
		'P': "parked", 'W': "paging",
	}
	if key := keys[state[0]]; key != "" {
		return i18n.Text(language, "processes.state."+key)
	}
	if fallback != "" && language != "en" {
		return fallback
	}
	return i18n.Text(language, "processes.state.unknown")
}

func renderSystemInformation(information []webdashboard.Information, uptimeSeconds int64, language string) string {
	if len(information) == 0 {
		return `<p class="muted">` + html.EscapeString(i18n.Text(language, "dashboard.unavailable")) + `</p>`
	}
	var result strings.Builder
	result.WriteString(`<dl class="detail-grid dashboard-information">`)
	for _, item := range information {
		attributes := ""
		if item.Label == "Durée de fonctionnement" {
			attributes = ` data-dashboard-uptime data-dashboard-uptime-seconds="` + strconv.FormatInt(uptimeSeconds, 10) + `"`
		}
		labels := map[string]string{"Nom d’hôte": "dashboard.host", "Système d’exploitation": "dashboard.os", "Distribution": "dashboard.distribution", "Version": "dashboard.version", "Noyau": "dashboard.kernel", "Architecture": "dashboard.architecture", "Durée de fonctionnement": "dashboard.uptime"}
		label := item.Label
		if key := labels[item.Label]; key != "" {
			label = i18n.Text(language, key)
		}
		value := item.Value
		if item.Label == "Durée de fonctionnement" {
			value = formatDashboardUptime(uptimeSeconds, language)
		}
		result.WriteString(`<div><dt>` + html.EscapeString(label) + `</dt><dd` + attributes + `>` + html.EscapeString(value) + `</dd></div>`)
	}
	result.WriteString(`<div class="dashboard-information__updates" data-dashboard-updates data-dashboard-updates-url="/updates/summary" aria-live="polite" aria-busy="true"><dt>` + html.EscapeString(i18n.Text(language, "dashboard.updates")) + `</dt><dd><span data-dashboard-updates-value>` + html.EscapeString(i18n.Text(language, "dashboard.checking")) + `</span><a href="/updates" data-dashboard-updates-link hidden>` + html.EscapeString(i18n.Text(language, "dashboard.view_updates")) + `</a></dd></div>`)
	result.WriteString(`</dl>`)
	return result.String()
}

func filterDashboardCards(cards []webdashboard.Card, supervision bool) []webdashboard.Card {
	result := make([]webdashboard.Card, 0, len(cards))
	for _, card := range cards {
		isSupervision := card.Title == "Stockage" || card.Title == "Services" || card.Title == "Réseau"
		if isSupervision == supervision {
			result = append(result, card)
		}
	}
	return result
}

func localizeDashboardCards(cards []webdashboard.Card, language string) []webdashboard.Card {
	localized := append([]webdashboard.Card(nil), cards...)
	if language != "en" {
		return localized
	}
	titles := map[string]string{"cpu": "dashboard.cpu", "memory": "dashboard.memory", "temperature": "dashboard.temperature", "processes": "dashboard.processes", "storage": "dashboard.storage", "services": "dashboard.services", "network": "dashboard.network"}
	for index := range localized {
		card := &localized[index]
		if key := titles[card.ID]; key != "" {
			card.Title = i18n.Text(language, key)
		}
		card.Value = strings.NewReplacer("Indisponible", "Unavailable", " actifs", " active", " actives", " up", " volume(s)", " volumes", ",", ".").Replace(card.Value)
		card.Subtitle = strings.NewReplacer("Lecture impossible", "Unable to read", "Processus actuellement détectés", "Processes currently detected", "Aucun capteur CPU disponible", "No CPU sensor available", "Capteur :", "Sensor:", " disponibles", " available", " utilisés sur ", " used out of ", "Occupation maximale :", "Maximum usage:", " service(s) surveillé(s)", " monitored services", " interface(s) arrêtée(s)", " interfaces down", "cœur(s)", "cores", "Charge :", "Load:", " à ", " at ", " o", " B", " Kio", " KiB", " Mio", " MiB", " Gio", " GiB", " Tio", " TiB", ",", ".").Replace(card.Subtitle)
	}
	return localized
}

func formatDashboardUptime(seconds int64, language string) string {
	if seconds < 0 {
		seconds = 0
	}
	days, hours, minutes := seconds/86400, seconds%86400/3600, seconds%3600/60
	if language == "en" {
		if days > 0 {
			return fmt.Sprintf("%d d %d h %d min", days, hours, minutes)
		}
		return fmt.Sprintf("%d h %d min", hours, minutes)
	}
	if days > 0 {
		return fmt.Sprintf("%d j %d h %d min", days, hours, minutes)
	}
	return fmt.Sprintf("%d h %d min", hours, minutes)
}

func (a *application) navigationMenu(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Navigation == nil {
		http.Error(response, "Navigation indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	categories, err := a.dependencies.Navigation.Menu(ctx, user)
	if err != nil {
		http.Error(response, "Navigation indisponible.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	type menuModule struct{ Name, Icon, Route string }
	type menuCategory struct {
		Name    string
		Modules []menuModule
	}
	result := make([]menuCategory, 0, len(categories))
	for _, category := range categories {
		categoryName := category.Name
		if language == "en" {
			categoryName = map[string]string{"Vue générale": "Overview", "Supervision": "Monitoring", "Services": "Services", "Sécurité": "Security", "Administration": "Administration"}[category.Name]
			if categoryName == "" {
				categoryName = category.Name
			}
		}
		item := menuCategory{Name: categoryName, Modules: []menuModule{}}
		for _, module := range category.Modules {
			route := goModuleRoute(module.Key)
			if route != "" {
				item.Modules = append(item.Modules, menuModule{Name: module.Name, Icon: module.Icon, Route: route})
			}
		}
		if len(item.Modules) > 0 {
			result = append(result, item)
		}
	}
	writeJSON(response, result, http.StatusOK)
}

func goModuleRoute(key string) string {
	routes := map[string]string{"dashboard": "/go/dashboard", "storage": "/storage", "services": "/services", "network": "/network", "processes": "/processes", "logs": "/logs", "about": "/about", "php": "/php", "mysql": "/mysql", "tor": "/tor", "apache": "/apache", "fail2ban": "/fail2ban", "firewall": "/firewall", "cron": "/cron", "certbot": "/certbot", "updates": "/updates", "users": "/users", "modules": "/modules", "setting": "/setting", "configuration": "/configuration"}
	return routes[key]
}

func renderOverview(cards []webdashboard.Card) string {
	var result strings.Builder
	for _, card := range cards {
		status := card.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		tag, end := "article", "article"
		if card.URL != "" {
			tag, end = `a href="`+html.EscapeString(card.URL)+`"`, "a"
		}
		result.WriteString(`<` + tag + ` class="overview-card overview-card--`)
		result.WriteString(status)
		result.WriteString(`" data-dashboard-card="` + html.EscapeString(card.ID) + `"><h3>`)
		result.WriteString(html.EscapeString(card.Title))
		result.WriteString(`</h3><p class="overview-card__value">`)
		result.WriteString(html.EscapeString(card.Value))
		result.WriteString(`</p><p class="muted">`)
		result.WriteString(html.EscapeString(card.Subtitle))
		result.WriteString(`</p></` + end + `>`)
	}
	return result.String()
}

func (a *application) storage(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Authorization == nil || a.dependencies.Storage == nil {
		http.Error(response, "Service de stockage indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	level, granted, err := a.dependencies.Authorization.PermissionForModule(ctx, user, "storage")
	if err != nil {
		http.Error(response, "Les droits d’accès n’ont pas pu être vérifiés.", http.StatusServiceUnavailable)
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	snapshot, err := a.dependencies.Storage.StorageSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations de stockage n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)),
		"{{SUMMARY}}", renderStorageSummary(snapshot.Summary, language),
		"{{MOUNTS}}", renderStorageMounts(snapshot.Mounts, language),
	).Replace(i18n.Localize(a.storagePage, language))
	writeHTML(response, page, http.StatusOK)
}

func renderStorageSummary(summary webstorage.Summary, language string) string {
	items := []struct {
		label string
		value int
	}{
		{i18n.Text(language, "storage.volumes"), summary.Total}, {i18n.Text(language, "storage.normal"), summary.Normal},
		{i18n.Text(language, "storage.warning"), summary.Warning}, {i18n.Text(language, "storage.critical"), summary.Danger},
	}
	var result strings.Builder
	for _, item := range items {
		result.WriteString(`<article class="summary-item"><span>`)
		result.WriteString(item.label)
		result.WriteString(`</span><strong>`)
		result.WriteString(strconv.Itoa(item.value))
		result.WriteString(`</strong></article>`)
	}
	return result.String()
}

func renderStorageMounts(mounts []webstorage.Mount, language string) string {
	if len(mounts) == 0 {
		return `<tr><td colspan="9" class="muted">` + html.EscapeString(i18n.Text(language, "storage.empty")) + `</td></tr>`
	}
	var result strings.Builder
	for _, mount := range mounts {
		status := mount.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		physicalDevice := mount.PhysicalDevice
		if physicalDevice == "" {
			physicalDevice = i18n.Text(language, "storage.undetermined")
		}
		statusLabel := mount.StatusLabel
		if language == "en" {
			switch status {
			case "success":
				statusLabel = "Healthy"
			case "warning":
				statusLabel = "Warning"
			case "danger":
				statusLabel = "Critical"
			default:
				statusLabel = "Unknown"
			}
		}
		result.WriteString(`<tr>`)
		cells := []struct {
			value string
			sort  string
		}{
			{physicalDevice, strings.ToLower(physicalDevice)},
			{mount.Mount, strings.ToLower(mount.Mount)},
			{mount.Filesystem, strings.ToLower(mount.Filesystem)},
			{mount.SizeLabel, strconv.FormatInt(mount.Size, 10)},
			{mount.UsedLabel, strconv.FormatInt(mount.Used, 10)},
			{mount.AvailableLabel, strconv.FormatInt(mount.Available, 10)},
		}
		for _, cell := range cells {
			result.WriteString(`<td data-sort-value="`)
			result.WriteString(html.EscapeString(cell.sort))
			result.WriteString(`">`)
			result.WriteString(html.EscapeString(cell.value))
			result.WriteString(`</td>`)
		}
		result.WriteString(`<td data-sort-value="`)
		result.WriteString(strconv.Itoa(mount.Percent))
		result.WriteString(`">`)
		result.WriteString(strconv.Itoa(mount.Percent))
		result.WriteString(` %</td><td data-sort-value="`)
		result.WriteString(html.EscapeString(strings.ToLower(statusLabel)))
		result.WriteString(`"><span class="status-badge status-badge--`)
		result.WriteString(status)
		result.WriteString(`">`)
		result.WriteString(html.EscapeString(statusLabel))
		result.WriteString(`</span></td><td data-sort-value="` + html.EscapeString(strings.ToLower(mount.Source)) + `">` + html.EscapeString(mount.Source) + `</td></tr>`)
	}
	return result.String()
}

func (a *application) services(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "services")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Services == nil {
		http.Error(response, "Service de supervision indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.Services.ServicesSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les services n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	canAct := level == "action" || level == "modify"
	notice := ""
	if request.URL.Query().Get("result") == "restarted" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "services.restarted")) + `</p>`
	}
	actionHeader := ""
	if canAct {
		actionHeader = "<th>" + html.EscapeString(i18n.Text(language, "services.action")) + "</th>"
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)),
		"{{NOTICE}}", notice,
		"{{SUMMARY}}", renderServicesSummary(snapshot.Summary, language),
		"{{ACTION_HEADER}}", actionHeader,
		"{{SERVICES}}", renderServices(snapshot.Services, session.CSRFToken, canAct, language),
	).Replace(i18n.Localize(a.servicesPage, language))
	writeHTML(response, page, http.StatusOK)
}

func (a *application) restartService(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "services")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Services == nil || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	if err := a.dependencies.Services.RestartService(ctx, request.PostForm.Get("service")); err != nil {
		http.Error(response, "Le redémarrage du service a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/services?result=restarted", http.StatusSeeOther)
}

func (a *application) modulePermission(response http.ResponseWriter, request *http.Request, user authstore.User, module string) (string, bool, bool) {
	if a.dependencies.Authorization == nil {
		http.Error(response, "Les droits d’accès ne peuvent pas être vérifiés.", http.StatusServiceUnavailable)
		return "", false, false
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	level, granted, err := a.dependencies.Authorization.PermissionForModule(ctx, user, module)
	if err != nil {
		http.Error(response, "Les droits d’accès n’ont pas pu être vérifiés.", http.StatusServiceUnavailable)
		return "", false, false
	}
	return level, granted, true
}

func renderServicesSummary(summary webservices.Summary, language string) string {
	items := []struct {
		label string
		value int
	}{
		{i18n.Text(language, "services.allowed"), summary.Total}, {i18n.Text(language, "services.installed"), summary.Installed},
		{i18n.Text(language, "services.active"), summary.Active}, {i18n.Text(language, "services.inactive"), summary.Inactive}, {i18n.Text(language, "services.missing"), summary.Missing},
	}
	var result strings.Builder
	for _, item := range items {
		result.WriteString(`<article class="summary-item"><span>`)
		result.WriteString(item.label)
		result.WriteString(`</span><strong>`)
		result.WriteString(strconv.Itoa(item.value))
		result.WriteString(`</strong></article>`)
	}
	return result.String()
}

func renderServices(services []webservices.Service, csrf string, canAct bool, language string) string {
	columns := 5
	if canAct {
		columns++
	}
	if len(services) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">` + html.EscapeString(i18n.Text(language, "services.empty")) + `</td></tr>`
	}
	var result strings.Builder
	for _, service := range services {
		status := service.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		result.WriteString(`<tr>`)
		if canAct {
			result.WriteString(`<td>`)
			if service.Exists {
				result.WriteString(`<form class="inline-form" method="post" action="/services/restart"><input type="hidden" name="_token" value="`)
				result.WriteString(html.EscapeString(csrf))
				result.WriteString(`"><input type="hidden" name="service" value="`)
				result.WriteString(html.EscapeString(service.ID))
				result.WriteString(`"><button class="danger-button" type="submit">` + html.EscapeString(i18n.Text(language, "services.restart")) + `</button></form>`)
			}
			result.WriteString(`</td>`)
		}
		result.WriteString(`<th scope="row">`)
		result.WriteString(html.EscapeString(service.ID))
		result.WriteString(`</th><td><span class="status-badge status-badge--` + status + `">`)
		result.WriteString(html.EscapeString(serviceStatusLabel(service, language)))
		result.WriteString(`</span></td><td>` + yesNoForLanguage(service.Exists, language) + `</td><td>` + yesNoForLanguage(service.Enabled, language) + `</td><td>`)
		result.WriteString(html.EscapeString(service.State))
		result.WriteString(`</td>`)
		result.WriteString(`</tr>`)
	}
	return result.String()
}

func serviceStatusLabel(service webservices.Service, language string) string {
	if !service.Exists {
		return i18n.Text(language, "services.status.missing")
	}
	if service.Active {
		return i18n.Text(language, "services.status.active")
	}
	if service.State == "failed" {
		return i18n.Text(language, "services.status.failed")
	}
	return i18n.Text(language, "services.status.stopped")
}

func yesNoForLanguage(value bool, language string) string {
	if value {
		return i18n.Text(language, "common.yes")
	}
	return i18n.Text(language, "common.no")
}

func yesNo(value bool) string {
	if value {
		return "Oui"
	}
	return "Non"
}

func (a *application) network(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "network")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Network == nil {
		http.Error(response, "Service réseau indisponible.", http.StatusServiceUnavailable)
		return
	}
	requested := request.URL.Query().Get("interface")
	if len(requested) > 64 {
		http.Error(response, "L’interface demandée est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.Network.NetworkSnapshot(ctx, requested)
	if err != nil {
		http.Error(response, "Les informations réseau n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)),
		"{{SUMMARY}}", renderNetworkSummary(snapshot.Summary, language),
		"{{INTERFACES}}", renderNetworkInterfaces(snapshot.Interfaces, snapshot.Selected, language),
		"{{DETAILS}}", renderNetworkDetails(snapshot.Selected, language),
	).Replace(i18n.Localize(a.networkPage, language))
	writeHTML(response, page, http.StatusOK)
}

func renderNetworkSummary(summary webnetwork.Summary, language string) string {
	items := []struct {
		label string
		value int
	}{{i18n.Text(language, "network.detected"), summary.Total}, {i18n.Text(language, "network.active"), summary.Up}, {i18n.Text(language, "network.down"), summary.Down}, {i18n.Text(language, "network.unknown"), summary.Unknown}}
	var result strings.Builder
	for _, item := range items {
		result.WriteString(`<article class="summary-item"><span>` + item.label + `</span><strong>` + strconv.Itoa(item.value) + `</strong></article>`)
	}
	return result.String()
}

func renderNetworkInterfaces(interfaces []webnetwork.Interface, selected *webnetwork.Interface, language string) string {
	if len(interfaces) == 0 {
		return `<tr><td colspan="4" class="muted">` + html.EscapeString(i18n.Text(language, "network.empty")) + `</td></tr>`
	}
	selectedID := ""
	if selected != nil {
		selectedID = selected.ID
	}
	var result strings.Builder
	for _, item := range interfaces {
		status := item.Status
		if status != "success" && status != "warning" {
			status = "neutral"
		}
		result.WriteString(`<tr><td>`)
		if item.ID == selectedID {
			result.WriteString(`<span class="network-selected-button" aria-current="true">` + html.EscapeString(i18n.Text(language, "network.selected")) + `</span>`)
		} else {
			result.WriteString(`<a class="secondary-link" href="/network?interface=` + url.QueryEscape(item.ID) + `">` + html.EscapeString(i18n.Text(language, "network.show")) + `</a>`)
		}
		result.WriteString(`</td><th scope="row">` + html.EscapeString(item.Name) + `</th><td>` + html.EscapeString(networkTypeLabel(item.Type, item.TypeLabel, language)) + `</td><td><span class="status-badge status-badge--` + status + `">` + html.EscapeString(networkStatusLabel(item.State, language)) + `</span></td></tr>`)
	}
	return result.String()
}

func renderNetworkDetails(item *webnetwork.Interface, language string) string {
	if item == nil {
		return ""
	}
	mac, mtu := i18n.Text(language, "network.unavailable"), i18n.Text(language, "network.unavailable")
	if item.MAC != nil {
		mac = *item.MAC
	}
	if item.MTU != nil {
		mtu = strconv.Itoa(*item.MTU)
	}
	return `<section class="detail-panel"><h2>` + html.EscapeString(i18n.Text(language, "network.details")) + ` ` + html.EscapeString(item.Name) + `</h2><dl class="detail-grid"><div><dt>` + html.EscapeString(i18n.Text(language, "network.type")) + `</dt><dd>` + html.EscapeString(networkTypeLabel(item.Type, item.TypeLabel, language)) + `</dd></div><div><dt>` + html.EscapeString(i18n.Text(language, "network.mac")) + `</dt><dd>` + html.EscapeString(mac) + `</dd></div><div><dt>MTU</dt><dd>` + html.EscapeString(mtu) + `</dd></div><div><dt>IPv4</dt><dd>` + renderAddresses(item.IPv4, language) + `</dd></div><div><dt>IPv6</dt><dd>` + renderAddresses(item.IPv6, language) + `</dd></div></dl></section>`
}

func renderAddresses(addresses []webnetwork.Address, language string) string {
	if len(addresses) == 0 {
		return i18n.Text(language, "network.none")
	}
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, html.EscapeString(address.Address)+"/"+strconv.Itoa(address.Prefix))
	}
	return strings.Join(values, "<br>")
}

func networkStatusLabel(state, language string) string {
	switch state {
	case "up":
		return i18n.Text(language, "network.status.active")
	case "down":
		return i18n.Text(language, "network.status.down")
	default:
		return i18n.Text(language, "network.status.unknown")
	}
}

func networkTypeLabel(kind, fallback, language string) string {
	keys := map[string]string{
		"loopback": "network.type.loopback", "bridge": "network.type.bridge", "bond": "network.type.bond",
		"tun": "network.type.tun", "tap": "network.type.tap",
	}
	if key := keys[kind]; key != "" {
		return i18n.Text(language, key)
	}
	if kind == "ethernet" || kind == "vlan" || kind == "wireguard" {
		return fallback
	}
	return i18n.Text(language, "network.type.unknown")
}

func (a *application) logs(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "logs")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Logs == nil {
		http.Error(response, "Service des journaux indisponible.", http.StatusServiceUnavailable)
		return
	}
	requested := request.URL.Query().Get("source")
	if len(requested) > 255 {
		http.Error(response, "Le journal demandé est invalide.", http.StatusBadRequest)
		return
	}
	logLevel := strings.TrimSpace(request.URL.Query().Get("level"))
	if logLevel != "" && logLevel != "error" && logLevel != "warning" && logLevel != "info" {
		http.Error(response, "Le type d’événement demandé est invalide.", http.StatusBadRequest)
		return
	}
	keyword := strings.TrimSpace(request.URL.Query().Get("keyword"))
	if len(keyword) > 128 || strings.ContainsAny(keyword, "\x00\r\n") {
		http.Error(response, "Le mot-clé demandé est invalide.", http.StatusBadRequest)
		return
	}
	lineCount := 100
	if value := request.URL.Query().Get("lines"); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 1 || parsed > 5000 || value != strconv.Itoa(parsed) {
			http.Error(response, "Le nombre de lignes doit être compris entre 1 et 5000.", http.StatusBadRequest)
			return
		}
		lineCount = parsed
	}
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	if requested == "" && a.dependencies.Settings != nil {
		if settings, settingsErr := a.dependencies.Settings.Settings(ctx); settingsErr == nil {
			requested = strings.TrimSpace(settings.DefaultLog)
		}
	}
	snapshot, err := a.dependencies.Logs.LogsSnapshot(ctx, requested, lineCount)
	if err != nil {
		http.Error(response, "Les journaux n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	lines := filterLogLines(snapshot.Lines, logLevel, keyword)
	lineContent := strings.Join(lines, "\n")
	if len(lines) == 0 {
		lineContent = i18n.Text(language, "logs.lines.empty")
	}
	results := ""
	if len(lines) == 1 {
		results = fmt.Sprintf(i18n.Text(language, "logs.results.one"), lineCount)
	} else {
		results = fmt.Sprintf(i18n.Text(language, "logs.results.many"), len(lines), lineCount)
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)),
		"{{SOURCES}}", renderLogSources(snapshot.Sources, snapshot.Selected, language),
		"{{LEVELS}}", renderLogLevels(logLevel, language),
		"{{LINE_COUNT}}", strconv.Itoa(lineCount),
		"{{KEYWORD}}", html.EscapeString(keyword),
		"{{SELECTED}}", html.EscapeString(logTitle(snapshot.Selected, language)),
		"{{RESET_URL}}", "/logs",
		"{{RESULTS}}", html.EscapeString(results),
		"{{LINES}}", html.EscapeString(lineContent),
	).Replace(i18n.Localize(a.logsPage, language))
	writeHTML(response, page, http.StatusOK)
}

func renderLogLevels(selectedLevel, language string) string {
	options := [][2]string{{"", i18n.Text(language, "logs.level.all")}, {"error", i18n.Text(language, "logs.level.error")}, {"warning", i18n.Text(language, "logs.level.warning")}, {"info", i18n.Text(language, "logs.level.info")}}
	var result strings.Builder
	for _, option := range options {
		result.WriteString(`<option value="` + option[0] + `"`)
		if option[0] == selectedLevel {
			result.WriteString(` selected`)
		}
		result.WriteString(`>` + option[1] + `</option>`)
	}
	return result.String()
}

func filterLogLines(lines []string, level, keyword string) []string {
	keyword = strings.ToLower(keyword)
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(line)
		if keyword != "" && !strings.Contains(lower, keyword) {
			continue
		}
		if level != "" && logLineLevel(lower) != level {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func logLineLevel(lowerLine string) string {
	for _, marker := range []string{"emerg", "alert", "critical", "crit", "fatal", "error", "erreur", "failed", "failure", "échec"} {
		if strings.Contains(lowerLine, marker) {
			return "error"
		}
	}
	for _, marker := range []string{"warning", "warn", "avertissement"} {
		if strings.Contains(lowerLine, marker) {
			return "warning"
		}
	}
	for _, marker := range []string{"info", "notice"} {
		if strings.Contains(lowerLine, marker) {
			return "info"
		}
	}
	return ""
}

func renderLogSources(sources []string, selected, language string) string {
	if len(sources) == 0 {
		return `<option value="">` + html.EscapeString(i18n.Text(language, "logs.source.empty")) + `</option>`
	}
	var result strings.Builder
	for _, source := range sources {
		result.WriteString(`<option value="` + html.EscapeString(source) + `"`)
		if source == selected {
			result.WriteString(` selected`)
		}
		result.WriteString(`>` + html.EscapeString(source) + `</option>`)
	}
	return result.String()
}

func logTitle(selected, language string) string {
	if selected == "" {
		return i18n.Text(language, "logs.selected.empty")
	}
	return selected
}

func (a *application) about(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	_, granted, ok := a.modulePermission(response, request, user, "about")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	language := a.languageForUser(request.Context(), user.ID)
	agplURL := "https://www.gnu.org/licenses/agpl-3.0.html"
	if language == "fr" {
		agplURL = "https://www.gnu.org/licenses/agpl-3.0.fr.html"
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{AGPL_URL}}", agplURL,
	).Replace(i18n.Localize(a.aboutPage, language))
	writeHTML(response, page, http.StatusOK)
}

func (a *application) php(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "php")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.PHP == nil {
		http.Error(response, "Service PHP indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.PHP.PHPSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations PHP n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	canAct := level == "action" || level == "modify"
	notice := ""
	if request.URL.Query().Get("result") == "restarted" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "php.restarted")) + `</p>`
	}
	actionHeader := ""
	if canAct {
		actionHeader = "<th>" + html.EscapeString(i18n.Text(language, "php.action")) + "</th>"
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice,
		"{{CLI_VERSION}}", html.EscapeString(snapshot.CLI.Version), "{{CLI_SAPI}}", html.EscapeString(snapshot.CLI.SAPI), "{{CLI_INI}}", html.EscapeString(localizeUnavailable(snapshot.CLI.IniFile, language)), "{{CLI_SCAN}}", html.EscapeString(localizeUnavailable(snapshot.CLI.ScanDir, language)),
		"{{ACTION_HEADER}}", actionHeader, "{{INSTANCES}}", renderPHPInstances(snapshot.Instances, session.CSRFToken, canAct, language)).Replace(i18n.Localize(a.phpPage, language))
	writeHTML(response, page, http.StatusOK)
}

func (a *application) restartPHP(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "php")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.PHP == nil || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if err := a.dependencies.PHP.RestartPHP(ctx, request.PostForm.Get("runtime")); err != nil {
		http.Error(response, "Le redémarrage de PHP-FPM a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/php?result=restarted", http.StatusSeeOther)
}

func renderPHPInstances(instances []webphp.Instance, csrf string, canAct bool, language string) string {
	columns := 5
	if canAct {
		columns++
	}
	if len(instances) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">` + html.EscapeString(i18n.Text(language, "php.empty")) + `</td></tr>`
	}
	var result strings.Builder
	for _, item := range instances {
		status := item.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		result.WriteString(`<tr><th>` + html.EscapeString(item.Version) + `</th><td>` + html.EscapeString(item.Service) + `</td><td><span class="status-badge status-badge--` + status + `">` + html.EscapeString(phpStatusLabel(item, language)) + `</span></td><td>` + yesNoForLanguage(item.Enabled, language) + `</td><td>` + html.EscapeString(item.State) + `</td>`)
		if canAct {
			result.WriteString(`<td>`)
			if item.Exists {
				result.WriteString(`<form class="inline-form" method="post" action="/php/restart"><input type="hidden" name="_token" value="` + html.EscapeString(csrf) + `"><input type="hidden" name="runtime" value="` + html.EscapeString(item.ID) + `"><button class="danger-button" type="submit">` + html.EscapeString(i18n.Text(language, "php.restart")) + `</button></form>`)
			}
			result.WriteString(`</td>`)
		}
		result.WriteString(`</tr>`)
	}
	return result.String()
}

func phpStatusLabel(instance webphp.Instance, language string) string {
	if !instance.Exists {
		return i18n.Text(language, "php.status.missing")
	}
	if instance.Active {
		return i18n.Text(language, "php.status.active")
	}
	if instance.State == "failed" {
		return i18n.Text(language, "php.status.failed")
	}
	return i18n.Text(language, "php.status.stopped")
}

func localizeUnavailable(value, language string) string {
	if strings.TrimSpace(value) == "" || value == "Non disponible" {
		return i18n.Text(language, "network.unavailable")
	}
	return value
}

func (a *application) mysql(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "mysql")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.MySQL == nil {
		http.Error(response, "Service MySQL indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.MySQL.MySQLSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations MySQL n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	if !snapshot.Server.Service.Exists {
		page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", `<p class="notice">`+html.EscapeString(i18n.Text(language, "mysql.not_installed"))+`</p>`, "{{RESTART}}", "", "{{SERVER}}", renderMySQLServer(snapshot.Server, language), "{{METRICS_HOOK}}", "", "{{METRICS}}", renderMySQLMetrics(snapshot.Metrics, language), "{{DATABASES}}", renderMySQLDatabases(nil, language)).Replace(i18n.Localize(a.mysqlPage, language))
		writeHTML(response, page, http.StatusOK)
		return
	}
	canAct := level == "action" || level == "modify"
	restart := ""
	if canAct && snapshot.Server.Service.Exists {
		restart = `<form method="post" action="/mysql/restart"><input type="hidden" name="_token" value="` + html.EscapeString(session.CSRFToken) + `"><button class="danger-button" type="submit">` + html.EscapeString(i18n.Text(language, "mysql.restart")) + ` ` + html.EscapeString(snapshot.Server.Product) + `</button></form>`
	}
	notice := ""
	if request.URL.Query().Get("result") == "restarted" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "mysql.restarted")) + `</p>`
	}
	for _, warning := range snapshot.Warnings {
		notice += `<p class="notice notice--warning">` + html.EscapeString(localizeMySQLWarning(warning, language)) + `</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{RESTART}}", restart, "{{SERVER}}", renderMySQLServer(snapshot.Server, language), "{{METRICS_HOOK}}", ` data-mysql-metrics data-mysql-metrics-url="/mysql/metrics" data-mysql-metrics-interval="2000"`, "{{METRICS}}", renderMySQLMetrics(snapshot.Metrics, language), "{{DATABASES}}", renderMySQLDatabases(snapshot.Databases, language)).Replace(i18n.Localize(a.mysqlPage, language))
	writeHTML(response, page, http.StatusOK)
}
func (a *application) restartMySQL(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "mysql")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.MySQL == nil || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if err := a.dependencies.MySQL.RestartMySQL(ctx); err != nil {
		http.Error(response, "Le redémarrage MySQL a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/mysql?result=restarted", http.StatusSeeOther)
}
func (a *application) mysqlMetrics(response http.ResponseWriter, request *http.Request) {
	_, _, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	if a.dependencies.MySQL == nil {
		http.Error(response, "Service MySQL indisponible.", http.StatusServiceUnavailable)
		return
	}
	metrics, err := a.dependencies.MySQL.MySQLMetrics(ctx)
	if err != nil {
		http.Error(response, "Les métriques MySQL n’ont pas pu être actualisées.", http.StatusServiceUnavailable)
		return
	}
	writeJSON(response, metrics, http.StatusOK)
}
func renderMySQLServer(s webmysql.Server, language string) string {
	available := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return i18n.Text(language, "mysql.unavailable")
		}
		return value
	}
	unit := i18n.Text(language, "mysql.not_detected")
	if s.Service.Unit != nil {
		unit = *s.Service.Unit
	}
	version := available(strings.TrimSpace(s.Product + " " + s.Version))
	port := i18n.Text(language, "mysql.unavailable")
	if s.Port > 0 {
		port = strconv.FormatInt(s.Port, 10)
	}
	rows := [][2]string{{i18n.Text(language, "mysql.version"), version}, {i18n.Text(language, "mysql.hostname"), available(s.Hostname)}, {i18n.Text(language, "mysql.port"), port}, {i18n.Text(language, "mysql.socket"), available(s.Socket)}, {i18n.Text(language, "mysql.data_directory"), available(s.DataDirectory)}, {i18n.Text(language, "mysql.default_engine"), available(s.DefaultStorageEngine)}, {i18n.Text(language, "mysql.service"), unit}, {i18n.Text(language, "mysql.autostart"), yesNoForLanguage(s.Service.Enabled, language)}, {i18n.Text(language, "mysql.service_state"), available(s.Service.State)}}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(`<div><dt>` + r[0] + `</dt><dd>` + html.EscapeString(r[1]) + `</dd></div>`)
	}
	return b.String()
}
func renderMySQLMetrics(m map[string]int64, language string) string {
	metric := func(key string, format func(int64) string) string {
		value, found := m[key]
		if !found {
			return i18n.Text(language, "mysql.unavailable")
		}
		return format(value)
	}
	decimal := func(value int64) string { return strconv.FormatInt(value, 10) }
	items := [][2]string{{i18n.Text(language, "mysql.connections"), metric("threads_connected", decimal)}, {i18n.Text(language, "mysql.threads"), metric("threads_running", decimal)}, {i18n.Text(language, "mysql.queries"), metric("queries", func(value int64) string { return formatIntegerForLanguage(value, language) })}, {i18n.Text(language, "mysql.slow_queries"), metric("slow_queries", decimal)}}
	var b strings.Builder
	keys := []string{"threads_connected", "threads_running", "queries", "slow_queries"}
	for index, i := range items {
		b.WriteString(`<article class="summary-item" data-mysql-metric="` + keys[index] + `"><span>` + i[0] + `</span><strong>` + i[1] + `</strong></article>`)
	}
	return b.String()
}
func formatFrenchInteger(value int64) string {
	digits := strconv.FormatInt(value, 10)
	start := 0
	if strings.HasPrefix(digits, "-") {
		start = 1
	}
	for position := len(digits) - 3; position > start; position -= 3 {
		digits = digits[:position] + "\u202f" + digits[position:]
	}
	return digits
}

func formatIntegerForLanguage(value int64, language string) string {
	if language != "en" {
		return formatFrenchInteger(value)
	}
	digits := strconv.FormatInt(value, 10)
	start := 0
	if strings.HasPrefix(digits, "-") {
		start = 1
	}
	for position := len(digits) - 3; position > start; position -= 3 {
		digits = digits[:position] + "," + digits[position:]
	}
	return digits
}
func renderMySQLDatabases(items []webmysql.Database, language string) string {
	if len(items) == 0 {
		return `<tr><td colspan="4" class="muted">` + html.EscapeString(i18n.Text(language, "mysql.empty")) + `</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		kind := i.Kind
		if i.Kind == "user" || i.Kind == "system" {
			kind = i18n.Text(language, "mysql.kind."+i.Kind)
		}
		b.WriteString(`<tr><th data-sort-value="` + html.EscapeString(strings.ToLower(i.Name)) + `">` + html.EscapeString(i.Name) + `</th><td data-sort-value="` + html.EscapeString(strings.ToLower(i.Kind)) + `">` + html.EscapeString(kind) + `</td><td data-sort-value="` + strconv.FormatInt(i.Tables, 10) + `">` + formatIntegerForLanguage(i.Tables, language) + `</td><td data-sort-value="` + strconv.FormatInt(i.Size, 10) + `">` + formatByteCountForLanguage(i.Size, language) + `</td></tr>`)
	}
	return b.String()
}
func formatByteCount(value int64) string {
	units := []string{"o", "Kio", "Mio", "Gio", "Tio"}
	amount := float64(value)
	unit := 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	return strings.ReplaceAll(strconv.FormatFloat(amount, 'f', 1, 64), ".", ",") + " " + units[unit]
}

func formatByteCountForLanguage(value int64, language string) string {
	if language != "en" {
		return formatByteCount(value)
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	amount, unit := float64(value), 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	return strconv.FormatFloat(amount, 'f', 1, 64) + " " + units[unit]
}

func localizeMySQLWarning(warning, language string) string {
	keys := map[string]string{
		"Le service est détecté, mais la connexion locale à MySQL/MariaDB a été refusée. Vérifiez l’authentification du client système.": "mysql.warning.login",
		"Les métriques MySQL ne sont pas disponibles.":         "mysql.warning.metrics",
		"La liste des bases MySQL ne peut pas être consultée.": "mysql.warning.databases",
	}
	if key := keys[warning]; key != "" {
		return i18n.Text(language, key)
	}
	return warning
}

func (a *application) tor(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "tor")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Tor == nil {
		http.Error(response, "Service Tor indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	s, err := a.dependencies.Tor.TorSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations Tor n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	if !s.Installed {
		unavailable := i18n.Text(language, "tor.unavailable")
		info := renderPairs([][2]string{{i18n.Text(language, "tor.state"), i18n.Text(language, "tor.not_installed")}})
		config := `<span class="status-badge status-badge--neutral">` + html.EscapeString(unavailable) + `</span><p>` + html.EscapeString(i18n.Text(language, "tor.install_help")) + `</p>`
		status := renderMetricPairs([][2]string{{i18n.Text(language, "tor.bootstrap"), unavailable}, {i18n.Text(language, "tor.memory"), unavailable}, {i18n.Text(language, "tor.tasks"), unavailable}, {i18n.Text(language, "tor.main_pid"), unavailable}})
		page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", `<p class="notice">`+html.EscapeString(i18n.Text(language, "tor.module_available"))+`</p>`, "{{INFO}}", info, "{{CONFIG}}", config, "{{ACTIONS}}", "", "{{INSTANCE_TITLE}}", html.EscapeString(i18n.Text(language, "tor.service")), "{{STATUS}}", status, "{{ONIONS}}", renderOnions(nil, language)).Replace(i18n.Localize(a.torPage, language))
		writeHTML(response, page, http.StatusOK)
		return
	}
	canAct := level == "action" || level == "modify"
	actions := ""
	if canAct && s.Status.Exists {
		actions = `<div class="header-actions"><form method="post" action="/tor/reload"><input type="hidden" name="_token" value="` + html.EscapeString(session.CSRFToken) + `"><button class="primary-button" type="submit">` + html.EscapeString(i18n.Text(language, "tor.reload")) + `</button></form><form method="post" action="/tor/restart"><input type="hidden" name="_token" value="` + html.EscapeString(session.CSRFToken) + `"><button class="danger-button" type="submit">` + html.EscapeString(i18n.Text(language, "tor.restart")) + `</button></form></div>`
	}
	notice := ""
	if result := request.URL.Query().Get("result"); result == "reload" || result == "restart" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "tor.action_done")) + `</p>`
	}
	config := `<span class="status-badge status-badge--danger">` + html.EscapeString(i18n.Text(language, "tor.invalid")) + `</span>`
	if s.ConfigurationValid {
		message := s.ConfigurationMessage
		if message == "La configuration de Tor est valide." || message == "Configuration valide" {
			message = i18n.Text(language, "tor.config_valid_message")
		}
		config = `<span class="status-badge status-badge--success">` + html.EscapeString(i18n.Text(language, "tor.valid")) + `</span><p>` + html.EscapeString(message) + `</p>`
	}
	bootstrap := i18n.Text(language, "tor.unavailable")
	if s.Status.Bootstrap != nil {
		bootstrap = strconv.Itoa(*s.Status.Bootstrap) + " %"
	}
	info := renderPairs([][2]string{{i18n.Text(language, "tor.version"), s.Info.Product + " " + s.Info.Version}, {i18n.Text(language, "tor.configuration"), s.Info.ConfigFile}, {i18n.Text(language, "tor.service"), s.Info.Service}, {i18n.Text(language, "tor.unit"), s.Info.Unit}})
	status := renderMetricPairs([][2]string{{i18n.Text(language, "tor.bootstrap"), bootstrap}, {i18n.Text(language, "tor.memory"), formatByteCountForLanguage(s.Status.Memory, language)}, {i18n.Text(language, "tor.tasks"), formatIntegerForLanguage(s.Status.Tasks, language)}, {i18n.Text(language, "tor.main_pid"), strconv.FormatInt(s.Status.MainPID, 10)}})
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{INFO}}", info, "{{CONFIG}}", config, "{{ACTIONS}}", actions, "{{INSTANCE_TITLE}}", html.EscapeString(i18n.Text(language, "tor.instance"))+" "+html.EscapeString(s.Info.Service), "{{STATUS}}", status, "{{ONIONS}}", renderOnions(s.Services, language)).Replace(i18n.Localize(a.torPage, language))
	writeHTML(response, page, http.StatusOK)
}
func (a *application) torAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "tor")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	action := request.PathValue("action")
	if action != "reload" && action != "restart" {
		http.NotFound(response, request)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if a.dependencies.Tor == nil || a.dependencies.Tor.TorAction(ctx, action) != nil {
		http.Error(response, "L’action Tor a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/tor?result="+action, http.StatusSeeOther)
}
func renderPairs(rows [][2]string) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(`<div><dt>` + r[0] + `</dt><dd>` + html.EscapeString(r[1]) + `</dd></div>`)
	}
	return b.String()
}
func renderMetricPairs(rows [][2]string) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(`<article class="summary-item"><span>` + r[0] + `</span><strong>` + html.EscapeString(r[1]) + `</strong></article>`)
	}
	return b.String()
}
func renderOnions(items []webtor.Onion, language string) string {
	if len(items) == 0 {
		return `<tr><td colspan="3" class="muted">` + html.EscapeString(i18n.Text(language, "tor.empty")) + `</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		host := i18n.Text(language, "tor.address_unavailable")
		if i.Hostname != nil {
			host = *i.Hostname
		}
		b.WriteString(`<tr><th>` + html.EscapeString(i.ID) + `</th><td>` + html.EscapeString(host) + `</td><td>` + strconv.Itoa(len(i.Ports)) + `</td></tr>`)
	}
	return b.String()
}

func (a *application) apache(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "apache")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Apache == nil {
		http.Error(response, "Service Apache indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	s, err := a.dependencies.Apache.ApacheSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations Apache n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	if !s.Installed && s.Version == "" {
		page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", `<p class="notice">`+html.EscapeString(i18n.Text(language, "apache.not_installed_notice"))+`</p>`, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "apache.installation"), i18n.Text(language, "apache.not_installed")}}), "{{CONFIG}}", `<span class="status-badge status-badge--neutral">`+html.EscapeString(i18n.Text(language, "apache.unavailable"))+`</span><p>`+html.EscapeString(i18n.Text(language, "apache.install_help"))+`</p>`, "{{ACTIONS}}", "", "{{SUMMARY}}", renderMetricPairs([][2]string{{i18n.Text(language, "apache.vhosts"), "0"}, {i18n.Text(language, "apache.sites"), "0"}, {i18n.Text(language, "apache.modules"), "0"}, {i18n.Text(language, "apache.active_sites"), "0"}}), "{{VHOSTS}}", renderVHosts(nil, language), "{{SITE_ACTIONS}}", renderApacheSites(nil, session.CSRFToken, false, language), "{{CREATE_SITE}}", "").Replace(i18n.Localize(a.apachePage, language))
		writeHTML(response, page, http.StatusOK)
		return
	}
	canAct := level == "action" || level == "modify"
	canModify := level == "modify"
	actions := ""
	if canAct {
		token := html.EscapeString(session.CSRFToken)
		actions = `<div class="header-actions"><form method="post" action="/apache/reload"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button" type="submit">` + html.EscapeString(i18n.Text(language, "apache.reload")) + `</button></form><form method="post" action="/apache/restart"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button" type="submit">` + html.EscapeString(i18n.Text(language, "apache.restart")) + `</button></form></div>`
	}
	configClass := "danger"
	configLabel := i18n.Text(language, "apache.invalid")
	if s.ConfigValid {
		configClass = "success"
		configLabel = i18n.Text(language, "apache.valid")
	}
	config := `<span class="status-badge status-badge--` + configClass + `">` + configLabel + `</span><p>` + html.EscapeString(s.ConfigMessage) + `</p>`
	notice := ""
	if action := request.URL.Query().Get("result"); action == "reload" || action == "restart" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "apache.action_done")) + `</p>`
	} else if action != "" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "apache.operation_done")) + `</p>`
	} else if message := request.URL.Query().Get("error"); message != "" {
		notice = `<p class="notice notice--danger">` + html.EscapeString(message) + `</p>`
	}
	summary := renderMetricPairs([][2]string{{i18n.Text(language, "apache.vhosts"), strconv.Itoa(len(s.VHosts))}, {i18n.Text(language, "apache.sites"), strconv.Itoa(len(s.Sites))}, {i18n.Text(language, "apache.modules"), strconv.Itoa(len(s.Modules))}, {i18n.Text(language, "apache.active_sites"), strconv.Itoa(enabledSites(s.Sites))}})
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "apache.version"), s.Version}, {i18n.Text(language, "apache.build"), s.Built}}), "{{CONFIG}}", config, "{{ACTIONS}}", actions, "{{SUMMARY}}", summary, "{{VHOSTS}}", renderVHosts(s.VHosts, language), "{{SITE_ACTIONS}}", renderApacheSites(s.Sites, session.CSRFToken, canModify, language), "{{CREATE_SITE}}", renderApacheCreate(session.CSRFToken, canModify, language)).Replace(i18n.Localize(a.apachePage, language))
	writeHTML(response, page, http.StatusOK)
}

func (a *application) apacheSite(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "apache")
	if !ok {
		return
	}
	if !granted || level != "modify" {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	if a.dependencies.Apache == nil {
		http.Error(response, "Service Apache indisponible.", http.StatusServiceUnavailable)
		return
	}
	config, err := a.dependencies.Apache.ApacheSite(ctx, request.PathValue("id"))
	if err != nil {
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(response).Encode(config)
}
func (a *application) apacheAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "apache")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	action := request.PathValue("action")
	allowed := action == "reload" || action == "restart" || (level == "modify" && (action == "create" || action == "update" || action == "enable" || action == "disable" || action == "delete" || action == "issue-certificate"))
	if !allowed {
		http.NotFound(response, request)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 1024*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	var err error
	if a.dependencies.Apache == nil {
		err = errors.New("Service Apache indisponible.")
	} else if action == "reload" || action == "restart" {
		err = a.dependencies.Apache.ApacheAction(ctx, action)
	} else if action == "issue-certificate" {
		domains, validationErr := certbotIssueValues(request.PostForm.Get("email"), request.PostForm.Get("domains"))
		if validationErr != nil {
			err = validationErr
		} else if a.dependencies.Certbot == nil || len(domains) == 0 {
			err = errors.New("La demande de certificat est incomplète.")
		} else {
			redirect := "no-redirect"
			if request.PostForm.Get("redirect") == "1" {
				redirect = "redirect"
			}
			var id string
			id, err = a.dependencies.Certbot.Start(ctx, "issue", append([]string{strings.TrimSpace(request.PostForm.Get("email")), redirect}, domains...))
			if err == nil {
				http.Redirect(response, request, "/certbot?execution="+url.QueryEscape(id), http.StatusSeeOther)
				return
			}
		}
	} else {
		identifier, filename, content := request.PostForm.Get("config_id"), request.PostForm.Get("filename"), request.PostForm.Get("content")
		if action == "create" {
			filename, content, err = apacheSiteConfiguration(request.PostForm)
		}
		if err == nil {
			err = a.dependencies.Apache.ApacheSiteAction(ctx, action, identifier, filename, content)
		}
	}
	if err != nil {
		http.Redirect(response, request, "/apache?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(response, request, "/apache?result="+action, http.StatusSeeOther)
}
func enabledSites(items []webapache.Site) int {
	count := 0
	for _, item := range items {
		if item.Enabled {
			count++
		}
	}
	return count
}

var apacheHostPattern = regexp.MustCompile(`^(?:\*\.)?[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
var certbotDomainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var certbotEmailPattern = regexp.MustCompile(`^[A-Za-z0-9.!#$%&'*+/=?^_{}|~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`)

func certbotIssueValues(email, rawDomains string) ([]string, error) {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 254 || !certbotEmailPattern.MatchString(email) {
		return nil, errors.New("L’adresse e-mail indiquée pour Certbot est invalide.")
	}
	domains, seen := []string{}, map[string]bool{}
	for _, value := range strings.Fields(strings.NewReplacer(",", " ", ";", " ").Replace(rawDomains)) {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if !certbotDomainPattern.MatchString(domain) || strings.HasSuffix(domain, ".onion") {
			return nil, errors.New("Le domaine « " + value + " » ne peut pas être utilisé par Certbot. Un nom DNS public complet est requis.")
		}
		if !seen[domain] {
			seen[domain] = true
			domains = append(domains, domain)
		}
	}
	if len(domains) == 0 {
		return nil, errors.New("Au moins un domaine doit être indiqué pour Certbot.")
	}
	return domains, nil
}

func apacheSiteConfiguration(form url.Values) (string, string, error) {
	serverName := strings.TrimSpace(form.Get("server_name"))
	if !apacheHostPattern.MatchString(serverName) {
		return "", "", errors.New("Le nom de domaine est invalide.")
	}
	aliases := strings.Fields(strings.NewReplacer(",", " ", ";", " ").Replace(form.Get("aliases")))
	for _, alias := range aliases {
		if !apacheHostPattern.MatchString(alias) {
			return "", "", errors.New("Un alias de domaine est invalide.")
		}
	}
	filename := strings.TrimPrefix(strings.ToLower(serverName), "*.") + ".conf"
	var b strings.Builder
	b.WriteString("<VirtualHost *:80>\n    ServerName " + serverName + "\n")
	if len(aliases) > 0 {
		b.WriteString("    ServerAlias " + strings.Join(aliases, " ") + "\n")
	}
	switch form.Get("site_type") {
	case "website":
		root := strings.TrimSpace(form.Get("document_root"))
		if root == "" || !strings.HasPrefix(root, "/") || strings.ContainsAny(root, "\r\n\x00") {
			return "", "", errors.New("Le chemin DocumentRoot est invalide.")
		}
		override := form.Get("allow_override")
		if override != "All" {
			override = "None"
		}
		options := "-Indexes"
		if form.Get("follow_sym_links") == "1" {
			options += " +FollowSymLinks"
		}
		b.WriteString("    DocumentRoot " + root + "\n\n    <Directory " + root + ">\n        Options " + options + "\n        AllowOverride " + override + "\n        Require all granted\n    </Directory>\n")
	case "proxy":
		target, err := url.Parse(strings.TrimSpace(form.Get("target_url")))
		if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || strings.ContainsAny(target.String(), "\r\n\x00") {
			return "", "", errors.New("L’URL du proxy inverse est invalide.")
		}
		value := strings.TrimRight(target.String(), "/") + "/"
		b.WriteString("    ProxyPreserveHost On\n    ProxyPass / " + value + "\n    ProxyPassReverse / " + value + "\n")
	default:
		return "", "", errors.New("Le type de site Apache est invalide.")
	}
	b.WriteString("\n    ErrorLog ${APACHE_LOG_DIR}/" + strings.TrimSuffix(filename, ".conf") + "-error.log\n    CustomLog ${APACHE_LOG_DIR}/" + strings.TrimSuffix(filename, ".conf") + "-access.log combined\n</VirtualHost>\n")
	return filename, b.String(), nil
}

func renderApacheSites(items []webapache.Site, token string, canModify bool, language string) string {
	if len(items) == 0 {
		return `<tr><td colspan="6" class="muted">` + html.EscapeString(i18n.Text(language, "apache.empty_sites")) + `</td></tr>`
	}
	var b strings.Builder
	for _, site := range items {
		state, stateClass := i18n.Text(language, "apache.disabled"), "muted"
		if site.Enabled {
			state, stateClass = i18n.Text(language, "apache.enabled"), "success"
		}
		actions := `<span class="muted">` + html.EscapeString(i18n.Text(language, "apache.read_only")) + `</span>`
		if canModify {
			actions = `<select class="compact-select" aria-label="` + html.EscapeString(i18n.Text(language, "apache.action_for")) + ` ` + html.EscapeString(site.Filename) + `" data-apache-site-action data-config-id="` + html.EscapeString(site.ConfigID) + `" data-filename="` + html.EscapeString(site.Filename) + `" data-domains="` + html.EscapeString(strings.Join(site.ServerNames, " ")) + `" data-csrf="` + html.EscapeString(token) + `"><option value="">` + html.EscapeString(i18n.Text(language, "apache.action_menu")) + `</option><option value="edit">` + html.EscapeString(i18n.Text(language, "apache.edit")) + `</option>`
			if site.Enabled {
				actions += `<option value="disable">` + html.EscapeString(i18n.Text(language, "apache.disable")) + `</option>`
			} else {
				actions += `<option value="enable">` + html.EscapeString(i18n.Text(language, "apache.enable")) + `</option>`
			}
			if len(site.ServerNames) > 0 {
				actions += `<option value="certificate">` + html.EscapeString(i18n.Text(language, "apache.certificate")) + `</option>`
			}
			actions += `<option value="delete">` + html.EscapeString(i18n.Text(language, "apache.delete")) + `</option></select>`
		}
		ports := make([]string, len(site.Ports))
		for i, port := range site.Ports {
			ports[i] = strconv.Itoa(port)
		}
		b.WriteString(`<tr><td>` + actions + `</td><th>` + html.EscapeString(site.Filename) + `</th><td><span class="status-badge status-badge--` + stateClass + `">` + state + `</span></td><td>` + html.EscapeString(strings.Join(site.ServerNames, ", ")) + `</td><td>` + html.EscapeString(strings.Join(ports, ", ")) + `</td><td>` + formatIntegerForLanguage(site.Size, language) + ` ` + html.EscapeString(i18n.Text(language, "apache.bytes")) + `</td></tr>`)
	}
	return b.String()
}

func renderApacheCreate(token string, canModify bool, language string) string {
	if !canModify {
		return ""
	}
	t := func(key string) string { return html.EscapeString(i18n.Text(language, key)) }
	return `<button class="primary-button" type="button" data-apache-create-open>` + t("apache.add_site") + `</button>
<dialog class="action-dialog action-dialog--wide" data-apache-create-dialog><form method="post" action="/apache/create"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><h2>` + t("apache.add_site_title") + `</h2><label>` + t("apache.site_type") + `<select name="site_type" data-apache-site-type><option value="website">` + t("apache.website") + `</option><option value="proxy">` + t("apache.reverse_proxy") + `</option></select></label><label>` + t("apache.domain_name") + `<input name="server_name" required placeholder="example.org"></label><label>` + t("apache.domain_aliases") + `<input name="aliases" placeholder="www.example.org"></label><fieldset data-apache-website-fields><label>DocumentRoot<input name="document_root" value="/var/www/" required></label><label>AllowOverride<select name="allow_override"><option value="None">None</option><option value="All">All</option></select></label><label class="check-row"><input type="checkbox" name="follow_sym_links" value="1" checked> ` + t("apache.follow_symlinks") + `</label></fieldset><fieldset data-apache-proxy-fields hidden><label>` + t("apache.target_url") + `<input type="url" name="target_url" placeholder="http://127.0.0.1:3000"></label></fieldset><div class="dialog-actions"><button class="secondary-button" type="button" data-apache-create-close>` + t("apache.cancel") + `</button><button class="primary-button" type="submit">` + t("apache.create_validate") + `</button></div></form></dialog>
<dialog class="action-dialog action-dialog--wide" data-apache-edit-dialog><form method="post" action="/apache/update"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input type="hidden" name="config_id"><h2>` + t("apache.edit_vhost") + `</h2><p class="muted" data-apache-edit-filename></p><label>` + t("apache.configuration") + `<textarea name="content" rows="20" required spellcheck="false"></textarea></label><div class="dialog-actions"><button class="secondary-button" type="button" data-apache-edit-close>` + t("apache.cancel") + `</button><button class="primary-button" type="submit">` + t("apache.save_validate") + `</button></div></form></dialog>
<dialog class="action-dialog" data-apache-certificate-dialog><form method="post" action="/apache/issue-certificate"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><h2>` + t("apache.certificate") + `</h2><label>` + t("apache.domains") + `<input name="domains" required></label><label>` + t("apache.email") + `<input type="email" name="email" required></label><label class="check-row"><input type="checkbox" name="redirect" value="1" checked> ` + t("apache.redirect_https") + `</label><div class="dialog-actions"><button class="secondary-button" type="button" data-apache-certificate-close>` + t("apache.cancel") + `</button><button class="primary-button" type="submit">` + t("apache.run_certbot") + `</button></div></form></dialog>`
}
func renderVHosts(items []webapache.VHost, language string) string {
	if len(items) == 0 {
		return `<tr><td colspan="4" class="muted">` + html.EscapeString(i18n.Text(language, "apache.empty_vhosts")) + `</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		root := i18n.Text(language, "apache.root_undefined")
		if i.DocumentRoot != nil {
			root = *i.DocumentRoot
		}
		b.WriteString(`<tr><th>` + html.EscapeString(i.ServerName) + `</th><td>` + strconv.Itoa(i.Port) + `</td><td>` + html.EscapeString(i.ConfigFile) + `</td><td>` + html.EscapeString(root) + `</td></tr>`)
	}
	return b.String()
}

func (a *application) fail2ban(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "fail2ban")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	language := a.languageForUser(request.Context(), user.ID)
	if a.dependencies.Fail2ban == nil {
		http.Error(response, "Service Fail2ban indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	s, err := a.dependencies.Fail2ban.Fail2banSnapshot(ctx, request.URL.Query().Get("jail"))
	if err != nil {
		http.Error(response, "Les informations Fail2ban n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	token := html.EscapeString(session.CSRFToken)
	if !s.Installed {
		page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", `<p class="notice">`+html.EscapeString(i18n.Text(language, "fail2ban.not_installed_notice"))+`</p>`, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "common.status"), i18n.Text(language, "fail2ban.not_installed")}, {i18n.Text(language, "fail2ban.autostart"), i18n.Text(language, "common.no")}, {i18n.Text(language, "fail2ban.jails"), "0"}}), "{{SERVICE_ACTIONS}}", "", "{{CONFIG}}", `<span class="status-badge status-badge--neutral">`+html.EscapeString(i18n.Text(language, "fail2ban.unavailable"))+`</span><p>`+html.EscapeString(i18n.Text(language, "fail2ban.install_help"))+`</p>`, "{{JAIL_SELECTOR}}", renderJailSelector(nil, "", language), "{{JAIL}}", "", "{{MODIFY_ACTIONS}}", "").Replace(i18n.Localize(a.fail2banPage, language))
		writeHTML(response, page, http.StatusOK)
		return
	}
	serviceActions := ""
	if level == "action" || level == "modify" {
		serviceActions = `<div class="header-actions"><form method="post" action="/fail2ban/reload"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">` + html.EscapeString(i18n.Text(language, "common.reload")) + `</button></form><form method="post" action="/fail2ban/restart"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button">` + html.EscapeString(i18n.Text(language, "common.restart")) + `</button></form></div>`
	}
	selector := renderJailSelector(s.Status.Jails, s.Selected, language)
	jail, modify := renderJail(s, token, level == "modify", language)
	configClass := "danger"
	if s.ConfigValid {
		configClass = "success"
	}
	notice := ""
	if request.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "fail2ban.action_done")) + `</p>`
	}
	configMessage := s.ConfigMessage
	if language == "en" && s.ConfigValid {
		configMessage = i18n.Text(language, "fail2ban.config_valid")
	}
	state := s.Status.State
	if language == "en" {
		state = map[string]string{"active": "Active", "running": "Running", "inactive": "Inactive", "failed": "Failed"}[strings.ToLower(state)]
		if state == "" {
			state = s.Status.State
		}
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "common.version"), s.Info.Product + " " + s.Info.Version}, {i18n.Text(language, "common.status"), state}, {i18n.Text(language, "fail2ban.autostart"), yesNoForLanguage(s.Status.Enabled, language)}, {i18n.Text(language, "fail2ban.jails"), formatIntegerForLanguage(int64(len(s.Status.Jails)), language)}}), "{{SERVICE_ACTIONS}}", serviceActions, "{{CONFIG}}", `<span class="status-badge status-badge--`+configClass+`">`+html.EscapeString(configMessage)+`</span>`, "{{JAIL_SELECTOR}}", selector, "{{JAIL}}", jail, "{{MODIFY_ACTIONS}}", modify).Replace(i18n.Localize(a.fail2banPage, language))
	writeHTML(response, page, http.StatusOK)
}
func (a *application) fail2banAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "fail2ban")
	if !ok {
		return
	}
	action := request.PathValue("action")
	required := "action"
	if action == "ban" || action == "unban" {
		required = "modify"
	}
	allowed := granted && (level == "modify" || (required == "action" && level == "action"))
	if !allowed {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if a.dependencies.Fail2ban == nil || a.dependencies.Fail2ban.Action(ctx, action, request.PostForm.Get("jail"), request.PostForm.Get("address")) != nil {
		http.Error(response, "L’action Fail2ban a échoué.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/fail2ban?jail="+url.QueryEscape(request.PostForm.Get("jail"))+"&result="+action, http.StatusSeeOther)
}
func renderJailSelector(jails []string, selected, language string) string {
	if len(jails) == 0 {
		return `<p class="muted">` + html.EscapeString(i18n.Text(language, "fail2ban.no_jail")) + `</p>`
	}
	var b strings.Builder
	b.WriteString(`<form class="selector-form" method="get" action="/fail2ban"><label>` + html.EscapeString(i18n.Text(language, "fail2ban.jail")) + `</label><select name="jail">`)
	for _, j := range jails {
		b.WriteString(`<option value="` + html.EscapeString(j) + `"`)
		if j == selected {
			b.WriteString(` selected`)
		}
		b.WriteString(`>` + html.EscapeString(j) + `</option>`)
	}
	b.WriteString(`</select><button class="primary-button">` + html.EscapeString(i18n.Text(language, "common.show")) + `</button></form>`)
	return b.String()
}
func renderJail(s webfail2ban.Snapshot, token string, modify bool, language string) (string, string) {
	if s.Jail == nil {
		return "", ""
	}
	j := s.Jail
	details := renderMetricPairs([][2]string{{i18n.Text(language, "fail2ban.current_failed"), formatIntegerForLanguage(j.CurrentlyFailed, language)}, {i18n.Text(language, "fail2ban.total_failed"), formatIntegerForLanguage(j.TotalFailed, language)}, {i18n.Text(language, "fail2ban.current_banned"), formatIntegerForLanguage(j.CurrentlyBanned, language)}, {i18n.Text(language, "fail2ban.total_banned"), formatIntegerForLanguage(j.TotalBanned, language)}})
	var actions strings.Builder
	actions.WriteString(`<h3>` + html.EscapeString(i18n.Text(language, "fail2ban.banned_addresses")) + `</h3>`)
	if len(j.BannedIPs) == 0 {
		actions.WriteString(`<p class="muted">` + html.EscapeString(i18n.Text(language, "fail2ban.no_banned_ip")) + `</p>`)
	}
	for _, ip := range j.BannedIPs {
		if modify {
			actions.WriteString(`<form class="fail2ban-banned-row" method="post" action="/fail2ban/unban"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="jail" value="` + html.EscapeString(s.Selected) + `"><input type="hidden" name="address" value="` + html.EscapeString(ip) + `"><button class="danger-button">` + html.EscapeString(i18n.Text(language, "fail2ban.unban")) + `</button><code>` + html.EscapeString(ip) + `</code></form>`)
		} else {
			actions.WriteString(`<div class="fail2ban-banned-row"><code>` + html.EscapeString(ip) + `</code></div>`)
		}
	}
	if modify {
		actions.WriteString(`<h3>` + html.EscapeString(i18n.Text(language, "fail2ban.ban_address")) + `</h3><form class="selector-form" method="post" action="/fail2ban/ban"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="jail" value="` + html.EscapeString(s.Selected) + `"><label>` + html.EscapeString(i18n.Text(language, "fail2ban.ip_address")) + `</label><input name="address" required><button class="danger-button">` + html.EscapeString(i18n.Text(language, "fail2ban.ban")) + `</button></form>`)
	}
	return details, actions.String()
}

func (a *application) firewall(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "firewall")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	language := a.languageForUser(request.Context(), user.ID)
	if a.dependencies.Firewall == nil {
		http.Error(response, "Service pare-feu indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	s, err := a.dependencies.Firewall.FirewallSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations du pare-feu n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	token := html.EscapeString(session.CSRFToken)
	reload := ""
	if level == "action" || level == "modify" {
		if s.Info.Active {
			reload = `<div class="header-actions"><form method="post" action="/firewall/reload"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">` + html.EscapeString(i18n.Text(language, "firewall.reload")) + `</button></form><form method="post" action="/firewall/disable" data-firewall-state-form data-firewall-state="disable"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button">` + html.EscapeString(i18n.Text(language, "firewall.disable")) + `</button></form></div>`
		} else {
			reload = `<form method="post" action="/firewall/enable" data-firewall-state-form data-firewall-state="enable"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">` + html.EscapeString(i18n.Text(language, "firewall.enable")) + `</button></form>`
		}
	}
	add := ""
	header := ""
	if level == "modify" {
		header = "<th>" + html.EscapeString(i18n.Text(language, "common.action")) + "</th>"
		add = `<article class="content-card"><h2>` + html.EscapeString(i18n.Text(language, "firewall.add_inbound")) + `</h2><form class="selector-form" method="post" action="/firewall/add"><input type="hidden" name="_token" value="` + token + `"><label>` + html.EscapeString(i18n.Text(language, "common.action")) + `</label><select name="rule_action"><option>allow</option><option>deny</option><option>reject</option><option>limit</option></select><label>` + html.EscapeString(i18n.Text(language, "firewall.ports")) + `</label><input name="ports" required><label>` + html.EscapeString(i18n.Text(language, "firewall.protocol")) + `</label><select name="protocol"><option>tcp</option><option>udp</option></select><label>` + html.EscapeString(i18n.Text(language, "firewall.source")) + `</label><input name="source" value="any" required><button class="danger-button">` + html.EscapeString(i18n.Text(language, "firewall.add_rule")) + `</button></form></article>`
	}
	version := i18n.Text(language, "common.unknown")
	if s.Info.Version != nil {
		version = *s.Info.Version
	}
	notice := ""
	if message := request.URL.Query().Get("error"); message != "" {
		notice = `<p class="notice notice--danger">` + html.EscapeString(message) + `</p>`
	} else if request.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "firewall.action_scheduled")) + `</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "firewall.product"), s.Info.Product + " " + version}, {i18n.Text(language, "firewall.active"), yesNoForLanguage(s.Info.Active, language)}, {"IPv6", optionalBoolForLanguage(s.Info.IPv6, language)}, {i18n.Text(language, "firewall.default_incoming"), s.Info.DefaultIncoming}, {i18n.Text(language, "firewall.default_outgoing"), s.Info.DefaultOutgoing}}), "{{RELOAD}}", reload, "{{ADD}}", add, "{{ACTION_HEADER}}", header, "{{RULES}}", renderFirewallRules(s.Rules, token, level == "modify", language)).Replace(i18n.Localize(a.firewallPage, language))
	writeHTML(response, page, http.StatusOK)
}
func (a *application) firewallAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "firewall")
	if !ok {
		return
	}
	action := request.PathValue("action")
	required := "action"
	if action == "add" || action == "delete" {
		required = "modify"
	}
	allowed := granted && (level == "modify" || (required == "action" && level == "action"))
	if !allowed {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8192)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	var err error
	switch action {
	case "reload":
		err = a.dependencies.Firewall.Reload(ctx)
	case "enable":
		err = a.dependencies.Firewall.SetEnabled(ctx, true)
	case "disable":
		err = a.dependencies.Firewall.SetEnabled(ctx, false)
	case "add":
		err = a.dependencies.Firewall.Add(ctx, request.PostForm.Get("rule_action"), request.PostForm.Get("ports"), request.PostForm.Get("protocol"), request.PostForm.Get("source"))
	case "delete":
		id, e := strconv.Atoi(request.PostForm.Get("id"))
		if e != nil {
			err = e
		} else {
			err = a.dependencies.Firewall.Delete(ctx, id)
		}
	default:
		http.NotFound(response, request)
		return
	}
	if err != nil {
		http.Redirect(response, request, "/firewall?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(response, request, "/firewall?result="+action, http.StatusSeeOther)
}
func optionalBool(v *bool) string {
	if v == nil {
		return "Inconnu"
	}
	return yesNo(*v)
}
func optionalBoolForLanguage(v *bool, language string) string {
	if v == nil {
		return i18n.Text(language, "common.unknown")
	}
	return yesNoForLanguage(*v, language)
}
func renderFirewallRules(items []webfirewall.Rule, token string, modify bool, language string) string {
	return renderFirewallRulesWithServices(items, token, modify, readFirewallServices("/etc/services"), language)
}

func renderFirewallRulesWithServices(items []webfirewall.Rule, token string, modify bool, services map[string]string, language string) string {
	columns := 7
	if modify {
		columns++
	}
	if len(items) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">` + html.EscapeString(i18n.Text(language, "firewall.empty")) + `</td></tr>`
	}
	items = append([]webfirewall.Rule(nil), items...)
	sort.SliceStable(items, func(left, right int) bool {
		leftPort, rightPort := firewallRulePort(items[left]), firewallRulePort(items[right])
		if leftPort == rightPort {
			return items[left].ID < items[right].ID
		}
		return leftPort < rightPort
	})
	var b strings.Builder
	for _, i := range items {
		b.WriteString(`<tr>`)
		if modify {
			b.WriteString(`<td><form class="inline-form" method="post" action="/firewall/delete"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="id" value="` + strconv.Itoa(i.ID) + `"><button class="danger-button">` + html.EscapeString(i18n.Text(language, "common.delete")) + `</button></form></td>`)
		}
		b.WriteString(`<th>` + strconv.Itoa(i.ID) + `</th><td>` + html.EscapeString(i.Action) + `</td><td>` + html.EscapeString(i.Direction) + `</td><td>` + html.EscapeString(i.Protocol) + `</td><td>` + html.EscapeString(formatFirewallRuleTarget(i, services)) + `</td><td>` + html.EscapeString(i.Source) + `</td><td>` + html.EscapeString(i.Family) + `</td></tr>`)
	}
	return b.String()
}

func formatFirewallRuleTarget(rule webfirewall.Rule, services map[string]string) string {
	if ports := formatFirewallPorts(rule.Ports, rule.Protocol, services); ports != "" {
		return ports
	}
	destination := strings.TrimSpace(rule.Destination)
	if destination != "" && !strings.EqualFold(destination, "any") {
		return destination
	}
	return "—"
}

func readFirewallServices(path string) map[string]string {
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	return parseFirewallServices(string(content))
}

func parseFirewallServices(content string) map[string]string {
	services := map[string]string{}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		port, protocol, ok := strings.Cut(strings.ToLower(fields[1]), "/")
		if !ok || (protocol != "tcp" && protocol != "udp") {
			continue
		}
		if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
			continue
		}
		key := port + "/" + protocol
		if _, exists := services[key]; !exists {
			services[key] = strings.ToLower(fields[0])
		}
	}
	return services
}

func formatFirewallPorts(ports []string, protocol string, services map[string]string) string {
	formatted := make([]string, 0, len(ports))
	for _, expression := range ports {
		for _, raw := range strings.Split(expression, ",") {
			port := strings.TrimSpace(raw)
			label := firewallPortService(port, protocol, services)
			if label != "" {
				formatted = append(formatted, port+" ("+label+")")
			} else {
				formatted = append(formatted, port)
			}
		}
	}
	return strings.Join(formatted, ", ")
}

func firewallPortService(port, protocol string, services map[string]string) string {
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}
	protocol = strings.ToLower(protocol)
	if protocol == "tcp" || protocol == "udp" {
		return services[port+"/"+protocol]
	}
	labels := []string{}
	for _, candidate := range []string{"tcp", "udp"} {
		label := services[port+"/"+candidate]
		if label != "" && !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, "/")
}

func firewallRulePort(rule webfirewall.Rule) int {
	if len(rule.Ports) == 0 {
		return int(^uint(0) >> 1)
	}
	value := rule.Ports[0]
	if separator := strings.IndexAny(value, ":,-"); separator >= 0 {
		value = value[:separator]
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return port
}

func (a *application) cron(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "cron")
	if !ok {
		return
	}
	if !granted {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Cron == nil {
		http.Error(response, "Service Cron indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	snapshot, err := a.dependencies.Cron.CronSnapshot(ctx)
	if err != nil {
		http.Error(response, "Les informations Cron n’ont pas pu être chargées.", http.StatusServiceUnavailable)
		return
	}
	language := a.languageForUser(ctx, user.ID)
	token := html.EscapeString(session.CSRFToken)
	create := ""
	header := ""
	if level == "action" || level == "modify" {
		header = "<th>" + html.EscapeString(i18n.Text(language, "cron.actions")) + "</th>"
	}
	if level == "modify" {
		var options strings.Builder
		for _, item := range snapshot.Users {
			label := item.Name
			if item.System {
				label += " (service)"
			}
			options.WriteString(`<option value="` + html.EscapeString(item.Name) + `">` + html.EscapeString(label) + `</option>`)
		}
		create = renderBackupLibrary(token, options.String(), language)
	}
	notice := ""
	if execution := request.URL.Query().Get("execution"); execution != "" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "cron.execution_scheduled")) + `</p>`
	} else if result := request.URL.Query().Get("result"); result != "" {
		notice = `<p class="notice notice--success">` + html.EscapeString(i18n.Text(language, "cron.action_done")) + `</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", token,
		"{{PERMISSION}}", html.EscapeString(permissionLabelForLanguage(level, language)),
		"{{NOTICE}}", notice,
		"{{INFO}}", renderPairs([][2]string{{i18n.Text(language, "cron.version"), snapshot.Info.Product + " " + snapshot.Info.Version}, {i18n.Text(language, "cron.service"), snapshot.Info.Service}, {i18n.Text(language, "cron.systemd_unit"), snapshot.Info.Unit}, {i18n.Text(language, "cron.anacron"), yesNoForLanguage(snapshot.Info.AnacronAvailable, language)}}),
		"{{STATUS}}", renderPairs([][2]string{{i18n.Text(language, "cron.installed"), yesNoForLanguage(snapshot.Status.Exists, language)}, {i18n.Text(language, "cron.active"), yesNoForLanguage(snapshot.Status.Active, language)}, {i18n.Text(language, "cron.autostart"), yesNoForLanguage(snapshot.Status.Enabled, language)}, {i18n.Text(language, "cron.status"), snapshot.Status.State}, {i18n.Text(language, "cron.main_pid"), strconv.FormatInt(snapshot.Status.MainPID, 10)}, {i18n.Text(language, "cron.memory"), formatByteCountForLanguage(snapshot.Status.MemoryBytes, language)}, {i18n.Text(language, "cron.tasks"), formatIntegerForLanguage(snapshot.Status.Tasks, language)}}),
		"{{CREATE}}", create,
		"{{ACTION_HEADER}}", header,
		"{{JOBS}}", renderCronJobs(snapshot.Jobs, token, level, language),
	).Replace(i18n.Localize(a.cronPage, language))
	writeHTML(response, page, http.StatusOK)
}

func renderBackupLibrary(token, userOptions, language string) string {
	months := []string{"Jan", "Fév", "Mar", "Avr", "Mai", "Juin", "Juil", "Août", "Sep", "Oct", "Nov", "Déc"}
	weekdays := []string{"Dim", "Lun", "Mar", "Mer", "Jeu", "Ven", "Sam"}
	content := `<section class="content-card backup-library"><h2>Bibliothèque de tâches Cron</h2><p class="muted">Choisissez un modèle ou créez une tâche personnalisée.</p><div class="cron-library-actions"><label class="library-selector">Type de tâche<select data-backup-template-select><option value="">Sélectionner un modèle</option><optgroup label="Sauvegardes"><option value="mysql">MySQL / MariaDB</option><option value="apache">Configuration Apache</option><option value="sites">Sites de /var/www</option></optgroup></select></label><button class="secondary-button" type="button" data-cron-create-open>Créer une tâche utilisateur</button></div>` +
		`<dialog class="action-dialog action-dialog--wide" data-cron-create-dialog><form method="post" action="/cron/create" data-cron-create-form><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="task_id" data-cron-task-id><input type="hidden" name="schedule" value="0 2 * * *" data-cron-schedule><h2 data-cron-form-title>Créer une tâche utilisateur</h2><label>Utilisateur<select name="user" required data-cron-user>` + userOptions + `</select></label><fieldset class="cron-schedule-builder"><legend>Périodicité</legend><label>Mode<select data-cron-mode><option value="visual">Sélection interactive</option><option value="custom">Expression avancée</option></select></label><div class="cron-choice-groups" data-cron-visual>` + cronChoiceGroup("Minutes", "minute", 0, 59, nil, []int{0}) + cronChoiceGroup("Heures", "hour", 0, 23, nil, []int{2}) + cronChoiceGroup("Jours du mois", "monthday", 1, 31, nil, nil) + cronChoiceGroup("Mois", "month", 1, 12, months, nil) + cronChoiceGroup("Jours de la semaine", "weekday", 0, 6, weekdays, nil) + `</div><label data-cron-custom-field hidden>Expression Cron<input value="0 2 * * *" data-cron-custom></label><p class="muted" data-cron-day-warning hidden>Lorsque les jours du mois et de la semaine sont tous deux limités, Cron exécute généralement la tâche si l’un des deux critères correspond.</p><p class="cron-schedule-preview">Expression générée : <code data-cron-schedule-preview>0 2 * * *</code></p></fieldset><label>Commande<input name="command" required autocomplete="off" data-cron-command></label><div class="form-actions"><button class="primary-button" data-cron-submit>Créer la tâche</button><button class="secondary-button" type="button" data-cron-create-close>Annuler</button></div></form></dialog>` +
		`<dialog class="action-dialog action-dialog--wide" data-backup-dialog><form method="post" action="/cron/backup/create" class="selector-form"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="kind" data-backup-kind><h2 data-backup-title>Nouvelle sauvegarde</h2><label>Nom<input name="name" maxlength="64" required></label><label>Source<input name="source" data-backup-source required></label><label>Fréquence<select name="schedule" required><option value="0 2 * * *">Chaque nuit à 2 h</option><option value="0 3 * * 0">Chaque dimanche à 3 h</option><option value="0 4 1 * *">Chaque mois à 4 h</option></select></label><label>Stockage temporaire local<input name="destination" value="/var/backups/aegisadmin" required></label><fieldset><legend>Destination rsync/SSH</legend><label>Serveur<input name="remote_host" required></label><label>Utilisateur SSH<input name="remote_user" required></label><label>Port SSH<input name="remote_port" type="number" min="1" max="65535" value="22" required></label><label>Répertoire distant<input name="remote_path" value="/var/backups/aegisadmin" required></label><label>Clé SSH privée<input name="ssh_key" value="/etc/aegisadmin-system/backup-ssh/id_ed25519" required></label></fieldset><label>Conservation locale (jours)<input name="retention_days" type="number" min="0" max="3650" value="2" required></label><label class="checkbox-line"><input type="checkbox" name="remove_local" value="true"> Supprimer la copie locale après un transfert réussi</label><div class="form-actions"><button class="primary-button">Créer la tâche</button><button class="secondary-button" type="button" data-backup-close>Annuler</button></div></form></dialog></section>`
	if language != "en" {
		return content
	}
	return strings.NewReplacer(
		"Bibliothèque de tâches Cron", "Cron task library", "Choisissez un modèle ou créez une tâche personnalisée.", "Choose a template or create a custom task.",
		"Type de tâche", "Task type", "Sélectionner un modèle", "Select a template", "Sauvegardes", "Backups", "Configuration Apache", "Apache configuration", "Sites de /var/www", "/var/www sites",
		"Créer une tâche utilisateur", "Create a user task", "Utilisateur", "User", "Périodicité", "Schedule", "Sélection interactive", "Interactive selection", "Expression avancée", "Advanced expression",
		"Minutes", "Minutes", "Heures", "Hours", "Jours du mois", "Days of month", "Mois", "Months", "Jours de la semaine", "Days of week", "Aucune sélection = toutes les valeurs", "No selection = all values", "Tout effacer", "Clear all",
		"Expression Cron", "Cron expression", "Lorsque les jours du mois et de la semaine sont tous deux limités, Cron exécute généralement la tâche si l’un des deux critères correspond.", "When both days of month and weekdays are restricted, Cron generally runs the task when either condition matches.", "Expression générée :", "Generated expression:",
		"Commande", "Command", "Créer la tâche", "Create task", "Annuler", "Cancel", "Nouvelle sauvegarde", "New backup", "Nom", "Name", "Source", "Source", "Fréquence", "Frequency", "Chaque nuit à 2 h", "Every night at 2 AM", "Chaque dimanche à 3 h", "Every Sunday at 3 AM", "Chaque mois à 4 h", "Every month at 4 AM",
		"Stockage temporaire local", "Local temporary storage", "Destination rsync/SSH", "rsync/SSH destination", "Serveur", "Server", "Utilisateur SSH", "SSH user", "Port SSH", "SSH port", "Répertoire distant", "Remote directory", "Clé SSH privée", "Private SSH key", "Conservation locale (jours)", "Local retention (days)", "Supprimer la copie locale après un transfert réussi", "Delete the local copy after a successful transfer",
		"Jan", "Jan", "Fév", "Feb", "Mar", "Mar", "Avr", "Apr", "Mai", "May", "Juin", "Jun", "Juil", "Jul", "Août", "Aug", "Sep", "Sep", "Oct", "Oct", "Nov", "Nov", "Déc", "Dec", "Dim", "Sun", "Lun", "Mon", "Mer", "Wed", "Jeu", "Thu", "Ven", "Fri", "Sam", "Sat",
	).Replace(content)
}

func cronChoiceGroup(title, part string, first, last int, labels []string, selected []int) string {
	chosen := map[int]bool{}
	for _, value := range selected {
		chosen[value] = true
	}
	var result strings.Builder
	result.WriteString(`<fieldset class="cron-choice-group"><legend>` + title + `</legend><p class="muted">Aucune sélection = toutes les valeurs</p><div class="cron-choice-grid">`)
	for value := first; value <= last; value++ {
		label := strconv.Itoa(value)
		if len(labels) > value-first {
			label = labels[value-first]
		}
		checked := ""
		if chosen[value] {
			checked = " checked"
		}
		result.WriteString(`<label><input type="checkbox" value="` + strconv.Itoa(value) + `" data-cron-part="` + part + `"` + checked + `><span>` + label + `</span></label>`)
	}
	result.WriteString(`</div><button class="text-button" type="button" data-cron-clear="` + part + `">Tout effacer</button></fieldset>`)
	return result.String()
}

func (a *application) cronBackupCreate(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "cron")
	if !ok {
		return
	}
	if !granted || level != "modify" {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 32*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	remove := "false"
	if request.PostForm.Get("remove_local") == "true" {
		remove = "true"
	}
	input := webcron.BackupRequest{Name: request.PostForm.Get("name"), Kind: request.PostForm.Get("kind"), Source: request.PostForm.Get("source"), Destination: request.PostForm.Get("destination"), Schedule: request.PostForm.Get("schedule"), RemoteHost: request.PostForm.Get("remote_host"), RemoteUser: request.PostForm.Get("remote_user"), RemotePort: request.PostForm.Get("remote_port"), RemotePath: request.PostForm.Get("remote_path"), SSHKey: request.PostForm.Get("ssh_key"), RetentionDays: request.PostForm.Get("retention_days"), RemoveLocal: remove}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if err := a.dependencies.Cron.CronBackupCreate(ctx, input); err != nil {
		http.Error(response, "La tâche de sauvegarde n’a pas pu être créée : "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	http.Redirect(response, request, "/cron?result=backup-create", http.StatusSeeOther)
}

func (a *application) cronBackupAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "cron")
	if !ok {
		return
	}
	action := request.PathValue("action")
	allowed := granted && (level == "modify" || (level == "action" && action == "run"))
	if !allowed || (action != "run" && action != "delete") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	execution, err := a.dependencies.Cron.CronBackupAction(ctx, action, request.PostForm.Get("task_id"))
	if err != nil {
		http.Error(response, "L’action de sauvegarde a échoué : "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	if action == "run" {
		http.Redirect(response, request, "/cron?execution="+url.QueryEscape(execution), http.StatusSeeOther)
		return
	}
	http.Redirect(response, request, "/cron?result=backup-delete", http.StatusSeeOther)
}

func (a *application) cronAction(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "cron")
	if !ok {
		return
	}
	action := request.PathValue("action")
	required := "modify"
	switch action {
	case "run":
		required = "action"
	case "create", "update", "suspend", "resume", "delete":
	default:
		http.NotFound(response, request)
		return
	}
	allowed := granted && (level == "modify" || (required == "action" && level == "action"))
	if !allowed {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 16*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	if a.dependencies.Cron == nil {
		http.Error(response, "Service Cron indisponible.", http.StatusServiceUnavailable)
		return
	}
	execution, err := a.dependencies.Cron.CronAction(ctx, action, request.PostForm.Get("user"), request.PostForm.Get("task_id"), request.PostForm.Get("schedule"), request.PostForm.Get("command"))
	if err != nil {
		http.Error(response, "L’action Cron a échoué : "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	if action == "run" {
		http.Redirect(response, request, "/cron?execution="+url.QueryEscape(execution), http.StatusSeeOther)
		return
	}
	http.Redirect(response, request, "/cron?result="+url.QueryEscape(action), http.StatusSeeOther)
}

func (a *application) cronResult(response http.ResponseWriter, request *http.Request) {
	_, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	level, granted, ok := a.modulePermission(response, request, user, "cron")
	if !ok {
		return
	}
	if !granted || (level != "action" && level != "modify") {
		http.Error(response, "Accès interdit.", http.StatusForbidden)
		return
	}
	if a.dependencies.Cron == nil {
		http.Error(response, "Service Cron indisponible.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	result, err := a.dependencies.Cron.CronResult(ctx, request.PathValue("id"))
	if err != nil {
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	writeJSON(response, result, http.StatusOK)
}

func renderCronJobs(items []webcron.Job, token, level, language string) string {
	columns := 5
	if level == "action" || level == "modify" {
		columns++
	}
	if len(items) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">` + html.EscapeString(i18n.Text(language, "cron.empty")) + `</td></tr>`
	}
	var result strings.Builder
	for _, item := range items {
		backupID := backupTaskID(item.Command)
		state, class := i18n.Text(language, "cron.suspended"), "warning"
		if item.Enabled {
			state, class = i18n.Text(language, "cron.enabled"), "success"
		}
		result.WriteString(`<tr><td><span class="status-badge status-badge--` + class + `">` + state + `</span></td>`)
		if level == "action" || level == "modify" {
			result.WriteString(`<td>`)
			if backupID != "" {
				result.WriteString(`<select class="cron-action-select" data-backup-action data-csrf="` + token + `" data-task-id="` + backupID + `"><option value="">` + html.EscapeString(i18n.Text(language, "cron.action")) + `</option><option value="run">` + html.EscapeString(i18n.Text(language, "cron.run")) + `</option>`)
				if level == "modify" {
					result.WriteString(`<option value="delete">` + html.EscapeString(i18n.Text(language, "cron.delete")) + `</option>`)
				}
				result.WriteString(`</select>`)
			} else if item.Editable {
				result.WriteString(`<select class="cron-action-select" data-cron-action data-csrf="` + token + `" data-user="` + html.EscapeString(item.User) + `" data-task-id="` + html.EscapeString(item.ID) + `" data-schedule="` + html.EscapeString(item.Schedule) + `" data-command="` + html.EscapeString(item.Command) + `"><option value="">` + html.EscapeString(i18n.Text(language, "cron.action")) + `</option>`)
				if level == "modify" {
					result.WriteString(`<option value="edit">` + html.EscapeString(i18n.Text(language, "cron.edit")) + `</option>`)
				}
				if item.Enabled {
					result.WriteString(`<option value="run">` + html.EscapeString(i18n.Text(language, "cron.run")) + `</option>`)
				}
				if level == "modify" && item.Enabled {
					result.WriteString(`<option value="suspend">` + html.EscapeString(i18n.Text(language, "cron.suspend")) + `</option>`)
				}
				if level == "modify" && !item.Enabled {
					result.WriteString(`<option value="resume">` + html.EscapeString(i18n.Text(language, "cron.resume")) + `</option>`)
				}
				if level == "modify" {
					result.WriteString(`<option value="delete">` + html.EscapeString(i18n.Text(language, "cron.delete")) + `</option>`)
				}
				result.WriteString(`</select>`)
			} else {
				result.WriteString(`<span class="muted">` + html.EscapeString(i18n.Text(language, "cron.read_only")) + `</span>`)
			}
			result.WriteString(`</td>`)
		}
		result.WriteString(`<th>` + html.EscapeString(item.User) + `</th><td>` + html.EscapeString(item.Schedule) + `</td><td><code>` + html.EscapeString(item.Command) + `</code></td><td>` + html.EscapeString(item.Source) + `</td></tr>`)
	}
	return result.String()
}

func backupTaskID(command string) string {
	const prefix = "/usr/libexec/aegisadmin/aegisadmin-backup "
	trimmed := strings.TrimSpace(command)
	if !strings.HasPrefix(trimmed, prefix) {
		return ""
	}
	value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	if !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(value) {
		return ""
	}
	return value
}

func (a *application) logout(response http.ResponseWriter, request *http.Request) {
	session, found := a.requestSession(request)
	if !found || session.State != websession.StateAuthenticated {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		http.Error(response, "La requête est invalide.", http.StatusBadRequest)
		return
	}
	if a.dependencies.Users != nil {
		ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		if user, ok, _ := a.dependencies.Users.FindByID(ctx, session.UserID); ok {
			a.recordAccess(request, &user, user.Login, "logout", true)
		}
		cancel()
	}
	if err := a.dependencies.Sessions.Destroy(session.ID); err != nil {
		http.Error(response, "La déconnexion n’a pas pu être enregistrée.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.ExpiredCookie())
	http.Redirect(response, request, "/login", http.StatusSeeOther)
}

func (a *application) authenticatedUser(response http.ResponseWriter, request *http.Request) (websession.Session, authstore.User, bool) {
	session, found := a.requestSession(request)
	if !found || session.State != websession.StateAuthenticated || a.dependencies.Users == nil {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	user, userFound, err := a.dependencies.Users.FindByID(ctx, session.UserID)
	if err != nil {
		http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
		return websession.Session{}, authstore.User{}, false
	}
	if !userFound || user.Status != "active" || user.AuthVersion != session.AuthVersion {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false
	}
	return session, user, true
}

func renderNavigation(categories []authstore.NavigationCategory) string {
	var result strings.Builder
	for _, category := range categories {
		result.WriteString(`<section class="navigation-category"><h2>`)
		result.WriteString(html.EscapeString(category.Name))
		result.WriteString(`</h2><ul class="module-list">`)
		for _, module := range category.Modules {
			result.WriteString(`<li class="module-item"><span class="module-icon" aria-hidden="true">`)
			result.WriteString(html.EscapeString(module.Icon))
			result.WriteString(`</span><div><div class="module-meta"><strong>`)
			location := ""
			switch module.Key {
			case "dashboard":
				location = "/go/dashboard"
			case "storage":
				location = "/storage"
			case "services":
				location = "/services"
			case "network":
				location = "/network"
			case "logs":
				location = "/logs"
			case "about":
				location = "/about"
			case "php":
				location = "/php"
			case "mysql":
				location = "/mysql"
			case "tor":
				location = "/tor"
			case "apache":
				location = "/apache"
			case "fail2ban":
				location = "/fail2ban"
			case "firewall":
				location = "/firewall"
			case "cron":
				location = "/cron"
			case "certbot":
				location = "/certbot"
			case "updates":
				location = "/updates"
			case "users":
				location = "/users"
			case "modules":
				location = "/modules"
			case "setting":
				location = "/setting"
			case "configuration":
				location = "/configuration"
			}
			if location != "" {
				result.WriteString(`<a href="`)
				result.WriteString(location)
				result.WriteString(`">`)
				result.WriteString(html.EscapeString(module.Name))
				result.WriteString(`</a>`)
			} else {
				result.WriteString(html.EscapeString(module.Name))
			}
			result.WriteString(`</strong><span class="permission-badge">`)
			result.WriteString(html.EscapeString(permissionLabel(module.PermissionLevel)))
			result.WriteString(`</span></div>`)
			if location == "" {
				result.WriteString(`<span class="migration-label">Migration Go à venir</span>`)
			}
			result.WriteString(`</div></li>`)
		}
		result.WriteString(`</ul></section>`)
	}
	return result.String()
}

func redirectCanonical(target string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != target+"/" {
			http.NotFound(response, request)
			return
		}
		http.Redirect(response, request, target, http.StatusPermanentRedirect)
	}
}

func permissionLabel(level string) string {
	switch level {
	case "modify":
		return "Modification"
	case "action":
		return "Actions"
	default:
		return "Consultation"
	}
}

func permissionLabelForLanguage(level, language string) string {
	if language != "en" {
		return permissionLabel(level)
	}
	switch level {
	case "modify":
		return "Modify"
	case "action":
		return "Actions"
	default:
		return "View"
	}
}

func (a *application) protected(state websession.State, title, message string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		session, found := a.requestSession(request)
		if !found || session.State != state || a.dependencies.Users == nil {
			a.invalidate(response, session.ID)
			http.Redirect(response, request, "/login", http.StatusSeeOther)
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		defer cancel()
		user, userFound, err := a.dependencies.Users.FindByID(ctx, session.UserID)
		if err != nil {
			http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
			return
		}
		if !userFound || user.Status != "active" || user.AuthVersion != session.AuthVersion {
			a.invalidate(response, session.ID)
			http.Redirect(response, request, "/login", http.StatusSeeOther)
			return
		}
		page := strings.NewReplacer(
			"{{TITLE}}", html.EscapeString(title),
			"{{MESSAGE}}", html.EscapeString(message),
		).Replace(a.statePage)
		writeHTML(response, page, http.StatusOK)
	}
}

func (a *application) password(response http.ResponseWriter, request *http.Request) {
	session, user, mandatory, found := a.passwordUser(response, request)
	if !found {
		return
	}
	a.renderPassword(response, session.CSRFToken, "", http.StatusOK, mandatory, user)
}

func (a *application) changePassword(response http.ResponseWriter, request *http.Request) {
	session, user, mandatory, found := a.passwordUser(response, request)
	if !found {
		return
	}
	if a.dependencies.Passwords == nil || a.dependencies.PasswordLimiter == nil {
		http.Error(response, "Service de changement de mot de passe indisponible.", http.StatusServiceUnavailable)
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		a.renderPassword(response, session.CSRFToken, "password.error.request", http.StatusBadRequest, mandatory, user)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderPassword(response, session.CSRFToken, "password.error.request", http.StatusBadRequest, mandatory, user)
		return
	}
	currentPassword := request.PostForm.Get("current_password")
	newPassword := request.PostForm.Get("new_password")
	confirmation := request.PostForm.Get("new_password_confirmation")
	address, account := requestAddress(request), user.Login
	if !a.dependencies.PasswordLimiter.Allowed(address, account) ||
		!webauth.VerifyPassword(currentPassword, user.PasswordHash) {
		a.dependencies.PasswordLimiter.Failure(address, account)
		a.renderPassword(response, session.CSRFToken, "password.error.current", http.StatusUnauthorized, mandatory, user)
		return
	}
	if subtle.ConstantTimeCompare([]byte(newPassword), []byte(confirmation)) != 1 {
		a.renderPassword(response, session.CSRFToken, "password.error.confirmation", http.StatusBadRequest, mandatory, user)
		return
	}
	if webauth.VerifyPassword(newPassword, user.PasswordHash) {
		a.renderPassword(response, session.CSRFToken, "password.error.same", http.StatusBadRequest, mandatory, user)
		return
	}
	newHash, err := webauth.HashPassword(newPassword)
	if err != nil {
		a.renderPassword(response, session.CSRFToken, "password.error.length", http.StatusBadRequest, mandatory, user)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	updated, err := a.dependencies.Passwords.ChangePassword(
		ctx, user.ID, user.AuthVersion, user.PasswordHash, newHash,
	)
	if err != nil {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return
	}
	a.dependencies.PasswordLimiter.Success(account)
	a.recordAccess(request, &updated, updated.Login, "password_changed", true)
	rotated, err := a.dependencies.Sessions.Rotate(
		session.ID, websession.StateAuthenticated, updated.ID, updated.AuthVersion,
	)
	if err != nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.Cookie(rotated.ID))
	http.Redirect(response, request, "/go/dashboard", http.StatusSeeOther)
}

func (a *application) passwordUser(response http.ResponseWriter, request *http.Request) (websession.Session, authstore.User, bool, bool) {
	session, found := a.requestSession(request)
	mandatory := session.State == websession.StatePassword
	if !found || (session.State != websession.StatePassword && session.State != websession.StateAuthenticated) || a.dependencies.Users == nil {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false, false
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	user, userFound, err := a.dependencies.Users.FindByID(ctx, session.UserID)
	if err != nil {
		http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
		return websession.Session{}, authstore.User{}, false, false
	}
	if !userFound || user.Status != "active" || user.AuthVersion != session.AuthVersion || (mandatory && !user.MustChangePassword) {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false, false
	}
	return session, user, mandatory, true
}

func (a *application) renderPassword(response http.ResponseWriter, csrf, messageKey string, status int, mandatory bool, user authstore.User) {
	language := a.languageForUser(context.Background(), user.ID)
	t := func(key string) string { return i18n.Text(language, key) }
	errorMessage := ""
	if messageKey != "" {
		errorMessage = `<p class="error">` + html.EscapeString(t(messageKey)) + `</p>`
	}
	template := a.accountPasswordPage
	twoFactor := ""
	languagePreference := ""
	if mandatory {
		template = a.passwordPage
	} else {
		languagePreference = `<section class="content-card account-password-card"><h2>` + html.EscapeString(t("account.language.title")) + `</h2><p class="muted">` + html.EscapeString(t("account.language.help")) + `</p><form class="selector-form" method="post" action="/go/account/language"><input type="hidden" name="_token" value="` + html.EscapeString(csrf) + `"><label for="account-language">` + html.EscapeString(t("account.language.label")) + `</label><select id="account-language" name="language" required>` + renderLanguageOptions(language) + `</select><button class="primary-button">` + html.EscapeString(t("account.language.save")) + `</button></form></section>`
	}
	if !mandatory && user.TOTPEnabledAt.Valid {
		detail := t("two_factor.optional")
		action := `<form method="post" action="/go/account/two-factor/disable"><input type="hidden" name="_token" value="` + html.EscapeString(csrf) + `"><label>` + html.EscapeString(t("two_factor.current_code")) + `<input name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9 ]{6,11}" maxlength="11" required></label><button class="danger-button">` + html.EscapeString(t("two_factor.disable")) + `</button></form>`
		if user.TwoFactorRequired {
			detail = t("two_factor.required")
			action = ""
		}
		twoFactor = `<section class="content-card account-password-card"><h2>` + html.EscapeString(t("two_factor.title")) + `</h2><p>` + html.EscapeString(t("two_factor.status")) + ` : <strong>` + html.EscapeString(t("two_factor.enabled")) + `</strong>. ` + html.EscapeString(detail) + `</p>` + action + `</section>`
	} else {
		twoFactor = `<section class="content-card account-password-card"><h2>` + html.EscapeString(t("two_factor.title")) + `</h2><p>` + html.EscapeString(t("two_factor.status")) + ` : <strong>` + html.EscapeString(t("two_factor.disabled")) + `</strong>.</p><a class="primary-button" href="/go/account/two-factor/setup">` + html.EscapeString(t("two_factor.enable")) + `</a></section>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
		"{{TWO_FACTOR}}", twoFactor,
		"{{LANGUAGE}}", languagePreference,
	).Replace(i18n.Localize(template, language))
	writeHTML(response, page, status)
}

func (a *application) twoFactor(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.pendingTwoFactorUser(response, request)
	if !found {
		return
	}
	a.renderTwoFactor(response, session.CSRFToken, user.ID, false, http.StatusOK)
}

func (a *application) verifyTwoFactor(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.pendingTwoFactorUser(response, request)
	if !found {
		return
	}
	if a.dependencies.TwoFactor == nil || a.dependencies.TwoFactorLimiter == nil {
		http.Error(response, "Service de double authentification indisponible.", http.StatusServiceUnavailable)
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		a.renderTwoFactor(response, session.CSRFToken, user.ID, true, http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderTwoFactor(response, session.CSRFToken, user.ID, true, http.StatusBadRequest)
		return
	}
	address, account := requestAddress(request), user.Login
	if !a.dependencies.TwoFactorLimiter.Allowed(address, account) ||
		!a.dependencies.TwoFactor.Verify(user.TOTPSecret.String, request.PostForm.Get("code")) {
		a.dependencies.TwoFactorLimiter.Failure(address, account)
		a.recordAccess(request, &user, account, "two_factor_failure", false)
		a.renderTwoFactor(response, session.CSRFToken, user.ID, true, http.StatusUnauthorized)
		return
	}
	a.dependencies.TwoFactorLimiter.Success(account)
	a.recordAccess(request, &user, account, "two_factor_success", true)
	state, location := websession.StateAuthenticated, "/go/dashboard"
	if user.MustChangePassword {
		state, location = websession.StatePassword, "/go/account/password"
	}
	rotated, err := a.dependencies.Sessions.Rotate(session.ID, state, user.ID, user.AuthVersion)
	if err != nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.Cookie(rotated.ID))
	http.Redirect(response, request, location, http.StatusSeeOther)
}

func (a *application) pendingTwoFactorUser(response http.ResponseWriter, request *http.Request) (websession.Session, authstore.User, bool) {
	session, found := a.requestSession(request)
	if !found || session.State != websession.StateTwoFactor || a.dependencies.Users == nil {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false
	}
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()
	user, userFound, err := a.dependencies.Users.FindByID(ctx, session.UserID)
	if err != nil {
		http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
		return websession.Session{}, authstore.User{}, false
	}
	if !userFound || user.Status != "active" || user.AuthVersion != session.AuthVersion ||
		!user.TOTPSecret.Valid || !user.TOTPEnabledAt.Valid {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false
	}
	return session, user, true
}

func (a *application) renderTwoFactor(response http.ResponseWriter, csrf string, userID int64, failed bool, status int) {
	language := a.languageForUser(context.Background(), userID)
	errorMessage := ""
	if failed {
		errorMessage = `<p class="error">` + html.EscapeString(i18n.Text(language, "two_factor.error")) + `</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
	).Replace(i18n.Localize(a.twoFactorPage, language))
	writeHTML(response, page, status)
}

func (a *application) accountTwoFactorSetup(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	if user.TOTPEnabledAt.Valid {
		http.Redirect(response, request, "/go/account/password", http.StatusSeeOther)
		return
	}
	if !user.TOTPSecret.Valid {
		secret, err := webauth.GenerateTOTPSecret()
		if err != nil {
			http.Error(response, "La configuration TOTP n’a pas pu être préparée.", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		user, err = a.dependencies.Enrollment.PrepareTOTP(ctx, user.ID, user.AuthVersion, secret)
		if err != nil {
			http.Error(response, "La configuration TOTP n’a pas pu être préparée.", http.StatusServiceUnavailable)
			return
		}
	}
	a.renderTwoFactorSetup(response, session.CSRFToken, user, false, http.StatusOK, "/go/account/two-factor/setup")
}

func (a *application) confirmAccountTwoFactorSetup(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) || !user.TOTPSecret.Valid || !a.dependencies.TwoFactor.Verify(user.TOTPSecret.String, request.PostForm.Get("code")) {
		a.renderTwoFactorSetup(response, session.CSRFToken, user, true, http.StatusBadRequest, "/go/account/two-factor/setup")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	updated, err := a.dependencies.Enrollment.EnableTOTP(ctx, user.ID, user.AuthVersion, user.TOTPSecret.String)
	if err != nil {
		http.Error(response, "La double authentification n’a pas pu être activée.", http.StatusConflict)
		return
	}
	a.recordAccess(request, &updated, updated.Login, "two_factor_enabled", true)
	rotated, err := a.dependencies.Sessions.Rotate(session.ID, websession.StateAuthenticated, updated.ID, updated.AuthVersion)
	if err != nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.Cookie(rotated.ID))
	http.Redirect(response, request, "/go/account/password", http.StatusSeeOther)
}

func (a *application) disableAccountTwoFactor(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.authenticatedUser(response, request)
	if !found {
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) || user.TwoFactorRequired || !user.TOTPSecret.Valid || !user.TOTPEnabledAt.Valid || !a.dependencies.TwoFactor.Verify(user.TOTPSecret.String, request.PostForm.Get("code")) {
		http.Error(response, "La désactivation de la double authentification est invalide.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	updated, err := a.dependencies.Enrollment.DisableTOTP(ctx, user.ID, user.AuthVersion)
	if err != nil {
		http.Error(response, "La double authentification n’a pas pu être désactivée.", http.StatusConflict)
		return
	}
	a.recordAccess(request, &updated, updated.Login, "two_factor_disabled", true)
	rotated, err := a.dependencies.Sessions.Rotate(session.ID, websession.StateAuthenticated, updated.ID, updated.AuthVersion)
	if err != nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.Cookie(rotated.ID))
	http.Redirect(response, request, "/go/account/password", http.StatusSeeOther)
}

func (a *application) twoFactorSetup(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.pendingEnrollmentUser(response, request)
	if !found {
		return
	}
	a.renderTwoFactorSetup(response, session.CSRFToken, user, false, http.StatusOK, "/go/two-factor/setup")
}

func (a *application) confirmTwoFactorSetup(response http.ResponseWriter, request *http.Request) {
	session, user, found := a.pendingEnrollmentUser(response, request)
	if !found {
		return
	}
	if a.dependencies.TwoFactor == nil || a.dependencies.TwoFactorLimiter == nil || a.dependencies.Enrollment == nil {
		http.Error(response, "Service de double authentification indisponible.", http.StatusServiceUnavailable)
		return
	}
	if contentType := request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		a.renderTwoFactorSetup(response, session.CSRFToken, user, true, http.StatusBadRequest, "/go/two-factor/setup")
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderTwoFactorSetup(response, session.CSRFToken, user, true, http.StatusBadRequest, "/go/two-factor/setup")
		return
	}
	address, account := requestAddress(request), user.Login
	if !a.dependencies.TwoFactorLimiter.Allowed(address, account) ||
		!a.dependencies.TwoFactor.Verify(user.TOTPSecret.String, request.PostForm.Get("code")) {
		a.dependencies.TwoFactorLimiter.Failure(address, account)
		a.renderTwoFactorSetup(response, session.CSRFToken, user, true, http.StatusUnauthorized, "/go/two-factor/setup")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	updated, err := a.dependencies.Enrollment.EnableTOTP(ctx, user.ID, user.AuthVersion, user.TOTPSecret.String)
	if err != nil {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return
	}
	a.recordAccess(request, &updated, updated.Login, "two_factor_enabled", true)
	a.dependencies.TwoFactorLimiter.Success(account)
	state, location := websession.StateAuthenticated, "/go/dashboard"
	if updated.MustChangePassword {
		state, location = websession.StatePassword, "/go/account/password"
	}
	rotated, err := a.dependencies.Sessions.Rotate(session.ID, state, updated.ID, updated.AuthVersion)
	if err != nil {
		http.Error(response, "Service de session indisponible.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(response, websession.Cookie(rotated.ID))
	http.Redirect(response, request, location, http.StatusSeeOther)
}

func (a *application) pendingEnrollmentUser(response http.ResponseWriter, request *http.Request) (websession.Session, authstore.User, bool) {
	session, found := a.requestSession(request)
	if !found || session.State != websession.StateTwoFactorSetup || a.dependencies.Users == nil || a.dependencies.Enrollment == nil {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	user, userFound, err := a.dependencies.Users.FindByID(ctx, session.UserID)
	if err != nil {
		http.Error(response, "Service d’authentification indisponible.", http.StatusServiceUnavailable)
		return websession.Session{}, authstore.User{}, false
	}
	if !userFound || user.Status != "active" || user.AuthVersion != session.AuthVersion ||
		!user.TwoFactorRequired || user.TOTPEnabledAt.Valid {
		a.invalidate(response, session.ID)
		http.Redirect(response, request, "/login", http.StatusSeeOther)
		return websession.Session{}, authstore.User{}, false
	}
	if !user.TOTPSecret.Valid {
		secret, secretErr := webauth.GenerateTOTPSecret()
		if secretErr != nil {
			http.Error(response, "La configuration TOTP n’a pas pu être préparée.", http.StatusServiceUnavailable)
			return websession.Session{}, authstore.User{}, false
		}
		user, err = a.dependencies.Enrollment.PrepareTOTP(ctx, user.ID, user.AuthVersion, secret)
		if err != nil {
			a.invalidate(response, session.ID)
			http.Redirect(response, request, "/login", http.StatusSeeOther)
			return websession.Session{}, authstore.User{}, false
		}
	}
	return session, user, true
}

func (a *application) renderTwoFactorSetup(response http.ResponseWriter, csrf string, user authstore.User, failed bool, status int, action string) {
	language := a.languageForUser(context.Background(), user.ID)
	uri, err := webauth.TOTPProvisioningURI(user.Login, user.TOTPSecret.String)
	if err != nil {
		http.Error(response, "La configuration TOTP est invalide.", http.StatusServiceUnavailable)
		return
	}
	image, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		http.Error(response, "Le QR code n’a pas pu être généré.", http.StatusServiceUnavailable)
		return
	}
	errorMessage := ""
	if failed {
		errorMessage = `<p class="error">` + html.EscapeString(i18n.Text(language, "two_factor.setup.error")) + `</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
		"{{SECRET}}", html.EscapeString(user.TOTPSecret.String),
		"{{QRCODE}}", base64.StdEncoding.EncodeToString(image),
		"{{ACTION}}", html.EscapeString(action),
	).Replace(i18n.Localize(a.twoFactorSetupPage, language))
	writeHTML(response, page, status)
}

func (a *application) requestSession(request *http.Request) (websession.Session, bool) {
	if a.dependencies.Sessions == nil {
		return websession.Session{}, false
	}
	cookie, err := request.Cookie(websession.CookieName)
	if err != nil {
		return websession.Session{}, false
	}
	return a.dependencies.Sessions.Get(cookie.Value)
}

func (a *application) renderLogin(request *http.Request, response http.ResponseWriter, csrf string, failed bool, status int) {
	language := a.languageForAnonymous(request)
	errorMessage := ""
	if failed {
		errorMessage = `<p class="error">` + html.EscapeString(i18n.Text(language, "login.invalid")) + `</p>`
	}
	if a.dependencies.Users != nil {
		_, initialized, err := a.dependencies.Users.FindRoot(request.Context())
		if err == nil && !initialized {
			errorMessage = `<div class="notice notice--warning"><strong>` + html.EscapeString(i18n.Text(language, "login.initialization.title")) + `</strong><p>` + i18n.Text(language, "login.initialization.message") + `</p></div>` + errorMessage
		}
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
	).Replace(i18n.Localize(a.loginPage, language))
	writeHTML(response, page, status)
}

func (a *application) defaultLanguage(ctx context.Context) string {
	if a.dependencies.Settings != nil {
		if settings, err := a.dependencies.Settings.Settings(ctx); err == nil && i18n.Supported(settings.DefaultLanguage) {
			return settings.DefaultLanguage
		}
	}
	if i18n.Supported(a.dependencies.DefaultLanguage) {
		return a.dependencies.DefaultLanguage
	}
	return i18n.DefaultLanguage
}

func (a *application) languageForAnonymous(request *http.Request) string {
	if cookie, err := request.Cookie("aegisadmin_language"); err == nil && i18n.Supported(cookie.Value) {
		return cookie.Value
	}
	return a.defaultLanguage(request.Context())
}

func (a *application) languageForUser(ctx context.Context, userID int64) string {
	if a.dependencies.Users != nil {
		if language, err := a.dependencies.Users.LanguageForUser(ctx, userID); err == nil && i18n.Supported(language) {
			return language
		}
	}
	return a.defaultLanguage(ctx)
}

func (a *application) invalidate(response http.ResponseWriter, id string) {
	if a.dependencies.Sessions != nil && id != "" {
		_ = a.dependencies.Sessions.Destroy(id)
	}
	http.SetCookie(response, websession.ExpiredCookie())
}

func (a *application) available() bool {
	return a.dependencies.Sessions != nil && a.dependencies.Authenticator != nil && a.dependencies.LoginLimiter != nil
}

func nextState(step webauth.NextStep) (websession.State, string) {
	switch step {
	case webauth.StepPasswordChange:
		return websession.StatePassword, "/go/account/password"
	case webauth.StepTwoFactorChallenge:
		return websession.StateTwoFactor, "/go/two-factor"
	case webauth.StepTwoFactorEnrollment:
		return websession.StateTwoFactorSetup, "/go/two-factor/setup"
	default:
		return websession.StateAuthenticated, "/go/dashboard"
	}
}

func remoteAddress(value string) string {
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return host
	}
	return value
}

// requestAddress accepts proxy headers only from a reverse proxy connected
// through the local loopback interface. The rightmost X-Forwarded-For value is
// the address appended by that trusted proxy; client-supplied values to its
// left cannot override it.
func requestAddress(request *http.Request) string {
	direct := remoteAddress(request.RemoteAddr)
	peer := net.ParseIP(direct)
	if peer == nil || !peer.IsLoopback() {
		return direct
	}
	forwarded := strings.Split(request.Header.Get("X-Forwarded-For"), ",")
	for index := len(forwarded) - 1; index >= 0; index-- {
		candidate := strings.TrimSpace(forwarded[index])
		if parsed := net.ParseIP(candidate); parsed != nil {
			return parsed.String()
		}
	}
	if candidate := strings.TrimSpace(request.Header.Get("X-Real-IP")); candidate != "" {
		if parsed := net.ParseIP(candidate); parsed != nil {
			return parsed.String()
		}
	}
	return direct
}

func (a *application) recordAccess(request *http.Request, user *authstore.User, login, event string, success bool) {
	if a.dependencies.AccessLog == nil {
		return
	}
	var id *int64
	if user != nil {
		value := user.ID
		id = &value
	}
	address := requestAddress(request)
	agent := strings.ToValidUTF8(request.UserAgent(), "�")
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	_ = a.dependencies.AccessLog.RecordAccess(ctx, id, login, event, success, address, agent)
}

func writeHTML(response http.ResponseWriter, page string, status int) {
	if strings.Contains(page, `class="dashboard-body"`) && !strings.Contains(page, `/assets/app.js`) {
		page = strings.Replace(page, "</body>", `<script src="/assets/app.js?v=`+url.QueryEscape(buildinfo.Version)+`" defer></script></body>`, 1)
	}
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_, _ = response.Write([]byte(page))
}

func projectAsset(contentType string, paths ...string) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			response.Header().Set("Content-Type", contentType)
			response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			_, _ = response.Write(content)
			return
		}
		http.NotFound(response, nil)
	}
}

func ready(checks ReadinessChecks) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		result := healthResponse{Status: "ok", Version: buildinfo.Version, Backend: "ok", Database: "ok"}
		status := http.StatusOK
		if checks.Backend == nil || checks.Backend() != nil {
			result.Status = "unavailable"
			result.Backend = "unavailable"
			status = http.StatusServiceUnavailable
		}
		if checks.Database == nil || checks.Database() != nil {
			result.Status = "unavailable"
			result.Database = "unavailable"
			status = http.StatusServiceUnavailable
		}
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		response.WriteHeader(status)
		_ = json.NewEncoder(response).Encode(result)
	}
}

func health(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(response).Encode(healthResponse{Status: "ok", Version: buildinfo.Version})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if !strings.HasPrefix(request.URL.Path, "/assets/") {
			response.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(response, request)
	})
}

func Server(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}
