package apache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type sitePayload struct {
	Filename *string `json:"filename"`
	ConfigID *string `json:"config_id"`
	Content  *string `json:"content"`
}

var base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)

func (b *LinuxBackend) availableSitePaths() ([]string, *Error) {
	entries, err := os.ReadDir(b.sitesAvailable)
	if err != nil {
		return nil, domainError(10, "APACHE_SITES_READ_FAILED", "La liste des sites Apache ne peut pas être lue.", map[string]any{"reason": err.Error()})
	}
	root, err := filepath.EvalSymlinks(b.sitesAvailable)
	if err != nil {
		return nil, domainError(10, "APACHE_SITES_READ_FAILED", "La liste des sites Apache ne peut pas être lue.", map[string]any{"reason": err.Error()})
	}
	paths := []string{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(b.sitesAvailable, entry.Name()))
		if err != nil || filepath.Dir(resolved) != root {
			continue
		}
		info, err := os.Stat(resolved)
		if err == nil && info.Mode().IsRegular() {
			paths = append(paths, resolved)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.ToLower(filepath.Base(paths[i])) < strings.ToLower(filepath.Base(paths[j]))
	})
	return paths, nil
}

func (b *LinuxBackend) sitePath(identifier string) (string, *Error) {
	if !identifierPattern.MatchString(identifier) {
		return "", domainError(2, "INVALID_APACHE_SITE_ID", "L’identifiant du site Apache est invalide.", nil)
	}
	paths, err := b.availableSitePaths()
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		if configIdentifier(path) == identifier {
			return path, nil
		}
	}
	return "", domainError(4, "APACHE_SITE_NOT_FOUND", "Le site Apache demandé n’existe pas.", nil)
}

func (b *LinuxBackend) siteEnabled(path string) (bool, *Error) {
	if filepath.Clean(b.sitesAvailable) == filepath.Clean(b.sitesEnabled) {
		return true, nil
	}
	entries, err := os.ReadDir(b.sitesEnabled)
	if err != nil {
		return false, domainError(10, "APACHE_SITES_READ_FAILED", "L’état des sites Apache ne peut pas être lu.", map[string]any{"reason": err.Error()})
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(b.sitesEnabled, entry.Name()))
		if err == nil && resolved == path {
			return true, nil
		}
	}
	return false, nil
}

func uniqueStrings(matches [][]string) []string {
	result, seen := []string{}, map[string]struct{}{}
	for _, match := range matches {
		if _, exists := seen[match[1]]; !exists {
			seen[match[1]] = struct{}{}
			result = append(result, match[1])
		}
	}
	return result
}

func uniquePorts(matches [][]string) []int {
	result, seen := []int{}, map[int]struct{}{}
	for _, match := range matches {
		port, _ := strconv.Atoi(match[1])
		if _, exists := seen[port]; !exists {
			seen[port] = struct{}{}
			result = append(result, port)
		}
	}
	return result
}

func (b *LinuxBackend) sites() (map[string]any, *Error) {
	paths, err := b.availableSitePaths()
	if err != nil {
		return nil, err
	}
	sites := []map[string]any{}
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, domainError(10, "APACHE_SITE_READ_FAILED", "Un site Apache ne peut pas être lu.", map[string]any{"reason": readErr.Error()})
		}
		info, readErr := os.Stat(path)
		if readErr != nil {
			return nil, domainError(10, "APACHE_SITE_READ_FAILED", "Un site Apache ne peut pas être lu.", map[string]any{"reason": readErr.Error()})
		}
		enabled, stateErr := b.siteEnabled(path)
		if stateErr != nil {
			return nil, stateErr
		}
		text := strings.ToValidUTF8(string(content), "�")
		sites = append(sites, map[string]any{
			"filename": filepath.Base(path), "config_id": configIdentifier(path), "enabled": enabled,
			"size_bytes": info.Size(), "server_names": uniqueStrings(serverNamePattern.FindAllStringSubmatch(text, -1)),
			"ports": uniquePorts(virtualHostPattern.FindAllStringSubmatch(text, -1)),
		})
	}
	return map[string]any{"sites": sites, "count": len(sites)}, nil
}

func (b *LinuxBackend) site(identifier string) (map[string]any, *Error) {
	path, err := b.sitePath(identifier)
	if err != nil {
		return nil, err
	}
	info, readErr := os.Stat(path)
	if readErr != nil {
		return nil, domainError(10, "APACHE_SITE_READ_FAILED", "Le site Apache ne peut pas être lu.", map[string]any{"reason": readErr.Error()})
	}
	if info.Size() > maximumSiteSize {
		return nil, domainError(10, "APACHE_SITE_TOO_LARGE", "Le site Apache dépasse la taille autorisée.", map[string]any{"max_bytes": maximumSiteSize, "size_bytes": info.Size()})
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, domainError(10, "APACHE_SITE_READ_FAILED", "Le site Apache ne peut pas être lu.", reason(readErr))
	}
	if !utf8.Valid(content) {
		return nil, domainError(10, "APACHE_SITE_READ_FAILED", "Le site Apache n’est pas encodé en UTF-8.", nil)
	}
	enabled, stateErr := b.siteEnabled(path)
	if stateErr != nil {
		return nil, stateErr
	}
	return map[string]any{"filename": filepath.Base(path), "config_id": identifier, "enabled": enabled, "content": string(content)}, nil
}

