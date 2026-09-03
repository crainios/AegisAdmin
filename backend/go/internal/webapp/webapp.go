package webapp

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	LogsSnapshot(context.Context, string) (weblogs.Snapshot, error)
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
	a.renderLogin(response, session.CSRFToken, false, http.StatusOK)
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
		a.renderLogin(response, session.CSRFToken, true, http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderLogin(response, session.CSRFToken, true, http.StatusBadRequest)
		return
	}
	login, password := request.PostForm.Get("login"), request.PostForm.Get("password")
	address := requestAddress(request)
	if len(login) < 3 || len(login) > 64 || password == "" || len(password) > 1024 ||
		!a.dependencies.LoginLimiter.Allowed(address, login) {
		a.dependencies.LoginLimiter.Failure(address, login)
		a.recordAccess(request, nil, login, "login_failure", false)
		a.renderLogin(response, session.CSRFToken, true, http.StatusUnauthorized)
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
		a.renderLogin(response, session.CSRFToken, true, http.StatusUnauthorized)
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
	http.Redirect(response, request, location, http.StatusSeeOther)
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
	page := strings.NewReplacer(
		"{{USER}}", html.EscapeString(user.Login),
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{SYSTEM_INFORMATION}}", renderSystemInformation(snapshot.Information, snapshot.UptimeSeconds),
		"{{RESOURCES}}", renderOverview(filterDashboardCards(snapshot.Cards, false)),
		"{{SUPERVISION}}", renderOverview(filterDashboardCards(snapshot.Cards, true)),
	).Replace(a.dashboardPage)
	writeHTML(response, page, http.StatusOK)
}

func (a *application) dashboardResources(response http.ResponseWriter, request *http.Request) {
	_, _, found := a.authenticatedUser(response, request)
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
	writeJSON(response, cards, http.StatusOK)
}

func (a *application) dashboardSupervision(response http.ResponseWriter, request *http.Request) {
	_, _, found := a.authenticatedUser(response, request)
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
	writeJSON(response, cards, http.StatusOK)
}

func (a *application) updatesSummary(response http.ResponseWriter, request *http.Request) {
	_, _, found := a.authenticatedUser(response, request)
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
	if err != nil {
		writeJSON(response, webupdates.Summary{Success: false, Message: "L’état des mises à jour est indisponible.", Status: "neutral", StatusLabel: "Indisponible", Value: "État indisponible", Subtitle: "La vérification n’a pas pu être effectuée.", URL: "/updates"}, http.StatusServiceUnavailable)
		return
	}
	writeJSON(response, summary, http.StatusOK)
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
	rows := renderProcesses(snapshot.Processes)
	notice := ""
	if snapshot.Truncated {
		notice = `<p class="notice">Seuls les ` + strconv.FormatInt(snapshot.Returned, 10) + ` premiers processus sur ` + strconv.FormatInt(snapshot.Total, 10) + ` sont affichés.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{TOTAL}}", strconv.FormatInt(snapshot.Total, 10), "{{RETURNED}}", strconv.FormatInt(snapshot.Returned, 10), "{{LIMIT}}", strconv.FormatInt(snapshot.Limit, 10), "{{NOTICE}}", notice, "{{PROCESSES}}", rows).Replace(a.processesPage)
	writeHTML(response, page, http.StatusOK)
}

func renderProcesses(processes []webdashboard.Process) string {
	if len(processes) == 0 {
		return `<tr><td colspan="8" class="muted">Aucun processus détecté.</td></tr>`
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
		result.WriteString(`<tr><td data-sort-value="` + strconv.FormatInt(process.PIDValue, 10) + `">` + html.EscapeString(process.PID) + `</td><th data-sort-value="` + html.EscapeString(strings.ToLower(process.Name)) + `">` + name + `</th><td data-sort-value="` + html.EscapeString(strings.ToLower(process.User)) + `">` + html.EscapeString(process.User) + `</td><td data-sort-value="` + html.EscapeString(strings.ToLower(process.StateLabel+" "+process.State)) + `"><span class="status-badge status-badge--` + status + `">` + html.EscapeString(process.StateLabel+" · "+process.State) + `</span></td><td data-sort-value="` + strconv.FormatFloat(process.CPUPercentValue, 'f', -1, 64) + `">` + html.EscapeString(process.CPU) + `</td><td data-sort-value="` + strconv.FormatFloat(process.MemoryPercentValue, 'f', -1, 64) + `">` + html.EscapeString(process.MemoryPercent) + `</td><td data-sort-value="` + strconv.FormatInt(process.MemoryBytes, 10) + `">` + html.EscapeString(process.Memory) + `</td><td data-sort-value="` + strconv.FormatInt(process.ElapsedSeconds, 10) + `">` + html.EscapeString(process.Elapsed) + `</td></tr>`)
	}
	return result.String()
}

func renderSystemInformation(information []webdashboard.Information, uptimeSeconds int64) string {
	if len(information) == 0 {
		return `<p class="muted">Les informations générales du système ne sont pas disponibles.</p>`
	}
	var result strings.Builder
	result.WriteString(`<dl class="detail-grid dashboard-information">`)
	for _, item := range information {
		attributes := ""
		if item.Label == "Durée de fonctionnement" {
			attributes = ` data-dashboard-uptime data-dashboard-uptime-seconds="` + strconv.FormatInt(uptimeSeconds, 10) + `"`
		}
		result.WriteString(`<div><dt>` + html.EscapeString(item.Label) + `</dt><dd` + attributes + `>` + html.EscapeString(item.Value) + `</dd></div>`)
	}
	result.WriteString(`<div class="dashboard-information__updates" data-dashboard-updates data-dashboard-updates-url="/updates/summary" aria-live="polite" aria-busy="true"><dt>Mises à jour</dt><dd><span data-dashboard-updates-value>Vérification en cours…</span><a href="/updates" data-dashboard-updates-link hidden>Consulter les mises à jour</a></dd></div>`)
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
	type menuModule struct{ Name, Icon, Route string }
	type menuCategory struct {
		Name    string
		Modules []menuModule
	}
	result := make([]menuCategory, 0, len(categories))
	for _, category := range categories {
		item := menuCategory{Name: category.Name, Modules: []menuModule{}}
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
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabel(level)),
		"{{SUMMARY}}", renderStorageSummary(snapshot.Summary),
		"{{MOUNTS}}", renderStorageMounts(snapshot.Mounts),
	).Replace(a.storagePage)
	writeHTML(response, page, http.StatusOK)
}

func renderStorageSummary(summary webstorage.Summary) string {
	items := []struct {
		label string
		value int
	}{
		{"Volumes", summary.Total}, {"État normal", summary.Normal},
		{"À surveiller", summary.Warning}, {"Critiques", summary.Danger},
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

func renderStorageMounts(mounts []webstorage.Mount) string {
	if len(mounts) == 0 {
		return `<tr><td colspan="9" class="muted">Aucun volume n’a été détecté.</td></tr>`
	}
	var result strings.Builder
	for _, mount := range mounts {
		status := mount.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		physicalDevice := mount.PhysicalDevice
		if physicalDevice == "" {
			physicalDevice = "Indéterminé"
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
		result.WriteString(html.EscapeString(strings.ToLower(mount.StatusLabel)))
		result.WriteString(`"><span class="status-badge status-badge--`)
		result.WriteString(status)
		result.WriteString(`">`)
		result.WriteString(html.EscapeString(mount.StatusLabel))
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
	canAct := level == "action" || level == "modify"
	notice := ""
	if request.URL.Query().Get("result") == "restarted" {
		notice = `<p class="notice notice--success">Le service a été redémarré.</p>`
	}
	actionHeader := ""
	if canAct {
		actionHeader = "<th>Action</th>"
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabel(level)),
		"{{NOTICE}}", notice,
		"{{SUMMARY}}", renderServicesSummary(snapshot.Summary),
		"{{ACTION_HEADER}}", actionHeader,
		"{{SERVICES}}", renderServices(snapshot.Services, session.CSRFToken, canAct),
	).Replace(a.servicesPage)
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

func renderServicesSummary(summary webservices.Summary) string {
	items := []struct {
		label string
		value int
	}{
		{"Services autorisés", summary.Total}, {"Installés", summary.Installed},
		{"Actifs", summary.Active}, {"Inactifs", summary.Inactive}, {"Absents", summary.Missing},
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

func renderServices(services []webservices.Service, csrf string, canAct bool) string {
	columns := 5
	if canAct {
		columns++
	}
	if len(services) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">Aucun service autorisé n’a été retourné.</td></tr>`
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
				result.WriteString(`"><button class="danger-button" type="submit">Redémarrer</button></form>`)
			}
			result.WriteString(`</td>`)
		}
		result.WriteString(`<th scope="row">`)
		result.WriteString(html.EscapeString(service.ID))
		result.WriteString(`</th><td><span class="status-badge status-badge--` + status + `">`)
		result.WriteString(html.EscapeString(service.StatusLabel))
		result.WriteString(`</span></td><td>` + yesNo(service.Exists) + `</td><td>` + yesNo(service.Enabled) + `</td><td>`)
		result.WriteString(html.EscapeString(service.State))
		result.WriteString(`</td>`)
		result.WriteString(`</tr>`)
	}
	return result.String()
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
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabel(level)),
		"{{SUMMARY}}", renderNetworkSummary(snapshot.Summary),
		"{{INTERFACES}}", renderNetworkInterfaces(snapshot.Interfaces, snapshot.Selected),
		"{{DETAILS}}", renderNetworkDetails(snapshot.Selected),
	).Replace(a.networkPage)
	writeHTML(response, page, http.StatusOK)
}

