package apache

import (
	"context"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

type Backend interface {
	Execute(context.Context, string, string) (map[string]any, *Error)
}

type Handler struct{ backend Backend }

type Error struct {
	ExitCode int
	Code     string
	Message  string
	Details  map[string]any
}

var argumentCounts = map[string]int{
	"info": 0, "overview": 0, "configtest": 0, "vhosts": 0, "config": 1,
	"sites": 0, "site": 1, "create": 1, "update": 1,
	"enable": 1, "disable": 1, "delete": 1, "modules": 0,
	"reload": 0, "restart": 0,
}

func New(backend Backend) *Handler { return &Handler{backend: backend} }

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(&Error{ExitCode: 2, Code: "MISSING_COMMAND", Message: "Aucune commande n’a été indiquée pour le domaine apache."})
	}
	expected, known := argumentCounts[command]
	if !known {
		return failure(&Error{ExitCode: 4, Code: "COMMAND_NOT_FOUND", Message: "La commande demandée n’existe pas dans le domaine apache."})
	}
	if len(arguments) != expected {
		return failure(&Error{ExitCode: 2, Code: "INVALID_ARGUMENT_COUNT", Message: "Le nombre d’arguments fourni est invalide."})
	}
	argument := ""
	if expected == 1 {
		argument = arguments[0]
	}
	data, backendErr := h.backend.Execute(ctx, command, argument)
	if backendErr != nil {
		return failure(backendErr)
	}
	return protocol.Reply{Response: api.Success(data)}
}

func failure(err *Error) protocol.Reply {
	response := api.Failure(err.Code, err.Message)
	if response.Error != nil && len(err.Details) != 0 {
		response.Error.Details = err.Details
	}
	return protocol.Reply{ExitCode: err.ExitCode, Response: response}
}

func domainError(exit int, code, message string, details map[string]any) *Error {
	return &Error{ExitCode: exit, Code: code, Message: message, Details: details}
}
