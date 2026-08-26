package configuration

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

type Provider interface {
	Handle(context.Context, string, []string) protocol.Reply
}

type Platform interface {
	Inventory(context.Context) map[string]any
}

type Handler struct {
	providers map[string]Provider
	platform  Platform
	store     *snapshotStore
}

type request struct {
	id, label, provider string
	commands            []string
}

var sections = []request{
	{"system", "Système", "system", []string{"info"}},
	{"packages", "Paquets installés", "", nil},
	{"services", "Services systemd", "", nil},
	{"accounts", "Utilisateurs et groupes", "", nil},
	{"network", "Réseau", "network", []string{"list"}},
	{"ports", "Ports en écoute", "", nil},
	{"firewall", "Pare-feu", "firewall", []string{"info", "status", "rules"}},
	{"fail2ban", "Fail2ban", "fail2ban", []string{"info", "status"}},
	{"php", "PHP", "php", []string{"info", "fpm", "configuration:cli", "extensions:cli"}},
	{"apache", "Apache", "apache", []string{"overview"}},
	{"certificates", "Certificats", "certbot", []string{"info", "certificates"}},
	{"mysql", "MySQL / MariaDB", "mysql", []string{"info", "status", "databases"}},
	{"cron", "Tâches planifiées", "cron", []string{"info", "status", "jobs", "users"}},
	{"tor", "Tor", "tor", []string{"info", "status", "hidden-services"}},
}

func New(providers map[string]Provider, platform Platform) *Handler {
	return &Handler{providers: providers, platform: platform, store: newSnapshotStore(defaultSnapshotDir)}
}

func newHandler(providers map[string]Provider, platform Platform, store *snapshotStore) *Handler {
	return &Handler{providers: providers, platform: platform, store: store}
}

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine configuration.")
	}
	switch command {
	case "capabilities":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		items := make([]map[string]any, 0, len(sections))
		for _, section := range sections {
			_, provided := h.providers[section.provider]
			items = append(items, map[string]any{"id": section.id, "label": section.label, "supported": section.provider == "" || provided})
		}
		return success(map[string]any{
			"inventory_read_only": true, "snapshot_storage": true,
			"snapshot_import": true, "snapshot_comparison": true,
			"sections": items,
		})
	case "inventory":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		return success(h.inventory(ctx))
	case "snapshot-create":
		if len(arguments) > 1 {
			return invalidArgumentCount()
		}
		snapshot, err := h.store.create(h.inventory(ctx))
		if err != nil {
			return fail(10, "CONFIGURATION_SNAPSHOT_CREATE_FAILED", "Le snapshot de configuration n’a pas pu être créé.")
		}
		if len(arguments) == 1 {
			if err = h.store.rename(snapshot.ID, arguments[0]); err != nil {
				return fail(2, "CONFIGURATION_SNAPSHOT_NAME_INVALID", "Le nom du snapshot est invalide.")
			}
		}
		return success(map[string]any{"snapshot": snapshot})
	case "snapshot-name":
		if len(arguments) != 2 {
			return invalidArgumentCount()
		}
		if err := h.store.rename(arguments[0], arguments[1]); err != nil {
			return fail(5, "CONFIGURATION_SNAPSHOT_RENAME_FAILED", "Le nom du snapshot n’a pas pu être modifié.")
		}
		return success(map[string]any{"message": "Le nom du snapshot a été modifié."})
	case "snapshot-delete":
		if len(arguments) != 2 || arguments[0] != arguments[1] {
			return fail(2, "INVALID_CONFIRMATION", "La confirmation de suppression est invalide.")
		}
		if err := h.store.delete(arguments[0]); err != nil {
			return fail(5, "CONFIGURATION_SNAPSHOT_DELETE_FAILED", "Le snapshot n’a pas pu être supprimé.")
		}
		return success(map[string]any{"message": "Le snapshot a été supprimé."})
	case "snapshot-import":
		if len(arguments) != 2 {
			return invalidArgumentCount()
		}
		snapshot, err := h.store.importSnapshot([]byte(arguments[1]), arguments[0])
		if errors.Is(err, os.ErrExist) {
			return fail(3, "CONFIGURATION_SNAPSHOT_EXISTS", "Ce snapshot existe déjà sur le serveur.")
		}
		if err != nil {
			return fail(2, "CONFIGURATION_SNAPSHOT_IMPORT_FAILED", "Le fichier importé n’est pas un snapshot AegisAdmin valide.")
		}
		return success(map[string]any{"snapshot": snapshot})
	case "snapshot-compare":
		if len(arguments) != 2 || arguments[0] == arguments[1] {
			return invalidArgumentCount()
		}
		source, err := h.store.get(arguments[0])
		if err != nil {
			return fail(5, "CONFIGURATION_SNAPSHOT_NOT_FOUND", "Le snapshot source est introuvable ou altéré.")
		}
		target, err := h.store.get(arguments[1])
		if err != nil {
			return fail(5, "CONFIGURATION_SNAPSHOT_NOT_FOUND", "Le snapshot cible est introuvable ou altéré.")
		}
		return success(map[string]any{"comparison": compareSnapshots(source, target)})
	case "snapshots":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		items, err := h.store.list()
		if err != nil {
			return fail(10, "CONFIGURATION_SNAPSHOTS_READ_FAILED", "Les snapshots de configuration n’ont pas pu être lus.")
		}
		return success(map[string]any{"snapshots": items, "count": len(items)})
	case "snapshot":
		if len(arguments) != 1 {
			return invalidArgumentCount()
		}
		snapshot, err := h.store.get(arguments[0])
		if err != nil {
			return fail(5, "CONFIGURATION_SNAPSHOT_NOT_FOUND", "Le snapshot demandé est introuvable ou altéré.")
		}
		return success(map[string]any{"snapshot": snapshot})
	case "snapshot-export":
		if len(arguments) != 1 {
			return invalidArgumentCount()
		}
		content, err := h.store.export(arguments[0])
		if err != nil {
			return fail(5, "CONFIGURATION_SNAPSHOT_NOT_FOUND", "Le snapshot demandé est introuvable ou altéré.")
		}
		return success(map[string]any{"content": string(content)})
	default:
		return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine configuration.")
	}
}