func renderNetworkSummary(summary webnetwork.Summary) string {
	items := []struct {
		label string
		value int
	}{{"Interfaces détectées", summary.Total}, {"Actives", summary.Up}, {"Arrêtées", summary.Down}, {"État inconnu", summary.Unknown}}
	var result strings.Builder
	for _, item := range items {
		result.WriteString(`<article class="summary-item"><span>` + item.label + `</span><strong>` + strconv.Itoa(item.value) + `</strong></article>`)
	}
	return result.String()
}

func renderNetworkInterfaces(interfaces []webnetwork.Interface, selected *webnetwork.Interface) string {
	if len(interfaces) == 0 {
		return `<tr><td colspan="4" class="muted">Aucune interface réseau n’a été détectée.</td></tr>`
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
			result.WriteString(`<span class="network-selected-button" aria-current="true">Sélectionnée</span>`)
		} else {
			result.WriteString(`<a class="secondary-link" href="/network?interface=` + url.QueryEscape(item.ID) + `">Afficher</a>`)
		}
		result.WriteString(`</td><th scope="row">` + html.EscapeString(item.Name) + `</th><td>` + html.EscapeString(item.TypeLabel) + `</td><td><span class="status-badge status-badge--` + status + `">` + html.EscapeString(item.StatusLabel) + `</span></td></tr>`)
	}
	return result.String()
}

func renderNetworkDetails(item *webnetwork.Interface) string {
	if item == nil {
		return ""
	}
	mac, mtu := "Non disponible", "Non disponible"
	if item.MAC != nil {
		mac = *item.MAC
	}
	if item.MTU != nil {
		mtu = strconv.Itoa(*item.MTU)
	}
	return `<section class="detail-panel"><h2>Détails de ` + html.EscapeString(item.Name) + `</h2><dl class="detail-grid"><div><dt>Type</dt><dd>` + html.EscapeString(item.TypeLabel) + `</dd></div><div><dt>Adresse MAC</dt><dd>` + html.EscapeString(mac) + `</dd></div><div><dt>MTU</dt><dd>` + html.EscapeString(mtu) + `</dd></div><div><dt>IPv4</dt><dd>` + renderAddresses(item.IPv4) + `</dd></div><div><dt>IPv6</dt><dd>` + renderAddresses(item.IPv6) + `</dd></div></dl></section>`
}

