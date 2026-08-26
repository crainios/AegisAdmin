INSERT INTO navigation_modules (
    module_key, name, route, icon, category_id, position,
    is_enabled, is_essential, access_policy, created_at, updated_at
)
SELECT
    'configuration',
    'Configuration du serveur',
    '/configuration',
    '⌘',
    category_id,
    COALESCE((SELECT MAX(position) + 1 FROM navigation_modules sibling WHERE sibling.category_id = anchor.category_id), 0),
    1,
    0,
    'root',
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM navigation_modules anchor
WHERE anchor.module_key = 'updates'
LIMIT 1;