func invalidArgumentCount() protocol.Reply {
	return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
}

func (h *Handler) inventory(ctx context.Context) map[string]any {
	result := make([]map[string]any, len(sections))
	platform := h.platform.Inventory(ctx)
	var wait sync.WaitGroup
	for index, section := range sections {
		index, section := index, section
		if section.provider == "" {
			data, ok := platform[section.id]
			result[index] = sectionResult(section, ok, data, "")
			continue
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			provider, ok := h.providers[section.provider]
			if !ok {
				result[index] = sectionResult(section, false, nil, "Collecteur indisponible.")
				return
			}
			data := make(map[string]any, len(section.commands))
			for _, operation := range section.commands {
				command, argument, hasArgument := strings.Cut(operation, ":")
				arguments := []string(nil)
				if hasArgument {
					arguments = []string{argument}
				}
				reply := provider.Handle(ctx, command, arguments)
				if !reply.Response.Success || reply.Response.Data == nil {
					message := "Lecture impossible."
					if reply.Response.Error != nil && reply.Response.Error.Message != "" {
						message = reply.Response.Error.Message
					}
					result[index] = sectionResult(section, false, nil, message)
					return
				}
				data[operation] = *reply.Response.Data
			}
			if supplemental, exists := platform[section.id]; exists {
				data["configuration_files"] = supplemental
			}
			result[index] = sectionResult(section, true, data, "")
		}()
	}
	wait.Wait()
	return map[string]any{
		"schema":       "aegisadmin.configuration.inventory.v1",
		"read_only":    true,
		"collected_at": time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		"sections":     result,
	}
}

func sectionResult(section request, available bool, data any, message string) map[string]any {
	return map[string]any{"id": section.id, "label": section.label, "available": available, "message": message, "data": data}
}

func success(data map[string]any) protocol.Reply { return protocol.Reply{Response: api.Success(data)} }
func fail(exit int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exit, Response: api.Failure(code, message)}
}
