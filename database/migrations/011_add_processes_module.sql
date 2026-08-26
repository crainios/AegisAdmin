INSERT INTO navigation_modules (
    module_key, name, route, icon, category_id, position,
    is_enabled, is_essential, access_policy, created_at, updated_at
)
SELECT
    'processes',
    'Processus',
    '/processes',
    '≋',
    category.id,
    COALESCE((SELECT MAX(position) + 1 FROM navigation_modules WHERE category_id = category.id), 0),
    1,
    0,
    'view',
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM navigation_categories category
WHERE category.name = 'Supervision'
  AND NOT EXISTS (SELECT 1 FROM navigation_modules WHERE module_key = 'processes')
LIMIT 1;
