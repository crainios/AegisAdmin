CREATE TABLE application_settings (
    setting_key TEXT PRIMARY KEY,
    setting_value TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,

    CONSTRAINT application_settings_key_length CHECK (
        length(setting_key) BETWEEN 2 AND 64
    ),
    CONSTRAINT application_settings_value_length CHECK (
        length(setting_value) <= 2048
    )
);

INSERT INTO navigation_modules (
    module_key,
    name,
    route,
    icon,
    category_id,
    position,
    is_enabled,
    is_essential,
    access_policy,
    created_at,
    updated_at
)
SELECT
    'setting',
    'Paramètres',
    '/setting',
    '⚙',
    category.id,
    COALESCE(
        (
            SELECT MAX(position) + 1
            FROM navigation_modules
            WHERE category_id = category.id
        ),
        0
    ),
    1,
    1,
    'root',
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM navigation_categories AS category
INNER JOIN navigation_modules AS anchor
    ON anchor.category_id = category.id
WHERE anchor.module_key = 'modules'
LIMIT 1;
