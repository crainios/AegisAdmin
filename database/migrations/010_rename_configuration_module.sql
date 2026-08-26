UPDATE navigation_modules
SET name = 'Config. serveur',
    updated_at = CURRENT_TIMESTAMP
WHERE module_key = 'configuration';