func renderAddresses(addresses []webnetwork.Address) string {
	if len(addresses) == 0 {
		return "Aucune"
	}
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, html.EscapeString(address.Address)+"/"+strconv.Itoa(address.Prefix))
	}
	return strings.Join(values, "<br>")
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
	ctx, cancel := context.WithTimeout(request.Context(), 12*time.Second)
	defer cancel()
	if requested == "" && a.dependencies.Settings != nil {
		if settings, settingsErr := a.dependencies.Settings.Settings(ctx); settingsErr == nil {
			requested = strings.TrimSpace(settings.DefaultLog)
		}
	}
	snapshot, err := a.dependencies.Logs.LogsSnapshot(ctx, requested)
	if err != nil {
		http.Error(response, "Les journaux n’ont pas pu être chargés.", http.StatusServiceUnavailable)
		return
	}
	lines := filterLogLines(snapshot.Lines, logLevel, keyword)
	lineContent := strings.Join(lines, "\n")
	if len(lines) == 0 {
		lineContent = "Aucune ligne ne correspond aux critères de recherche."
	}
	results := strconv.Itoa(len(lines)) + " ligne(s) affichée(s) sur les 100 dernières"
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(session.CSRFToken),
		"{{PERMISSION}}", html.EscapeString(permissionLabel(level)),
		"{{SOURCES}}", renderLogSources(snapshot.Sources, snapshot.Selected),
		"{{LEVELS}}", renderLogLevels(logLevel),
		"{{KEYWORD}}", html.EscapeString(keyword),
		"{{SELECTED}}", html.EscapeString(logTitle(snapshot.Selected)),
		"{{RESET_URL}}", html.EscapeString("/logs?source="+url.QueryEscape(snapshot.Selected)),
		"{{RESULTS}}", html.EscapeString(results),
		"{{LINES}}", html.EscapeString(lineContent),
	).Replace(a.logsPage)
	writeHTML(response, page, http.StatusOK)
}