func (b *LinuxBackend) config(ctx context.Context, identifier string) (map[string]any, *Error) {
	if !identifierPattern.MatchString(identifier) {
		return nil, domainError(2, "INVALID_APACHE_CONFIG_ID", "L’identifiant de configuration Apache est invalide.", nil)
	}
	output, err := b.apacheS(ctx)
	if err != nil {
		return nil, err
	}
	configs := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		match := configPathPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		path, resolveErr := filepath.EvalSymlinks(match[1])
		if resolveErr == nil {
			configs[configIdentifier(path)] = path
		}
	}
	path, exists := configs[identifier]
	if !exists {
		return nil, domainError(4, "APACHE_CONFIG_NOT_FOUND", "La configuration Apache demandée n’est pas active.", nil)
	}
	root, _ := filepath.EvalSymlinks(b.apacheRoot)
	relative, relErr := filepath.Rel(root, path)
	info, statErr := os.Stat(path)
	if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || strings.ToLower(filepath.Ext(path)) != ".conf" || statErr != nil || !info.Mode().IsRegular() {
		return nil, domainError(4, "APACHE_CONFIG_NOT_ALLOWED", "La configuration Apache demandée n’est pas autorisée.", nil)
	}
	if info.Size() > maximumConfigSize {
		return nil, domainError(10, "APACHE_CONFIG_TOO_LARGE", "La configuration Apache dépasse la taille autorisée.", map[string]any{"max_bytes": maximumConfigSize, "size_bytes": info.Size()})
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, domainError(10, "APACHE_CONFIG_READ_FAILED", "La configuration Apache ne peut pas être lue.", map[string]any{"reason": readErr.Error()})
	}
	return map[string]any{"config_file": path, "content": strings.ToValidUTF8(string(content), "�")}, nil
}

func parsePayload(argument string) (sitePayload, *Error) {
	var payload sitePayload
	decoder := json.NewDecoder(strings.NewReader(argument))
	if err := decoder.Decode(&payload); err != nil {
		return payload, domainError(2, "INVALID_APACHE_SITE_PAYLOAD", "Les données du site Apache sont invalides.", nil)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return payload, domainError(2, "INVALID_APACHE_SITE_PAYLOAD", "Les données du site Apache sont invalides.", nil)
	}
	return payload, nil
}

func decodeContent(payload sitePayload) (string, *Error) {
	if payload.Content == nil || *payload.Content == "" {
		return "", domainError(2, "INVALID_APACHE_SITE_CONTENT", "Le contenu du site Apache est invalide.", nil)
	}
	if !base64Pattern.MatchString(*payload.Content) {
		return "", domainError(2, "INVALID_APACHE_SITE_CONTENT", "Le contenu du site Apache est invalide.", nil)
	}
	content, err := base64.StdEncoding.Strict().DecodeString(*payload.Content)
	if err != nil {
		return "", domainError(2, "INVALID_APACHE_SITE_CONTENT", "Le contenu du site Apache est invalide.", nil)
	}
	if len(content) == 0 || len(content) > maximumSiteSize {
		return "", domainError(2, "INVALID_APACHE_SITE_CONTENT", "Le contenu du site Apache dépasse les limites autorisées.", map[string]any{"max_bytes": maximumSiteSize, "size_bytes": len(content)})
	}
	if strings.IndexByte(string(content), 0) >= 0 {
		return "", domainError(2, "INVALID_APACHE_SITE_CONTENT", "Le contenu du site Apache contient un caractère interdit.", nil)
	}
	if !utf8.Valid(content) {
		return "", domainError(2, "INVALID_APACHE_SITE_CONTENT", "Le contenu du site Apache n’est pas encodé en UTF-8.", nil)
	}
	text := string(content)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, nil
}

func actionResponse(action, path, identifier string, enabled bool) map[string]any {
	return map[string]any{"action": action, "filename": filepath.Base(path), "config_id": identifier, "enabled": enabled, "result": "success"}
}

func reason(err error) map[string]any            { return map[string]any{"reason": err.Error()} }
func outputDetails(output string) map[string]any { return map[string]any{"output": output} }
func backupName(path string) string {
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405.000000Z"), filepath.Base(path))
}
