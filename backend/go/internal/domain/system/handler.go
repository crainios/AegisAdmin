package system

import (
	"context"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	exitInvalidArguments = 2
	exitReadFailed       = 3
	exitCommandNotFound  = 4
)

type Collector interface {
	Info(context.Context) (map[string]any, error)
	Processes(context.Context) (map[string]any, error)
}

type Handler struct {
	collector Collector
}

func New(collector Collector) *Handler {
	return &Handler{collector: collector}
}

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(
			exitInvalidArguments,
			"MISSING_COMMAND",
			"Aucune commande n’a été indiquée pour le domaine system.",
		)
	}

	if len(arguments) != 0 {
		return failure(
			exitInvalidArguments,
			"INVALID_ARGUMENT_COUNT",
			"Le nombre d’arguments fourni est invalide.",
		)
	}

	switch command {
	case "info":
		data, err := h.collector.Info(ctx)
		if err != nil {
			return failure(
				exitReadFailed,
				"SYSTEM_INFO_READ_FAILED",
				"Les informations générales du système n’ont pas pu être lues.",
			)
		}
		return success(data)

	case "processes":
		data, err := h.collector.Processes(ctx)
		if err != nil {
			return failure(
				exitReadFailed,
				"SYSTEM_PROCESSES_READ_FAILED",
				"La liste des processus du système n’a pas pu être lue.",
			)
		}
		return success(data)

	default:
		return failure(
			exitCommandNotFound,
			"COMMAND_NOT_FOUND",
			"La commande demandée n’existe pas dans le domaine system.",
		)
	}
}

func success(data map[string]any) protocol.Reply {
	return protocol.Reply{Response: api.Success(data)}
}

func failure(exitCode int, code string, message string) protocol.Reply {
	return protocol.Reply{
		ExitCode: exitCode,
		Response: api.Failure(code, message),
	}
}
