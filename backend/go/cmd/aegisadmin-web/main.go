package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aegisadmin/backend/internal/authstore"
	"aegisadmin/backend/internal/buildinfo"
	"aegisadmin/backend/internal/client"
	"aegisadmin/backend/internal/protocol"
	"aegisadmin/backend/internal/webapache"
	"aegisadmin/backend/internal/webapp"
	"aegisadmin/backend/internal/webauth"
	"aegisadmin/backend/internal/webcertbot"
	"aegisadmin/backend/internal/webconfiguration"
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
)

func main() {
	address := flag.String("listen", "127.0.0.1:9080", "HTTP listen address")
	socketPath := flag.String("backend-socket", "/run/aegisadmin-system/backend.sock", "backend Unix socket path")
	tlsCertificate := flag.String("tls-cert", "", "TLS certificate path")
	tlsPrivateKey := flag.String("tls-key", "", "TLS private key path")
	databasePath := flag.String("database", "/var/lib/aegisadmin/database/aegisadmin.sqlite", "application SQLite database path")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	if (*tlsCertificate == "") != (*tlsPrivateKey == "") {
		fmt.Fprintln(os.Stderr, "tls-cert et tls-key doivent être indiqués ensemble")
		os.Exit(2)
	}

	logger := log.New(os.Stderr, "aegisadmin-web: ", log.LstdFlags|log.LUTC)
	store, err := authstore.OpenReadWrite(*databasePath)
	if err != nil {
		logger.Fatalf("base SQLite indisponible : %v", err)
	}
	defer store.Close()
	migrationContext, migrationCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := store.EnsureGoNavigation(migrationContext); err != nil {
		migrationCancel()
		logger.Fatalf("navigation Go indisponible : %v", err)
	}
	migrationCancel()
	backendClient := client.New(*socketPath)
	readiness := func() error {
		reply, err := backendClient.Execute(protocol.Request{Domain: "system", Command: "info"})
		if err != nil {
			return err
		}
		if !reply.Response.Success {
			return fmt.Errorf("backend returned exit code %d", reply.ExitCode)
		}
		return nil
	}
	databaseReadiness := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return store.Check(ctx)
	}
	sessions, err := websession.New(websession.DefaultConfig())
	if err != nil {
		logger.Fatalf("configuration des sessions invalide : %v", err)
	}
	server := webapp.Server(*address, webapp.Handler(webapp.Dependencies{
		Readiness: webapp.ReadinessChecks{Backend: readiness, Database: databaseReadiness},
		Sessions:  sessions, Authenticator: webauth.New(store), Users: store, AccessLog: store,
		LoginLimiter: webauth.NewLoginLimiter(),
		TwoFactor:    webauth.NewTOTPVerifier(), TwoFactorLimiter: webauth.NewLoginLimiter(),
		Enrollment: store,
		Passwords:  store, PasswordLimiter: webauth.NewLoginLimiter(),
		Navigation:      store,
		Dashboard:       webdashboard.New(backendClient),
		Authorization:   store,
		Storage:         webstorage.New(backendClient),
		Services:        webservices.New(backendClient),
		Network:         webnetwork.New(backendClient),
		Logs:            weblogs.New(backendClient),
		PHP:             webphp.New(backendClient),
		MySQL:           webmysql.New(backendClient),
		Tor:             webtor.New(backendClient),
		Apache:          webapache.New(backendClient),
		Fail2ban:        webfail2ban.New(backendClient),
		Firewall:        webfirewall.New(backendClient),
		Cron:            webcron.New(backendClient),
		Certbot:         webcertbot.New(backendClient),
		Updates:         webupdates.New(backendClient),
		UserAdmin:       store,
		NavigationAdmin: store,
		Settings:        store,
		AdminAccess:     websettings.New(backendClient),
		Configuration:   webconfiguration.New(backendClient),
	}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Printf("arrêt incomplet : %v", err)
		}
	}()

	protocolLabel := "HTTP"
	serve := server.ListenAndServe
	if *tlsCertificate != "" {
		protocolLabel = "HTTPS"
		serve = func() error { return server.ListenAndServeTLS(*tlsCertificate, *tlsPrivateKey) }
	}
	logger.Printf("écoute %s sur %s", protocolLabel, *address)
	if err := serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}
