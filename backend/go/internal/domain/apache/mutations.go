package apache

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var filenamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}\.conf$`)

func (b *LinuxBackend) atomicWrite(path, content string) error {
	file, err := os.CreateTemp(b.sitesAvailable, ".aegisadmin-apache-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err = file.WriteString(content); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(temporary, 0644)
	}
	if err == nil {
		err = os.Rename(temporary, path)
	}
	return err
}

func (b *LinuxBackend) backup(path string) (string, *Error) {
	if err := os.MkdirAll(b.backupRoot, 0700); err != nil {
		return "", domainError(10, "APACHE_SITE_BACKUP_FAILED", "La sauvegarde du site Apache a échoué.", reason(err))
	}
	if err := os.Chmod(b.backupRoot, 0700); err != nil {
		return "", domainError(10, "APACHE_SITE_BACKUP_FAILED", "La sauvegarde du site Apache a échoué.", reason(err))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", domainError(10, "APACHE_SITE_BACKUP_FAILED", "La sauvegarde du site Apache a échoué.", reason(err))
	}
	backupPath := filepath.Join(b.backupRoot, backupName(path))
	if err := os.WriteFile(backupPath, content, 0600); err != nil {
		return "", domainError(10, "APACHE_SITE_BACKUP_FAILED", "La sauvegarde du site Apache a échoué.", reason(err))
	}
	return backupPath, nil
}

func (b *LinuxBackend) restore(path, backup string) {
	content, err := os.ReadFile(backup)
	if err == nil {
		_ = os.WriteFile(path, content, 0644)
		_ = os.Chmod(path, 0644)
	}
}

func (b *LinuxBackend) validateDisabled(ctx context.Context, path string) (bool, string, *Error) {
	if filepath.Clean(b.sitesAvailable) == filepath.Clean(b.sitesEnabled) {
		return b.configTest(ctx)
	}
	link := filepath.Join(b.sitesEnabled, "zz-aegisadmin-validation-"+time.Now().UTC().Format("150405.000000000")+".conf")
	_ = os.Remove(link)
	if err := os.Symlink(path, link); err != nil {
		return false, "", domainError(10, "APACHE_SITE_VALIDATION_FAILED", "Le site Apache ne peut pas être validé.", reason(err))
	}
	defer os.Remove(link)
	return b.configTest(ctx)
}

func (b *LinuxBackend) reloadCommand(ctx context.Context) (bool, string, *Error) {
	result, err := b.runner.Run(ctx, systemctlCommand, "reload", "--", b.service+".service")
	if err != nil {
		return false, "", err
	}
	return result.OK, result.Output, nil
}

func (b *LinuxBackend) siteCommand(ctx context.Context, command, filename string) (bool, string, *Error) {
	result, err := b.runner.Run(ctx, command, filename)
	if err != nil {
		return false, "", err
	}
	return result.OK, result.Output, nil
}

func (b *LinuxBackend) create(ctx context.Context, argument string) (map[string]any, *Error) {
	payload, err := parsePayload(argument)
	if err != nil {
		return nil, err
	}
	if payload.Filename == nil || !filenamePattern.MatchString(*payload.Filename) || filepath.Base(*payload.Filename) != *payload.Filename {
		return nil, domainError(2, "INVALID_APACHE_SITE_FILENAME", "Le nom du fichier de site Apache est invalide.", nil)
	}
	content, err := decodeContent(payload)
	if err != nil {
		return nil, err
	}
	root, resolveErr := filepath.EvalSymlinks(b.sitesAvailable)
	if resolveErr != nil {
		return nil, domainError(10, "APACHE_SITE_WRITE_FAILED", "L’écriture du site Apache a échoué.", reason(resolveErr))
	}
	path := filepath.Join(root, *payload.Filename)
	if _, statErr := os.Lstat(path); statErr == nil || !os.IsNotExist(statErr) {
		return nil, domainError(5, "APACHE_SITE_ALREADY_EXISTS", "Un site Apache portant ce nom existe déjà.", nil)
	}
	if writeErr := b.atomicWrite(path, content); writeErr != nil {
		return nil, domainError(10, "APACHE_SITE_WRITE_FAILED", "L’écriture du site Apache a échoué.", reason(writeErr))
	}
	valid, message, validationErr := b.validateDisabled(ctx, path)
	if validationErr != nil {
		_ = os.Remove(path)
		return nil, validationErr
	}
	if !valid {
		_ = os.Remove(path)
		return nil, domainError(5, "APACHE_SITE_CONFIG_INVALID", "La configuration du site Apache est invalide.", outputDetails(message))
	}
	enabled := filepath.Clean(b.sitesAvailable) == filepath.Clean(b.sitesEnabled)
	if enabled {
		reloaded, output, reloadErr := b.reloadCommand(ctx)
		if reloadErr != nil || !reloaded {
			_ = os.Remove(path)
			if reloadErr != nil {
				return nil, reloadErr
			}
			return nil, domainError(10, "APACHE_SITE_RELOAD_FAILED", "La création du site Apache a été annulée après l’échec du rechargement.", outputDetails(output))
		}
	}
	return actionResponse("create", path, configIdentifier(path), enabled), nil
}

func (b *LinuxBackend) update(ctx context.Context, argument string) (map[string]any, *Error) {
	payload, err := parsePayload(argument)
	if err != nil {
		return nil, err
	}
	if payload.ConfigID == nil {
		return nil, domainError(2, "INVALID_APACHE_SITE_ID", "L’identifiant du site Apache est invalide.", nil)
	}
	path, err := b.sitePath(*payload.ConfigID)
	if err != nil {
		return nil, err
	}
	content, err := decodeContent(payload)
	if err != nil {
		return nil, err
	}
	enabled, err := b.siteEnabled(path)
	if err != nil {
		return nil, err
	}
	backup, err := b.backup(path)
	if err != nil {
		return nil, err
	}
	if writeErr := b.atomicWrite(path, content); writeErr != nil {
		return nil, domainError(10, "APACHE_SITE_WRITE_FAILED", "L’écriture du site Apache a échoué.", reason(writeErr))
	}
	var valid bool
	var message string
	var validationErr *Error
	if enabled {
		valid, message, validationErr = b.configTest(ctx)
	} else {
		valid, message, validationErr = b.validateDisabled(ctx, path)
	}
	if validationErr != nil {
		b.restore(path, backup)
		return nil, validationErr
	}
	if !valid {
		b.restore(path, backup)
		return nil, domainError(5, "APACHE_SITE_CONFIG_INVALID", "La configuration du site Apache est invalide.", outputDetails(message))
	}
	if enabled {
		reloaded, output, reloadErr := b.reloadCommand(ctx)
		if reloadErr != nil || !reloaded {
			b.restore(path, backup)
			_, _, _ = b.reloadCommand(ctx)
			if reloadErr != nil {
				return nil, reloadErr
			}
			return nil, domainError(10, "APACHE_SITE_RELOAD_FAILED", "Le site Apache a été restauré après l’échec du rechargement.", outputDetails(output))
		}
	}
	return actionResponse("update", path, *payload.ConfigID, enabled), nil
}

func (b *LinuxBackend) enable(ctx context.Context, identifier string) (map[string]any, *Error) {
	path, err := b.sitePath(identifier)
	if err != nil {
		return nil, err
	}
	enabled, err := b.siteEnabled(path)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, domainError(5, "APACHE_SITE_ALREADY_ENABLED", "Le site Apache est déjà activé.", nil)
	}
	changed, output, commandErr := b.siteCommand(ctx, b.enableCommand, filepath.Base(path))
	if commandErr != nil {
		return nil, commandErr
	}
	enabled, stateErr := b.siteEnabled(path)
	if stateErr != nil {
		return nil, stateErr
	}
	if !changed || !enabled {
		return nil, domainError(10, "APACHE_SITE_ENABLE_FAILED", "L’activation du site Apache a échoué.", outputDetails(output))
	}
	valid, message, validationErr := b.configTest(ctx)
	if validationErr != nil {
		return nil, validationErr
	}
	if !valid {
		_, _, _ = b.siteCommand(ctx, b.disableCommand, filepath.Base(path))
		return nil, domainError(5, "APACHE_SITE_CONFIG_INVALID", "Le site Apache n’a pas été activé car sa configuration est invalide.", outputDetails(message))
	}
	reloaded, reloadOutput, reloadErr := b.reloadCommand(ctx)
	if reloadErr != nil || !reloaded {
		_, _, _ = b.siteCommand(ctx, b.disableCommand, filepath.Base(path))
		_, _, _ = b.reloadCommand(ctx)
		if reloadErr != nil {
			return nil, reloadErr
		}
		return nil, domainError(10, "APACHE_SITE_RELOAD_FAILED", "L’activation du site Apache a été annulée après l’échec du rechargement.", outputDetails(reloadOutput))
	}
	return actionResponse("enable", path, identifier, true), nil
}

func (b *LinuxBackend) disable(ctx context.Context, identifier string) (map[string]any, *Error) {
	path, err := b.sitePath(identifier)
	if err != nil {
		return nil, err
	}
	enabled, err := b.siteEnabled(path)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, domainError(5, "APACHE_SITE_ALREADY_DISABLED", "Le site Apache est déjà désactivé.", nil)
	}
	if filepath.Clean(b.sitesAvailable) == filepath.Clean(b.sitesEnabled) {
		return nil, domainError(5, "APACHE_SITE_DISABLE_UNSUPPORTED", "Ce profil Apache active directement tous les fichiers de site ; la désactivation individuelle n’est pas disponible.", nil)
	}
	changed, output, commandErr := b.siteCommand(ctx, b.disableCommand, filepath.Base(path))
	if commandErr != nil {
		return nil, commandErr
	}
	enabled, stateErr := b.siteEnabled(path)
	if stateErr != nil {
		return nil, stateErr
	}
	if !changed || enabled {
		return nil, domainError(10, "APACHE_SITE_DISABLE_FAILED", "La désactivation du site Apache a échoué.", outputDetails(output))
	}
	valid, message, validationErr := b.configTest(ctx)
	if validationErr != nil {
		return nil, validationErr
	}
	if !valid {
		_, _, _ = b.siteCommand(ctx, b.enableCommand, filepath.Base(path))
		return nil, domainError(10, "APACHE_CONFIG_INVALID", "La désactivation du site Apache a produit une configuration invalide.", outputDetails(message))
	}
	reloaded, reloadOutput, reloadErr := b.reloadCommand(ctx)
	if reloadErr != nil || !reloaded {
		_, _, _ = b.siteCommand(ctx, b.enableCommand, filepath.Base(path))
		_, _, _ = b.reloadCommand(ctx)
		if reloadErr != nil {
			return nil, reloadErr
		}
		return nil, domainError(10, "APACHE_SITE_RELOAD_FAILED", "La désactivation du site Apache a été annulée après l’échec du rechargement.", outputDetails(reloadOutput))
	}
	return actionResponse("disable", path, identifier, false), nil
}

func (b *LinuxBackend) delete(ctx context.Context, identifier string) (map[string]any, *Error) {
	path, err := b.sitePath(identifier)
	if err != nil {
		return nil, err
	}
	enabled, err := b.siteEnabled(path)
	if err != nil {
		return nil, err
	}
	backup, err := b.backup(path)
	if err != nil {
		return nil, err
	}
	direct := filepath.Clean(b.sitesAvailable) == filepath.Clean(b.sitesEnabled)
	if enabled && !direct {
		changed, output, commandErr := b.siteCommand(ctx, b.disableCommand, filepath.Base(path))
		if commandErr != nil {
			return nil, commandErr
		}
		stillEnabled, stateErr := b.siteEnabled(path)
		if stateErr != nil {
			return nil, stateErr
		}
		if !changed || stillEnabled {
			return nil, domainError(10, "APACHE_SITE_DISABLE_FAILED", "La désactivation du site Apache a échoué.", outputDetails(output))
		}
	}
	if removeErr := os.Remove(path); removeErr != nil {
		if enabled && !direct {
			_, _, _ = b.siteCommand(ctx, b.enableCommand, filepath.Base(path))
		}
		return nil, domainError(10, "APACHE_SITE_DELETE_FAILED", "La suppression du site Apache a échoué.", reason(removeErr))
	}
	valid, message, validationErr := b.configTest(ctx)
	if validationErr != nil {
		b.restore(path, backup)
		return nil, validationErr
	}
	if !valid {
		b.restore(path, backup)
		if enabled && !direct {
			_, _, _ = b.siteCommand(ctx, b.enableCommand, filepath.Base(path))
		}
		return nil, domainError(10, "APACHE_CONFIG_INVALID", "La suppression du site Apache a été annulée car la configuration est invalide.", outputDetails(message))
	}
	if enabled {
		reloaded, output, reloadErr := b.reloadCommand(ctx)
		if reloadErr != nil || !reloaded {
			b.restore(path, backup)
			if !direct {
				_, _, _ = b.siteCommand(ctx, b.enableCommand, filepath.Base(path))
			}
			_, _, _ = b.reloadCommand(ctx)
			if reloadErr != nil {
				return nil, reloadErr
			}
			return nil, domainError(10, "APACHE_SITE_RELOAD_FAILED", "La suppression du site Apache a été annulée après l’échec du rechargement.", outputDetails(output))
		}
	}
	return actionResponse("delete", path, identifier, false), nil
}

func (b *LinuxBackend) reload(ctx context.Context) (map[string]any, *Error) {
	valid, message, err := b.configTest(ctx)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, domainError(10, "APACHE_CONFIG_INVALID", "La configuration Apache est invalide. Le rechargement a été refusé.", outputDetails(message))
	}
	result, commandErr := b.runner.Run(ctx, systemctlCommand, "reload", "--", b.service+".service")
	if commandErr != nil {
		return nil, commandErr
	}
	if !result.OK {
		return nil, domainError(10, "APACHE_RELOAD_FAILED", "Le rechargement d’Apache a échoué.", outputDetails(result.Output))
	}
	return map[string]any{"action": "reload", "service": b.service, "result": "success"}, nil
}

func (b *LinuxBackend) restart(ctx context.Context) (map[string]any, *Error) {
	valid, message, err := b.configTest(ctx)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, domainError(10, "APACHE_CONFIG_INVALID", "La configuration Apache est invalide. Le redémarrage a été refusé.", outputDetails(message))
	}
	result, commandErr := b.runner.Run(ctx, systemdRunCommand, "--quiet", "--collect", "--on-active=5s", systemctlCommand, "restart", "--", b.service+".service")
	if commandErr != nil {
		return nil, commandErr
	}
	if !result.OK {
		return nil, domainError(10, "APACHE_RESTART_SCHEDULE_FAILED", "La programmation du redémarrage d’Apache a échoué.", outputDetails(result.Output))
	}
	return map[string]any{"action": "restart", "service": b.service, "result": "scheduled", "delay_seconds": 5}, nil
}
