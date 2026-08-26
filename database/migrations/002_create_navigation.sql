CREATE TABLE navigation_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL COLLATE NOCASE,
    position INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,

    CONSTRAINT navigation_categories_name_unique UNIQUE (name),
    CONSTRAINT navigation_categories_name_length CHECK (
        length(trim(name)) BETWEEN 2 AND 64
    ),
    CONSTRAINT navigation_categories_position_valid CHECK (
        position >= 0
    )
);

CREATE TABLE navigation_modules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_key TEXT NOT NULL COLLATE NOCASE,
    name TEXT NOT NULL COLLATE NOCASE,
    route TEXT NOT NULL,
    icon TEXT NOT NULL,
    category_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    is_essential INTEGER NOT NULL DEFAULT 0,
    access_policy TEXT NOT NULL DEFAULT 'view',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,

    CONSTRAINT navigation_modules_key_unique UNIQUE (module_key),
    CONSTRAINT navigation_modules_route_unique UNIQUE (route),
    CONSTRAINT navigation_modules_key_length CHECK (
        length(module_key) BETWEEN 2 AND 64
    ),
    CONSTRAINT navigation_modules_name_length CHECK (
        length(trim(name)) BETWEEN 2 AND 64
    ),
    CONSTRAINT navigation_modules_route_valid CHECK (
        length(route) >= 2
        AND substr(route, 1, 1) = '/'
    ),
    CONSTRAINT navigation_modules_icon_required CHECK (
        length(trim(icon)) >= 1
    ),
    CONSTRAINT navigation_modules_position_valid CHECK (
        position >= 0
    ),
    CONSTRAINT navigation_modules_enabled_boolean CHECK (
        is_enabled IN (0, 1)
    ),
    CONSTRAINT navigation_modules_essential_boolean CHECK (
        is_essential IN (0, 1)
    ),
    CONSTRAINT navigation_modules_essential_enabled CHECK (
        is_essential = 0 OR is_enabled = 1
    ),
    CONSTRAINT navigation_modules_access_policy_valid CHECK (
        access_policy IN ('view', 'root')
    ),
    CONSTRAINT navigation_modules_category_foreign_key FOREIGN KEY (
        category_id
    ) REFERENCES navigation_categories (id)
        ON UPDATE RESTRICT
        ON DELETE RESTRICT
);

CREATE INDEX navigation_categories_position
    ON navigation_categories (position, id);

CREATE INDEX navigation_modules_category_position
    ON navigation_modules (category_id, position, id);

CREATE INDEX navigation_modules_enabled
    ON navigation_modules (is_enabled);

CREATE TRIGGER navigation_modules_protect_essential_update
BEFORE UPDATE OF is_enabled, is_essential ON navigation_modules
FOR EACH ROW
WHEN OLD.is_essential = 1
    AND (
        NEW.is_enabled != 1
        OR NEW.is_essential != 1
    )
BEGIN
    SELECT RAISE(
        ABORT,
        'Un module essentiel ne peut pas être désactivé ou déprotégé.'
    );
END;

CREATE TRIGGER navigation_modules_prevent_deletion
BEFORE DELETE ON navigation_modules
FOR EACH ROW
BEGIN
    SELECT RAISE(
        ABORT,
        'Les modules de navigation ne peuvent pas être supprimés.'
    );
END;

INSERT INTO navigation_categories (
    id,
    name,
    position,
    created_at,
    updated_at
) VALUES
    (
        1,
        'Supervision',
        0,
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        2,
        'Sécurité',
        1,
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        3,
        'Administration',
        2,
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        4,
        'Maintenance',
        3,
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        5,
        'Information',
        4,
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    );

INSERT INTO navigation_modules (
    id,
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
) VALUES
    (
        1,
        'dashboard',
        'Dashboard',
        '/dashboard',
        '◫',
        1,
        0,
        1,
        1,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        2,
        'storage',
        'Stockage',
        '/storage',
        '▤',
        1,
        1,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        3,
        'services',
        'Services',
        '/services',
        '⚙',
        1,
        2,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        4,
        'cron',
        'Cron',
        '/cron',
        '◷',
        1,
        3,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        5,
        'network',
        'Réseau',
        '/network',
        '⇄',
        1,
        4,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        6,
        'apache',
        'Apache',
        '/apache',
        '◉',
        1,
        5,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        7,
        'certbot',
        'Certificats TLS',
        '/certbot',
        '▣',
        1,
        6,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        8,
        'php',
        'PHP-FPM',
        '/php',
        '</>',
        1,
        7,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        9,
        'mysql',
        'MySQL',
        '/mysql',
        '◇',
        1,
        8,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        10,
        'tor',
        'Tor',
        '/tor',
        '◎',
        1,
        9,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        11,
        'fail2ban',
        'Fail2ban',
        '/fail2ban',
        '⛨',
        2,
        0,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        12,
        'firewall',
        'Pare-feu',
        '/firewall',
        '▦',
        2,
        1,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        13,
        'users',
        'Utilisateurs',
        '/users',
        '♙',
        3,
        0,
        1,
        1,
        'root',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        14,
        'modules',
        'Modules',
        '/modules',
        '⊞',
        3,
        1,
        1,
        1,
        'root',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        15,
        'logs',
        'Journaux',
        '/logs',
        '≡',
        4,
        0,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        16,
        'updates',
        'Mises à jour',
        '/updates',
        '↻',
        4,
        1,
        1,
        0,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    ),
    (
        17,
        'about',
        'À propos de…',
        '/about',
        'ⓘ',
        5,
        0,
        1,
        1,
        'view',
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    );
