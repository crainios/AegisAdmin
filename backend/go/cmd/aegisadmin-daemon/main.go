package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"aegisadmin/backend/internal/buildinfo"
	adminaccessdomain "aegisadmin/backend/internal/domain/adminaccess"
	apachedomain "aegisadmin/backend/internal/domain/apache"
	certbotdomain "aegisadmin/backend/internal/domain/certbot"
	configurationdomain "aegisadmin/backend/internal/domain/configuration"
	crondomain "aegisadmin/backend/internal/domain/cron"
	fail2bandomain "aegisadmin/backend/internal/domain/fail2ban"
	firewalldomain "aegisadmin/backend/internal/domain/firewall"
	logsdomain "aegisadmin/backend/internal/domain/logs"
	mysqldomain "aegisadmin/backend/internal/domain/mysql"
	networkdomain "aegisadmin/backend/internal/domain/network"
	phpdomain "aegisadmin/backend/internal/domain/php"
	servicesdomain "aegisadmin/backend/internal/domain/services"
	storagedomain "aegisadmin/backend/internal/domain/storage"
	systemdomain "aegisadmin/backend/internal/domain/system"
	tordomain "aegisadmin/backend/internal/domain/tor"
	updatesdomain "aegisadmin/backend/internal/domain/updates"
	"aegisadmin/backend/internal/router"
	"aegisadmin/backend/internal/server"
)

const defaultSocketPath = "/run/aegisadmin-system/backend.sock"

func main() {
	socketPath := flag.String("socket", defaultSocketPath, "absolute Unix socket path")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}

	logger := log.New(os.Stderr, "aegisadmin-daemon: ", log.LstdFlags|log.LUTC)
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	backendRouter := router.New()
	apacheHandler := apachedomain.New(apachedomain.NewLinuxBackend())
	certbotHandler := certbotdomain.New(certbotdomain.NewLinuxBackend())
	cronHandler := crondomain.New(crondomain.NewLinuxBackend())
	fail2banHandler := fail2bandomain.New(fail2bandomain.NewLinuxBackend())
	firewallHandler := firewalldomain.New(firewalldomain.NewLinuxBackend())
	mysqlHandler := mysqldomain.New(mysqldomain.NewLinuxBackend())
	networkHandler := networkdomain.New(networkdomain.NewLinuxCollector())
	phpHandler := phpdomain.New(phpdomain.NewLinuxBackend())
	servicesHandler := servicesdomain.New(servicesdomain.NewLinuxCollector())
	systemHandler := systemdomain.New(systemdomain.NewLinuxCollector(ctx))
	torHandler := tordomain.New(tordomain.NewLinuxBackend())
	configurationHandler := configurationdomain.New(map[string]configurationdomain.Provider{
		"apache": apacheHandler, "certbot": certbotHandler, "cron": cronHandler,
		"fail2ban": fail2banHandler, "firewall": firewallHandler, "mysql": mysqlHandler,
		"network": networkHandler, "php": phpHandler, "services": servicesHandler,
		"system": systemHandler, "tor": torHandler,
	}, configurationdomain.NewLinuxPlatform())
	backendRouter.Register("admin-access", adminaccessdomain.New())
	backendRouter.Register("certbot", certbotHandler)
	backendRouter.Register("configuration", configurationHandler)
	backendRouter.Register("cron", cronHandler)
	backendRouter.Register("fail2ban", fail2banHandler)
	backendRouter.Register("firewall", firewallHandler)
	backendRouter.Register(
		"apache",
		apacheHandler,
	)
	backendRouter.Register(
		"logs",
		logsdomain.New(logsdomain.NewLinuxCollector()),
	)
	backendRouter.Register(
		"mysql",
		mysqlHandler,
	)
	backendRouter.Register(
		"network",
		networkHandler,
	)
	backendRouter.Register(
		"php",
		phpHandler,
	)
	backendRouter.Register(
		"services",
		servicesHandler,
	)
	backendRouter.Register(
		"storage",
		storagedomain.New(storagedomain.NewLinuxCollector()),
	)
	backendRouter.Register(
		"system",
		systemHandler,
	)
	backendRouter.Register("tor", torHandler)
	backendRouter.Register("updates", updatesdomain.New(updatesdomain.NewLinuxBackend(ctx)))
	backendServer := server.New(*socketPath, backendRouter, logger)

	if err := backendServer.ListenAndServe(ctx); err != nil {
		logger.Fatal(err)
	}
}
