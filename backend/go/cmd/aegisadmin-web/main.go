package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	allowFrom := flag.String("allow-from", "all", "allowed client IP or CIDR")
	socketPath := flag.String("backend-socket", "/run/aegisadmin-system/backend.sock", "backend Unix socket path")
	tlsCertificate := flag.String("tls-cert", "", "TLS certificate path")
	tlsPrivateKey := flag.String("tls-key", "", "TLS private key path")
	databasePath := flag.String("database", "/var/lib/aegisadmin/database/aegisadmin.sqlite", "application SQLite database path")
	sessionPath := flag.String("sessions", "/var/lib/aegisadmin/sessions/sessions.json", "persistent authenticated sessions path")
	secretKeyPath := flag.String("secret-key", "/var/lib/aegisadmin/secrets/settings.key", "application secret encryption key path")
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
	secretKey, err := authstore.LoadOrCreateSecretKey(*secretKeyPath)
	if err != nil {
		logger.Fatalf("clé de chiffrement indisponible : %v", err)
	}
	if err = store.SetSecretKey(secretKey); err != nil {
		logger.Fatalf("clé de chiffrement invalide : %v", err)
	}
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
	sessions, err := websession.NewPersistent(websession.DefaultConfig(), *sessionPath)
	if err != nil {
		logger.Fatalf("configuration des sessions invalide : %v", err)
	}
	handler := webapp.Handler(webapp.Dependencies{
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
	})
	handler, err = restrictClients(handler, *allowFrom)
	if err != nil {
		logger.Fatalf("restriction réseau invalide : %v", err)
	}
	addresses := splitListenAddresses(*address)
	if len(addresses) == 0 {
		logger.Fatal("aucune adresse d’écoute valide")
	}
	servers := make([]*http.Server, 0, len(addresses))
	for _, listenAddress := range addresses {
		servers = append(servers, webapp.Server(listenAddress, handler))
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, server := range servers {
			if shutdownErr := server.Shutdown(shutdownContext); shutdownErr != nil {
				logger.Printf("arrêt incomplet : %v", shutdownErr)
			}
		}
	}()

	protocolLabel := "HTTP"
	if *tlsCertificate != "" {
		protocolLabel = "HTTPS"
	}
	errorsChannel := make(chan error, len(servers))
	for index, server := range servers {
		listenAddress := addresses[index]
		logger.Printf("écoute %s sur %s", protocolLabel, listenAddress)
		go func(server *http.Server) {
			serve := server.ListenAndServe
			if *tlsCertificate != "" {
				serve = func() error { return server.ListenAndServeTLS(*tlsCertificate, *tlsPrivateKey) }
			}
			errorsChannel <- serve()
		}(server)
	}
	for range servers {
		if serveErr := <-errorsChannel; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Fatal(serveErr)
		}
	}
}

func splitListenAddresses(value string) []string {
	result := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func restrictClients(next http.Handler, value string) (http.Handler, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "all" {
		return next, nil
	}
	var network *net.IPNet
	if address := net.ParseIP(value); address != nil {
		bits := 128
		if address.To4() != nil {
			bits = 32
		}
		network = &net.IPNet{IP: address, Mask: net.CIDRMask(bits, bits)}
	} else {
		_, parsed, err := net.ParseCIDR(value)
		if err != nil {
			return nil, err
		}
		network = parsed
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		host, _, err := net.SplitHostPort(request.RemoteAddr)
		if err != nil || !network.Contains(net.ParseIP(host)) {
			http.Error(response, "Accès interdit.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(response, request)
	}), nil
}
