CREATE TABLE user_module_permissions (
    user_id INTEGER NOT NULL,
    module_id INTEGER NOT NULL,
    permission_level TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,

    PRIMARY KEY (user_id, module_id),
    CONSTRAINT user_module_permissions_level_valid CHECK (
        permission_level IN ('view', 'action', 'modify')
    ),
    CONSTRAINT user_module_permissions_user_foreign_key FOREIGN KEY (user_id)
        REFERENCES users (id) ON UPDATE RESTRICT ON DELETE CASCADE,
    CONSTRAINT user_module_permissions_module_foreign_key FOREIGN KEY (module_id)
        REFERENCES navigation_modules (id) ON UPDATE RESTRICT ON DELETE CASCADE
);

CREATE INDEX user_module_permissions_module
    ON user_module_permissions (module_id, user_id);

INSERT INTO user_module_permissions (
    user_id,
    module_id,
    permission_level,
    created_at,
    updated_at
)
SELECT
    u.id,
    m.id,
    CASE
        WHEN u.can_modify = 1 THEN 'modify'
        WHEN u.can_act = 1 THEN 'action'
        ELSE 'view'
    END,
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM users u
CROSS JOIN navigation_modules m
WHERE u.type = 'user'
  AND u.can_view = 1
  AND m.is_enabled = 1
  AND m.access_policy = 'view';
