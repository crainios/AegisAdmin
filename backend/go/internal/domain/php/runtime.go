package php

import (
	"context"
	"encoding/json"
	"path/filepath"
)

const infoSource = `echo json_encode(['version' => PHP_VERSION, 'sapi' => PHP_SAPI, 'ini_file' => php_ini_loaded_file() ?: null, 'scan_dir' => PHP_CONFIG_FILE_SCAN_DIR ?: null], JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);`

const configurationSource = `$names = json_decode('["memory_limit","max_execution_time","max_input_time","post_max_size","upload_max_filesize","max_file_uploads","date.timezone","display_errors","log_errors","error_log","opcache.enable","opcache.memory_consumption"]', true); $directives = []; foreach ($names as $name) { $value = ini_get($name); $directives[$name] = $value === false ? null : $value; } echo json_encode(['ini_file' => php_ini_loaded_file() ?: null, 'scan_dir' => PHP_CONFIG_FILE_SCAN_DIR ?: null, 'directives' => $directives], JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);`

const extensionsSource = `$extensions = get_loaded_extensions(); natcasesort($extensions); echo json_encode(array_values($extensions), JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);`

func (b *Backend) requireRuntime(ctx context.Context, identifier string) (runtime, *Error) {
	if identifier == "cli" {
		binary := b.binary("php")
		if !executable(binary) {
			return runtime{}, domainError(4, "PHP_CLI_NOT_FOUND", "L’interpréteur PHP en ligne de commande est introuvable.", nil)
		}
		return runtime{ID: "cli", Type: "cli", Binary: binary}, nil
	}
	match := fpmRuntimePattern.FindStringSubmatch(identifier)
	if match == nil {
		return runtime{}, domainError(2, "INVALID_PHP_RUNTIME_ID", "L’identifiant d’exécution PHP est invalide.", nil)
	}
	instances, err := b.instances(ctx)
	if err != nil {
		return runtime{}, err
	}
	for index := range instances {
		if instances[index].ID != identifier {
			continue
		}
		binary := instances[index].binary
		if !executable(binary) {
			return runtime{}, domainError(4, "PHP_RUNTIME_BINARY_NOT_FOUND", "L’interpréteur correspondant à cette instance PHP-FPM est introuvable.", nil)
		}
		return runtime{ID: identifier, Type: "fpm", Binary: binary, Version: instances[index].Version, Instance: &instances[index]}, nil
	}
	return runtime{}, domainError(4, "PHP_RUNTIME_NOT_FOUND", "L’instance PHP-FPM demandée n’est pas installée.", nil)
}

func (b *Backend) query(ctx context.Context, selected runtime, source string) (any, *Error) {
	arguments := []string{}
	environment := map[string]string{}
	if selected.Type == "fpm" && selected.Instance != nil && selected.Instance.debianLayout {
		configurationRoot := filepath.Join("/etc/php", selected.Version, "fpm")
		arguments = append(arguments, "-c", filepath.Join(configurationRoot, "php.ini"))
		environment["PHP_INI_SCAN_DIR"] = filepath.Join(configurationRoot, "conf.d")
	}
	arguments = append(arguments, "-r", source)
	result, err := b.runner.Run(ctx, environment, selected.Binary, arguments...)
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, domainError(10, "PHP_RUNTIME_QUERY_FAILED", "Les informations de l’exécution PHP sont indisponibles.", map[string]any{"output": result.Output})
	}
	var data any
	if jsonErr := json.Unmarshal([]byte(result.Output), &data); jsonErr != nil {
		return nil, domainError(10, "INVALID_PHP_RUNTIME_RESPONSE", "La réponse retournée par PHP est invalide.", nil)
	}
	switch data.(type) {
	case map[string]any, []any:
		return data, nil
	default:
		return nil, domainError(10, "INVALID_PHP_RUNTIME_RESPONSE", "La réponse retournée par PHP est invalide.", nil)
	}
}

func (b *Backend) info(ctx context.Context) (map[string]any, *Error) {
	selected, err := b.requireRuntime(ctx, "cli")
	if err != nil {
		return nil, err
	}
	data, err := b.query(ctx, selected, infoSource)
	if err != nil {
		return nil, err
	}
	values, ok := data.(map[string]any)
	if !ok {
		return nil, domainError(10, "INVALID_PHP_RUNTIME_RESPONSE", "La réponse retournée par PHP est invalide.", nil)
	}
	result := map[string]any{"runtime": "cli"}
	for key, value := range values {
		result[key] = value
	}
	return result, nil
}

func (b *Backend) configuration(ctx context.Context, identifier string) (map[string]any, *Error) {
	selected, err := b.requireRuntime(ctx, identifier)
	if err != nil {
		return nil, err
	}
	data, err := b.query(ctx, selected, configurationSource)
	if err != nil {
		return nil, err
	}
	values, ok := data.(map[string]any)
	if !ok {
		return nil, domainError(10, "INVALID_PHP_RUNTIME_RESPONSE", "La réponse retournée par PHP est invalide.", nil)
	}
	result := map[string]any{"runtime": identifier}
	for key, value := range values {
		result[key] = value
	}
	return result, nil
}

func (b *Backend) extensions(ctx context.Context, identifier string) (map[string]any, *Error) {
	selected, err := b.requireRuntime(ctx, identifier)
	if err != nil {
		return nil, err
	}
	data, err := b.query(ctx, selected, extensionsSource)
	if err != nil {
		return nil, err
	}
	extensions, ok := data.([]any)
	if !ok {
		return nil, domainError(10, "INVALID_PHP_RUNTIME_RESPONSE", "La réponse retournée par PHP est invalide.", nil)
	}
	return map[string]any{"runtime": identifier, "extensions": extensions}, nil
}

func (b *Backend) restart(ctx context.Context, identifier string) (map[string]any, *Error) {
	selected, err := b.requireRuntime(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if selected.Type != "fpm" {
		return nil, domainError(2, "PHP_RUNTIME_NOT_RESTARTABLE", "Seule une instance PHP-FPM peut être redémarrée.", nil)
	}
	if selected.Instance == nil || !selected.Instance.Exists {
		return nil, domainError(4, "PHP_FPM_SERVICE_NOT_FOUND", "Le service PHP-FPM demandé est introuvable.", nil)
	}
	result, commandErr := b.runner.Run(ctx, nil, systemdRunCommand, "--quiet", "--collect", "--on-active=5s", systemctlCommand, "restart", "--", selected.Instance.Unit)
	if commandErr != nil {
		return nil, commandErr
	}
	if !result.OK {
		return nil, domainError(10, "PHP_FPM_RESTART_SCHEDULE_FAILED", "La programmation du redémarrage de PHP-FPM a échoué.", map[string]any{"output": result.Output})
	}
	return map[string]any{"action": "restart", "runtime": identifier, "service": selected.Instance.Service, "result": "scheduled", "delay_seconds": 5}, nil
}