func renderLogLevels(selectedLevel string) string {
	options := [][2]string{{"", "Tous les événements"}, {"error", "Erreurs"}, {"warning", "Avertissements"}, {"info", "Informations"}}
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

func renderLogSources(sources []string, selected string) string {
	if len(sources) == 0 {
		return `<option value="">Aucun journal disponible</option>`
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

func logTitle(selected string) string {
	if selected == "" {
		return "Aucun journal sélectionné"
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
	page := strings.ReplaceAll(a.aboutPage, "{{CSRF}}", html.EscapeString(session.CSRFToken))
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
	canAct := level == "action" || level == "modify"
	notice := ""
	if request.URL.Query().Get("result") == "restarted" {
		notice = `<p class="notice notice--success">Le redémarrage de PHP-FPM a été programmé.</p>`
	}
	actionHeader := ""
	if canAct {
		actionHeader = "<th>Action</th>"
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice,
		"{{CLI_VERSION}}", html.EscapeString(snapshot.CLI.Version), "{{CLI_SAPI}}", html.EscapeString(snapshot.CLI.SAPI), "{{CLI_INI}}", html.EscapeString(snapshot.CLI.IniFile), "{{CLI_SCAN}}", html.EscapeString(snapshot.CLI.ScanDir),
		"{{ACTION_HEADER}}", actionHeader, "{{INSTANCES}}", renderPHPInstances(snapshot.Instances, session.CSRFToken, canAct)).Replace(a.phpPage)
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

func renderPHPInstances(instances []webphp.Instance, csrf string, canAct bool) string {
	columns := 5
	if canAct {
		columns++
	}
	if len(instances) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">Aucune instance PHP-FPM détectée.</td></tr>`
	}
	var result strings.Builder
	for _, item := range instances {
		status := item.Status
		if status != "success" && status != "warning" && status != "danger" {
			status = "neutral"
		}
		result.WriteString(`<tr><th>` + html.EscapeString(item.Version) + `</th><td>` + html.EscapeString(item.Service) + `</td><td><span class="status-badge status-badge--` + status + `">` + html.EscapeString(item.StatusLabel) + `</span></td><td>` + yesNo(item.Enabled) + `</td><td>` + html.EscapeString(item.State) + `</td>`)
		if canAct {
			result.WriteString(`<td>`)
			if item.Exists {
				result.WriteString(`<form class="inline-form" method="post" action="/php/restart"><input type="hidden" name="_token" value="` + html.EscapeString(csrf) + `"><input type="hidden" name="runtime" value="` + html.EscapeString(item.ID) + `"><button class="danger-button" type="submit">Redémarrer</button></form>`)
			}
			result.WriteString(`</td>`)
		}
		result.WriteString(`</tr>`)
	}
	return result.String()
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
	canAct := level == "action" || level == "modify"
	restart := ""
	if canAct && snapshot.Server.Service.Exists {
		restart = `<form method="post" action="/mysql/restart"><input type="hidden" name="_token" value="` + html.EscapeString(session.CSRFToken) + `"><button class="danger-button" type="submit">Redémarrer ` + html.EscapeString(snapshot.Server.Product) + `</button></form>`
	}
	notice := ""
	if request.URL.Query().Get("result") == "restarted" {
		notice = `<p class="notice notice--success">Le redémarrage du serveur de bases de données a été programmé.</p>`
	}
	for _, warning := range snapshot.Warnings {
		notice += `<p class="notice notice--warning">` + html.EscapeString(warning) + `</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{RESTART}}", restart, "{{SERVER}}", renderMySQLServer(snapshot.Server), "{{METRICS}}", renderMySQLMetrics(snapshot.Metrics), "{{DATABASES}}", renderMySQLDatabases(snapshot.Databases)).Replace(a.mysqlPage)
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
func renderMySQLServer(s webmysql.Server) string {
	available := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return "Indisponible"
		}
		return value
	}
	unit := "Non détecté"
	if s.Service.Unit != nil {
		unit = *s.Service.Unit
	}
	version := available(strings.TrimSpace(s.Product + " " + s.Version))
	port := "Indisponible"
	if s.Port > 0 {
		port = strconv.FormatInt(s.Port, 10)
	}
	rows := [][2]string{{"Version", version}, {"Nom d’hôte", available(s.Hostname)}, {"Port", port}, {"Socket", available(s.Socket)}, {"Répertoire des données", available(s.DataDirectory)}, {"Moteur par défaut", available(s.DefaultStorageEngine)}, {"Service", unit}, {"Démarrage automatique", yesNo(s.Service.Enabled)}, {"État du service", available(s.Service.State)}}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(`<div><dt>` + r[0] + `</dt><dd>` + html.EscapeString(r[1]) + `</dd></div>`)
	}
	return b.String()
}
func renderMySQLMetrics(m map[string]int64) string {
	metric := func(key string, format func(int64) string) string {
		value, found := m[key]
		if !found {
			return "Indisponible"
		}
		return format(value)
	}
	decimal := func(value int64) string { return strconv.FormatInt(value, 10) }
	items := [][2]string{{"Connexions actives", metric("threads_connected", decimal)}, {"Threads actifs", metric("threads_running", decimal)}, {"Requêtes", metric("queries", formatFrenchInteger)}, {"Requêtes lentes", metric("slow_queries", decimal)}}
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
func renderMySQLDatabases(items []webmysql.Database) string {
	if len(items) == 0 {
		return `<tr><td colspan="4" class="muted">Aucune base détectée.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		b.WriteString(`<tr><th data-sort-value="` + html.EscapeString(strings.ToLower(i.Name)) + `">` + html.EscapeString(i.Name) + `</th><td data-sort-value="` + html.EscapeString(strings.ToLower(i.Kind)) + `">` + html.EscapeString(i.Kind) + `</td><td data-sort-value="` + strconv.FormatInt(i.Tables, 10) + `">` + strconv.FormatInt(i.Tables, 10) + `</td><td data-sort-value="` + strconv.FormatInt(i.Size, 10) + `">` + formatByteCount(i.Size) + `</td></tr>`)
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
	if !s.Installed {
		info := renderPairs([][2]string{{"État", "Tor n’est pas installé sur ce serveur."}})
		config := `<span class="status-badge status-badge--neutral">Indisponible</span><p>Installez Tor pour accéder à sa configuration.</p>`
		status := renderMetricPairs([][2]string{{"Amorçage", "Indisponible"}, {"Mémoire", "Indisponible"}, {"Tâches", "Indisponible"}, {"PID principal", "Indisponible"}})
		page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", `<p class="notice">Le module reste disponible, mais Tor n’est pas installé sur ce serveur.</p>`, "{{INFO}}", info, "{{CONFIG}}", config, "{{ACTIONS}}", "", "{{INSTANCE_TITLE}}", "Service Tor", "{{STATUS}}", status, "{{ONIONS}}", renderOnions(nil)).Replace(a.torPage)
		writeHTML(response, page, http.StatusOK)
		return
	}
	canAct := level == "action" || level == "modify"
	actions := ""
	if canAct && s.Status.Exists {
		actions = `<div class="header-actions"><form method="post" action="/tor/reload"><input type="hidden" name="_token" value="` + html.EscapeString(session.CSRFToken) + `"><button class="primary-button" type="submit">Recharger Tor</button></form><form method="post" action="/tor/restart"><input type="hidden" name="_token" value="` + html.EscapeString(session.CSRFToken) + `"><button class="danger-button" type="submit">Redémarrer Tor</button></form></div>`
	}
	notice := ""
	if result := request.URL.Query().Get("result"); result == "reload" || result == "restart" {
		notice = `<p class="notice notice--success">L’action Tor a été exécutée.</p>`
	}
	config := `<span class="status-badge status-badge--danger">Invalide</span>`
	if s.ConfigurationValid {
		config = `<span class="status-badge status-badge--success">Valide</span><p>` + html.EscapeString(s.ConfigurationMessage) + `</p>`
	}
	bootstrap := "Indisponible"
	if s.Status.Bootstrap != nil {
		bootstrap = strconv.Itoa(*s.Status.Bootstrap) + " %"
	}
	info := renderPairs([][2]string{{"Version", s.Info.Product + " " + s.Info.Version}, {"Configuration", s.Info.ConfigFile}, {"Service", s.Info.Service}, {"Unité", s.Info.Unit}})
	status := renderMetricPairs([][2]string{{"Amorçage", bootstrap}, {"Mémoire", formatByteCount(s.Status.Memory)}, {"Tâches", strconv.FormatInt(s.Status.Tasks, 10)}, {"PID principal", strconv.FormatInt(s.Status.MainPID, 10)}})
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{INFO}}", info, "{{CONFIG}}", config, "{{ACTIONS}}", actions, "{{INSTANCE_TITLE}}", "Instance "+html.EscapeString(s.Info.Service), "{{STATUS}}", status, "{{ONIONS}}", renderOnions(s.Services)).Replace(a.torPage)
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
func renderOnions(items []webtor.Onion) string {
	if len(items) == 0 {
		return `<tr><td colspan="3" class="muted">Aucun service Onion détecté.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		host := "Non disponible"
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
	if !s.Installed && s.Version == "" {
		page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", `<p class="notice">Apache n’est pas installé sur ce serveur. AegisAdmin utilise directement son serveur HTTPS Go.</p>`, "{{INFO}}", renderPairs([][2]string{{"Installation", "Non installé"}}), "{{CONFIG}}", `<span class="status-badge status-badge--neutral">Indisponible</span><p>Installez Apache pour gérer ses VirtualHosts.</p>`, "{{ACTIONS}}", "", "{{SUMMARY}}", renderMetricPairs([][2]string{{"VirtualHosts", "0"}, {"Sites", "0"}, {"Modules", "0"}, {"Sites actifs", "0"}}), "{{VHOSTS}}", renderVHosts(nil), "{{SITE_ACTIONS}}", renderApacheSites(nil, session.CSRFToken, false), "{{CREATE_SITE}}", "").Replace(a.apachePage)
		writeHTML(response, page, http.StatusOK)
		return
	}
	canAct := level == "action" || level == "modify"
	canModify := level == "modify"
	actions := ""
	if canAct {
		token := html.EscapeString(session.CSRFToken)
		actions = `<div class="header-actions"><form method="post" action="/apache/reload"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button" type="submit">Recharger Apache</button></form><form method="post" action="/apache/restart"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button" type="submit">Redémarrer Apache</button></form></div>`
	}
	configClass := "danger"
	configLabel := "Invalide"
	if s.ConfigValid {
		configClass = "success"
		configLabel = "Valide"
	}
	config := `<span class="status-badge status-badge--` + configClass + `">` + configLabel + `</span><p>` + html.EscapeString(s.ConfigMessage) + `</p>`
	notice := ""
	if action := request.URL.Query().Get("result"); action == "reload" || action == "restart" {
		notice = `<p class="notice notice--success">L’action Apache a été exécutée.</p>`
	} else if action != "" {
		notice = `<p class="notice notice--success">L’opération Apache a été exécutée.</p>`
	} else if message := request.URL.Query().Get("error"); message != "" {
		notice = `<p class="notice notice--danger">` + html.EscapeString(message) + `</p>`
	}
	summary := renderMetricPairs([][2]string{{"VirtualHosts", strconv.Itoa(len(s.VHosts))}, {"Sites", strconv.Itoa(len(s.Sites))}, {"Modules", strconv.Itoa(len(s.Modules))}, {"Sites actifs", strconv.Itoa(enabledSites(s.Sites))}})
	page := strings.NewReplacer("{{CSRF}}", html.EscapeString(session.CSRFToken), "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{"Version", s.Version}, {"Compilation", s.Built}}), "{{CONFIG}}", config, "{{ACTIONS}}", actions, "{{SUMMARY}}", summary, "{{VHOSTS}}", renderVHosts(s.VHosts), "{{SITE_ACTIONS}}", renderApacheSites(s.Sites, session.CSRFToken, canModify), "{{CREATE_SITE}}", renderApacheCreate(session.CSRFToken, canModify)).Replace(a.apachePage)
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

func renderApacheSites(items []webapache.Site, token string, canModify bool) string {
	if len(items) == 0 {
		return `<tr><td colspan="6" class="muted">Aucun site Apache détecté.</td></tr>`
	}
	var b strings.Builder
	for _, site := range items {
		state, stateClass := "Désactivé", "muted"
		if site.Enabled {
			state, stateClass = "Activé", "success"
		}
		actions := `<span class="muted">Lecture seule</span>`
		if canModify {
			actions = `<select class="compact-select" aria-label="Action pour ` + html.EscapeString(site.Filename) + `" data-apache-site-action data-config-id="` + html.EscapeString(site.ConfigID) + `" data-filename="` + html.EscapeString(site.Filename) + `" data-domains="` + html.EscapeString(strings.Join(site.ServerNames, " ")) + `" data-csrf="` + html.EscapeString(token) + `"><option value="">Actions…</option><option value="edit">Modifier</option>`
			if site.Enabled {
				actions += `<option value="disable">Désactiver</option>`
			} else {
				actions += `<option value="enable">Activer</option>`
			}
			if len(site.ServerNames) > 0 {
				actions += `<option value="certificate">Créer un certificat TLS</option>`
			}
			actions += `<option value="delete">Supprimer</option></select>`
		}
		ports := make([]string, len(site.Ports))
		for i, port := range site.Ports {
			ports[i] = strconv.Itoa(port)
		}
		b.WriteString(`<tr><td>` + actions + `</td><th>` + html.EscapeString(site.Filename) + `</th><td><span class="status-badge status-badge--` + stateClass + `">` + state + `</span></td><td>` + html.EscapeString(strings.Join(site.ServerNames, ", ")) + `</td><td>` + html.EscapeString(strings.Join(ports, ", ")) + `</td><td>` + strconv.FormatInt(site.Size, 10) + ` octets</td></tr>`)
	}
	return b.String()
}

func renderApacheCreate(token string, canModify bool) string {
	if !canModify {
		return ""
	}
	return `<button class="primary-button" type="button" data-apache-create-open>Ajouter un site</button>
<dialog class="action-dialog action-dialog--wide" data-apache-create-dialog><form method="post" action="/apache/create"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><h2>Ajouter un site Apache</h2><label>Type<select name="site_type" data-apache-site-type><option value="website">Site web</option><option value="proxy">Proxy inverse</option></select></label><label>Nom de domaine<input name="server_name" required placeholder="example.org"></label><label>Alias de domaine<input name="aliases" placeholder="www.example.org"></label><fieldset data-apache-website-fields><label>DocumentRoot<input name="document_root" value="/var/www/" required></label><label>AllowOverride<select name="allow_override"><option value="None">None</option><option value="All">All</option></select></label><label class="check-row"><input type="checkbox" name="follow_sym_links" value="1" checked> Autoriser FollowSymLinks</label></fieldset><fieldset data-apache-proxy-fields hidden><label>URL cible<input type="url" name="target_url" placeholder="http://127.0.0.1:3000"></label></fieldset><div class="dialog-actions"><button class="secondary-button" type="button" data-apache-create-close>Annuler</button><button class="primary-button" type="submit">Créer et valider</button></div></form></dialog>
<dialog class="action-dialog action-dialog--wide" data-apache-edit-dialog><form method="post" action="/apache/update"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><input type="hidden" name="config_id"><h2>Modifier le VirtualHost</h2><p class="muted" data-apache-edit-filename></p><label>Configuration<textarea name="content" rows="20" required spellcheck="false"></textarea></label><div class="dialog-actions"><button class="secondary-button" type="button" data-apache-edit-close>Annuler</button><button class="primary-button" type="submit">Valider et enregistrer</button></div></form></dialog>
<dialog class="action-dialog" data-apache-certificate-dialog><form method="post" action="/apache/issue-certificate"><input type="hidden" name="_token" value="` + html.EscapeString(token) + `"><h2>Créer un certificat TLS</h2><label>Domaines<input name="domains" required></label><label>Adresse e-mail<input type="email" name="email" required></label><label class="check-row"><input type="checkbox" name="redirect" value="1" checked> Rediriger HTTP vers HTTPS</label><div class="dialog-actions"><button class="secondary-button" type="button" data-apache-certificate-close>Annuler</button><button class="primary-button" type="submit">Lancer Certbot</button></div></form></dialog>`
}
func renderVHosts(items []webapache.VHost) string {
	if len(items) == 0 {
		return `<tr><td colspan="4" class="muted">Aucun VirtualHost détecté.</td></tr>`
	}
	var b strings.Builder
	for _, i := range items {
		root := "Non défini"
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
		page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", `<p class="notice">Le module reste disponible, mais Fail2ban n’est pas installé sur ce serveur.</p>`, "{{INFO}}", renderPairs([][2]string{{"État", "Fail2ban n’est pas installé sur ce serveur."}, {"Démarrage automatique", "Non"}, {"Prisons", "0"}}), "{{SERVICE_ACTIONS}}", "", "{{CONFIG}}", `<span class="status-badge status-badge--neutral">Indisponible</span><p>Installez Fail2ban pour accéder à sa configuration.</p>`, "{{JAIL_SELECTOR}}", renderJailSelector(nil, ""), "{{JAIL}}", "", "{{MODIFY_ACTIONS}}", "").Replace(a.fail2banPage)
		writeHTML(response, page, http.StatusOK)
		return
	}
	serviceActions := ""
	if level == "action" || level == "modify" {
		serviceActions = `<div class="header-actions"><form method="post" action="/fail2ban/reload"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">Recharger</button></form><form method="post" action="/fail2ban/restart"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button">Redémarrer</button></form></div>`
	}
	selector := renderJailSelector(s.Status.Jails, s.Selected)
	jail, modify := renderJail(s, token, level == "modify")
	configClass := "danger"
	if s.ConfigValid {
		configClass = "success"
	}
	notice := ""
	if request.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">L’action Fail2ban a été exécutée.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{"Version", s.Info.Product + " " + s.Info.Version}, {"État", s.Status.State}, {"Démarrage automatique", yesNo(s.Status.Enabled)}, {"Prisons", strconv.Itoa(len(s.Status.Jails))}}), "{{SERVICE_ACTIONS}}", serviceActions, "{{CONFIG}}", `<span class="status-badge status-badge--`+configClass+`">`+html.EscapeString(s.ConfigMessage)+`</span>`, "{{JAIL_SELECTOR}}", selector, "{{JAIL}}", jail, "{{MODIFY_ACTIONS}}", modify).Replace(a.fail2banPage)
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
func renderJailSelector(jails []string, selected string) string {
	if len(jails) == 0 {
		return `<p class="muted">Aucune prison disponible.</p>`
	}
	var b strings.Builder
	b.WriteString(`<form class="selector-form" method="get" action="/fail2ban"><label>Prison</label><select name="jail">`)
	for _, j := range jails {
		b.WriteString(`<option value="` + html.EscapeString(j) + `"`)
		if j == selected {
			b.WriteString(` selected`)
		}
		b.WriteString(`>` + html.EscapeString(j) + `</option>`)
	}
	b.WriteString(`</select><button class="primary-button">Afficher</button></form>`)
	return b.String()
}
func renderJail(s webfail2ban.Snapshot, token string, modify bool) (string, string) {
	if s.Jail == nil {
		return "", ""
	}
	j := s.Jail
	details := renderMetricPairs([][2]string{{"Échecs actuels", strconv.FormatInt(j.CurrentlyFailed, 10)}, {"Échecs totaux", strconv.FormatInt(j.TotalFailed, 10)}, {"Bannis actuels", strconv.FormatInt(j.CurrentlyBanned, 10)}, {"Bannis totaux", strconv.FormatInt(j.TotalBanned, 10)}})
	var actions strings.Builder
	actions.WriteString(`<h3>Adresses bannies</h3>`)
	if len(j.BannedIPs) == 0 {
		actions.WriteString(`<p class="muted">Aucune adresse IP n’est actuellement bannie dans cette prison.</p>`)
	}
	for _, ip := range j.BannedIPs {
		if modify {
			actions.WriteString(`<form class="fail2ban-banned-row" method="post" action="/fail2ban/unban"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="jail" value="` + html.EscapeString(s.Selected) + `"><input type="hidden" name="address" value="` + html.EscapeString(ip) + `"><button class="danger-button">Débannir</button><code>` + html.EscapeString(ip) + `</code></form>`)
		} else {
			actions.WriteString(`<div class="fail2ban-banned-row"><code>` + html.EscapeString(ip) + `</code></div>`)
		}
	}
	if modify {
		actions.WriteString(`<h3>Bannir une adresse</h3><form class="selector-form" method="post" action="/fail2ban/ban"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="jail" value="` + html.EscapeString(s.Selected) + `"><label>Adresse IP</label><input name="address" required><button class="danger-button">Bannir</button></form>`)
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
			reload = `<div class="header-actions"><form method="post" action="/firewall/reload"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">Recharger le pare-feu</button></form><form method="post" action="/firewall/disable" data-firewall-state-form data-firewall-state="disable"><input type="hidden" name="_token" value="` + token + `"><button class="danger-button">Désactiver le pare-feu</button></form></div>`
		} else {
			reload = `<form method="post" action="/firewall/enable" data-firewall-state-form data-firewall-state="enable"><input type="hidden" name="_token" value="` + token + `"><button class="primary-button">Activer le pare-feu</button></form>`
		}
	}
	add := ""
	header := ""
	if level == "modify" {
		header = "<th>Action</th>"
		add = `<article class="content-card"><h2>Ajouter une règle entrante</h2><form class="selector-form" method="post" action="/firewall/add"><input type="hidden" name="_token" value="` + token + `"><label>Action</label><select name="rule_action"><option>allow</option><option>deny</option><option>reject</option><option>limit</option></select><label>Ports</label><input name="ports" required><label>Protocole</label><select name="protocol"><option>tcp</option><option>udp</option></select><label>Source</label><input name="source" value="any" required><button class="danger-button">Ajouter la règle</button></form></article>`
	}
	version := "Inconnue"
	if s.Info.Version != nil {
		version = *s.Info.Version
	}
	notice := ""
	if message := request.URL.Query().Get("error"); message != "" {
		notice = `<p class="notice notice--danger">` + html.EscapeString(message) + `</p>`
	} else if request.URL.Query().Get("result") != "" {
		notice = `<p class="notice notice--success">L’action du pare-feu a été programmée.</p>`
	}
	page := strings.NewReplacer("{{CSRF}}", token, "{{PERMISSION}}", html.EscapeString(permissionLabel(level)), "{{NOTICE}}", notice, "{{INFO}}", renderPairs([][2]string{{"Produit", s.Info.Product + " " + version}, {"Actif", yesNo(s.Info.Active)}, {"IPv6", optionalBool(s.Info.IPv6)}, {"Entrant par défaut", s.Info.DefaultIncoming}, {"Sortant par défaut", s.Info.DefaultOutgoing}}), "{{RELOAD}}", reload, "{{ADD}}", add, "{{ACTION_HEADER}}", header, "{{RULES}}", renderFirewallRules(s.Rules, token, level == "modify")).Replace(a.firewallPage)
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
func renderFirewallRules(items []webfirewall.Rule, token string, modify bool) string {
	return renderFirewallRulesWithServices(items, token, modify, readFirewallServices("/etc/services"))
}

func renderFirewallRulesWithServices(items []webfirewall.Rule, token string, modify bool, services map[string]string) string {
	columns := 7
	if modify {
		columns++
	}
	if len(items) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">Aucune règle.</td></tr>`
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
			b.WriteString(`<td><form class="inline-form" method="post" action="/firewall/delete"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="id" value="` + strconv.Itoa(i.ID) + `"><button class="danger-button">Supprimer</button></form></td>`)
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
	token := html.EscapeString(session.CSRFToken)
	create := ""
	header := ""
	if level == "action" || level == "modify" {
		header = "<th>Actions</th>"
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
		create = renderBackupLibrary(token, options.String())
	}
	notice := ""
	if execution := request.URL.Query().Get("execution"); execution != "" {
		notice = `<p class="notice notice--success">Exécution Cron programmée. Le résultat va s’afficher automatiquement.</p>`
	} else if result := request.URL.Query().Get("result"); result != "" {
		notice = `<p class="notice notice--success">L’action Cron a été exécutée.</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", token,
		"{{PERMISSION}}", html.EscapeString(permissionLabel(level)),
		"{{NOTICE}}", notice,
		"{{INFO}}", renderPairs([][2]string{{"Version", snapshot.Info.Product + " " + snapshot.Info.Version}, {"Service", snapshot.Info.Service}, {"Unité systemd", snapshot.Info.Unit}, {"Anacron", yesNo(snapshot.Info.AnacronAvailable)}}),
		"{{STATUS}}", renderPairs([][2]string{{"Installé", yesNo(snapshot.Status.Exists)}, {"Actif", yesNo(snapshot.Status.Active)}, {"Activé au démarrage", yesNo(snapshot.Status.Enabled)}, {"État", snapshot.Status.State}, {"PID principal", strconv.FormatInt(snapshot.Status.MainPID, 10)}, {"Mémoire", formatByteCount(snapshot.Status.MemoryBytes)}, {"Tâches du service", strconv.FormatInt(snapshot.Status.Tasks, 10)}}),
		"{{CREATE}}", create,
		"{{ACTION_HEADER}}", header,
		"{{JOBS}}", renderCronJobs(snapshot.Jobs, token, level),
	).Replace(a.cronPage)
	writeHTML(response, page, http.StatusOK)
}

func renderBackupLibrary(token, userOptions string) string {
	months := []string{"Jan", "Fév", "Mar", "Avr", "Mai", "Juin", "Juil", "Août", "Sep", "Oct", "Nov", "Déc"}
	weekdays := []string{"Dim", "Lun", "Mar", "Mer", "Jeu", "Ven", "Sam"}
	return `<section class="content-card backup-library"><h2>Bibliothèque de tâches Cron</h2><p class="muted">Choisissez un modèle ou créez une tâche personnalisée.</p><div class="cron-library-actions"><label class="library-selector">Type de tâche<select data-backup-template-select><option value="">Sélectionner un modèle</option><optgroup label="Sauvegardes"><option value="mysql">MySQL / MariaDB</option><option value="apache">Configuration Apache</option><option value="sites">Sites de /var/www</option></optgroup></select></label><button class="secondary-button" type="button" data-cron-create-open>Créer une tâche utilisateur</button></div>` +
		`<dialog class="action-dialog action-dialog--wide" data-cron-create-dialog><form method="post" action="/cron/create" data-cron-create-form><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="task_id" data-cron-task-id><input type="hidden" name="schedule" value="0 2 * * *" data-cron-schedule><h2 data-cron-form-title>Créer une tâche utilisateur</h2><label>Utilisateur<select name="user" required data-cron-user>` + userOptions + `</select></label><fieldset class="cron-schedule-builder"><legend>Périodicité</legend><label>Mode<select data-cron-mode><option value="visual">Sélection interactive</option><option value="custom">Expression avancée</option></select></label><div class="cron-choice-groups" data-cron-visual>` + cronChoiceGroup("Minutes", "minute", 0, 59, nil, []int{0}) + cronChoiceGroup("Heures", "hour", 0, 23, nil, []int{2}) + cronChoiceGroup("Jours du mois", "monthday", 1, 31, nil, nil) + cronChoiceGroup("Mois", "month", 1, 12, months, nil) + cronChoiceGroup("Jours de la semaine", "weekday", 0, 6, weekdays, nil) + `</div><label data-cron-custom-field hidden>Expression Cron<input value="0 2 * * *" data-cron-custom></label><p class="muted" data-cron-day-warning hidden>Lorsque les jours du mois et de la semaine sont tous deux limités, Cron exécute généralement la tâche si l’un des deux critères correspond.</p><p class="cron-schedule-preview">Expression générée : <code data-cron-schedule-preview>0 2 * * *</code></p></fieldset><label>Commande<input name="command" required autocomplete="off" data-cron-command></label><div class="form-actions"><button class="primary-button" data-cron-submit>Créer la tâche</button><button class="secondary-button" type="button" data-cron-create-close>Annuler</button></div></form></dialog>` +
		`<dialog class="action-dialog action-dialog--wide" data-backup-dialog><form method="post" action="/cron/backup/create" class="selector-form"><input type="hidden" name="_token" value="` + token + `"><input type="hidden" name="kind" data-backup-kind><h2 data-backup-title>Nouvelle sauvegarde</h2><label>Nom<input name="name" maxlength="64" required></label><label>Source<input name="source" data-backup-source required></label><label>Fréquence<select name="schedule" required><option value="0 2 * * *">Chaque nuit à 2 h</option><option value="0 3 * * 0">Chaque dimanche à 3 h</option><option value="0 4 1 * *">Chaque mois à 4 h</option></select></label><label>Stockage temporaire local<input name="destination" value="/var/backups/aegisadmin" required></label><fieldset><legend>Destination rsync/SSH</legend><label>Serveur<input name="remote_host" required></label><label>Utilisateur SSH<input name="remote_user" required></label><label>Port SSH<input name="remote_port" type="number" min="1" max="65535" value="22" required></label><label>Répertoire distant<input name="remote_path" value="/var/backups/aegisadmin" required></label><label>Clé SSH privée<input name="ssh_key" value="/etc/aegisadmin-system/backup-ssh/id_ed25519" required></label></fieldset><label>Conservation locale (jours)<input name="retention_days" type="number" min="0" max="3650" value="2" required></label><label class="checkbox-line"><input type="checkbox" name="remove_local" value="true"> Supprimer la copie locale après un transfert réussi</label><div class="form-actions"><button class="primary-button">Créer la tâche</button><button class="secondary-button" type="button" data-backup-close>Annuler</button></div></form></dialog></section>`
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

func renderCronJobs(items []webcron.Job, token, level string) string {
	columns := 5
	if level == "action" || level == "modify" {
		columns++
	}
	if len(items) == 0 {
		return `<tr><td colspan="` + strconv.Itoa(columns) + `" class="muted">Aucune tâche Cron détectée.</td></tr>`
	}
	var result strings.Builder
	for _, item := range items {
		backupID := backupTaskID(item.Command)
		state, class := "Suspendue", "warning"
		if item.Enabled {
			state, class = "Active", "success"
		}
		result.WriteString(`<tr><td><span class="status-badge status-badge--` + class + `">` + state + `</span></td>`)
		if level == "action" || level == "modify" {
			result.WriteString(`<td>`)
			if backupID != "" {
				result.WriteString(`<select class="cron-action-select" data-backup-action data-csrf="` + token + `" data-task-id="` + backupID + `"><option value="">Action</option><option value="run">Exécuter</option>`)
				if level == "modify" {
					result.WriteString(`<option value="delete">Supprimer</option>`)
				}
				result.WriteString(`</select>`)
			} else if item.Editable {
				result.WriteString(`<select class="cron-action-select" aria-label="Action pour ` + html.EscapeString(item.User) + `" data-cron-action data-csrf="` + token + `" data-user="` + html.EscapeString(item.User) + `" data-task-id="` + html.EscapeString(item.ID) + `" data-schedule="` + html.EscapeString(item.Schedule) + `" data-command="` + html.EscapeString(item.Command) + `"><option value="">Action</option>`)
				if level == "modify" {
					result.WriteString(`<option value="edit">Modifier</option>`)
				}
				if item.Enabled {
					result.WriteString(`<option value="run">Exécuter</option>`)
				}
				if level == "modify" && item.Enabled {
					result.WriteString(`<option value="suspend">Suspendre</option>`)
				}
				if level == "modify" && !item.Enabled {
					result.WriteString(`<option value="resume">Réactiver</option>`)
				}
				if level == "modify" {
					result.WriteString(`<option value="delete">Supprimer</option>`)
				}
				result.WriteString(`</select>`)
			} else {
				result.WriteString(`<span class="muted">Lecture seule</span>`)
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
		a.renderPassword(response, session.CSRFToken, "La requête est invalide.", http.StatusBadRequest, mandatory, user)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 8*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderPassword(response, session.CSRFToken, "La requête est invalide.", http.StatusBadRequest, mandatory, user)
		return
	}
	currentPassword := request.PostForm.Get("current_password")
	newPassword := request.PostForm.Get("new_password")
	confirmation := request.PostForm.Get("new_password_confirmation")
	address, account := requestAddress(request), user.Login
	if !a.dependencies.PasswordLimiter.Allowed(address, account) ||
		!webauth.VerifyPassword(currentPassword, user.PasswordHash) {
		a.dependencies.PasswordLimiter.Failure(address, account)
		a.renderPassword(response, session.CSRFToken, "Le mot de passe actuel est incorrect.", http.StatusUnauthorized, mandatory, user)
		return
	}
	if subtle.ConstantTimeCompare([]byte(newPassword), []byte(confirmation)) != 1 {
		a.renderPassword(response, session.CSRFToken, "La confirmation du nouveau mot de passe ne correspond pas.", http.StatusBadRequest, mandatory, user)
		return
	}
	if webauth.VerifyPassword(newPassword, user.PasswordHash) {
		a.renderPassword(response, session.CSRFToken, "Le nouveau mot de passe doit être différent du mot de passe actuel.", http.StatusBadRequest, mandatory, user)
		return
	}
	newHash, err := webauth.HashPassword(newPassword)
	if err != nil {
		a.renderPassword(response, session.CSRFToken, "Le nouveau mot de passe doit contenir entre 12 et 128 caractères.", http.StatusBadRequest, mandatory, user)
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

func (a *application) renderPassword(response http.ResponseWriter, csrf, message string, status int, mandatory bool, user authstore.User) {
	errorMessage := ""
	if message != "" {
		errorMessage = `<p class="error">` + html.EscapeString(message) + `</p>`
	}
	template := a.accountPasswordPage
	twoFactor := ""
	if mandatory {
		template = a.passwordPage
	} else if user.TOTPEnabledAt.Valid {
		detail := "Elle est facultative pour ce compte."
		action := `<form method="post" action="/go/account/two-factor/disable"><input type="hidden" name="_token" value="` + html.EscapeString(csrf) + `"><label>Code d’authentification actuel<input name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9 ]{6,11}" maxlength="11" required></label><button class="danger-button">Désactiver la double authentification</button></form>`
		if user.TwoFactorRequired {
			detail = "Elle est obligatoire pour ce compte."
			action = ""
		}
		twoFactor = `<section class="content-card account-password-card"><h2>Double authentification</h2><p>État : <strong>activée</strong>. ` + detail + `</p>` + action + `</section>`
	} else {
		twoFactor = `<section class="content-card account-password-card"><h2>Double authentification</h2><p>État : <strong>désactivée</strong>.</p><a class="primary-button" href="/go/account/two-factor/setup">Activer la double authentification</a></section>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
		"{{TWO_FACTOR}}", twoFactor,
	).Replace(template)
	writeHTML(response, page, status)
}

func (a *application) twoFactor(response http.ResponseWriter, request *http.Request) {
	session, _, found := a.pendingTwoFactorUser(response, request)
	if !found {
		return
	}
	a.renderTwoFactor(response, session.CSRFToken, false, http.StatusOK)
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
		a.renderTwoFactor(response, session.CSRFToken, true, http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4*1024)
	if err := request.ParseForm(); err != nil || !a.dependencies.Sessions.ValidateCSRF(session.ID, request.PostForm.Get("_token")) {
		a.renderTwoFactor(response, session.CSRFToken, true, http.StatusBadRequest)
		return
	}
	address, account := requestAddress(request), user.Login
	if !a.dependencies.TwoFactorLimiter.Allowed(address, account) ||
		!a.dependencies.TwoFactor.Verify(user.TOTPSecret.String, request.PostForm.Get("code")) {
		a.dependencies.TwoFactorLimiter.Failure(address, account)
		a.recordAccess(request, &user, account, "two_factor_failure", false)
		a.renderTwoFactor(response, session.CSRFToken, true, http.StatusUnauthorized)
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

func (a *application) renderTwoFactor(response http.ResponseWriter, csrf string, failed bool, status int) {
	errorMessage := ""
	if failed {
		errorMessage = `<p class="error">Le code de vérification ou la requête est invalide.</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
	).Replace(a.twoFactorPage)
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
		errorMessage = `<p class="error">Le code de contrôle ou la requête est invalide.</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
		"{{SECRET}}", html.EscapeString(user.TOTPSecret.String),
		"{{QRCODE}}", base64.StdEncoding.EncodeToString(image),
		"{{ACTION}}", html.EscapeString(action),
	).Replace(a.twoFactorSetupPage)
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

func (a *application) renderLogin(response http.ResponseWriter, csrf string, failed bool, status int) {
	errorMessage := ""
	if failed {
		errorMessage = `<p class="error">Identifiant, mot de passe ou requête invalide.</p>`
	}
	page := strings.NewReplacer(
		"{{CSRF}}", html.EscapeString(csrf),
		"{{ERROR}}", errorMessage,
	).Replace(a.loginPage)
	writeHTML(response, page, status)
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
