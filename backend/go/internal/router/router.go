package router

import (
	"context"
	"regexp"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	ExitMissingArgument     = 2
	ExitDomainNotFound      = 3
	ExitFeatureNotAvailable = 8
	ExitInvalidDomain       = 10
)

var domainPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

var knownDomains = map[string]struct{}{
	"admin-access":  {},
	"apache":        {},
	"certbot":       {},
	"configuration": {},
	"cron":          {},
	"fail2ban":      {},
	"firewall":      {},
	"logs":          {},
	"mysql":         {},
	"network":       {},
	"php":           {},
	"services":      {},
	"storage":       {},
	"system":        {},
	"tor":           {},
	"updates":       {},
}

type Handler interface {
	Handle(context.Context, string, []string) protocol.Reply
}

type Router struct {
	handlers map[string]Handler
}

func New() *Router {
	return &Router{handlers: make(map[string]Handler)}
}

func (r *Router) Register(domain string, handler Handler) {
	if !domainPattern.MatchString(domain) {
		panic("invalid backend domain: " + domain)
	}

	if handler == nil {
		panic("nil handler for backend domain: " + domain)
	}

	if _, exists := r.handlers[domain]; exists {
		panic("duplicate backend domain: " + domain)
	}

	r.handlers[domain] = handler
}

func (r *Router) Route(ctx context.Context, request protocol.Request) protocol.Reply {
	if !domainPattern.MatchString(request.Domain) {
		return failure(
			ExitInvalidDomain,
			"INVALID_DOMAIN_NAME",
			"Le nom interne du domaine est invalide.",
		)
	}

	if _, known := knownDomains[request.Domain]; !known {
		return failure(
			ExitDomainNotFound,
			"DOMAIN_NOT_FOUND",
			"Le domaine demandé n’existe pas.",
		)
	}

	handler, exists := r.handlers[request.Domain]
	if !exists {
		return failure(
			ExitFeatureNotAvailable,
			"FEATURE_NOT_AVAILABLE",
			"Ce domaine n’est pas encore disponible dans le backend Go.",
		)
	}

	return handler.Handle(ctx, request.Command, request.Arguments)
}

func failure(exitCode int, code string, message string) protocol.Reply {
	return protocol.Reply{
		ExitCode: exitCode,
		Response: api.Failure(code, message),
	}
}
