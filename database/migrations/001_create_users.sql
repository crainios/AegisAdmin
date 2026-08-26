CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    login TEXT NOT NULL COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    can_view INTEGER NOT NULL DEFAULT 1,
    can_act INTEGER NOT NULL DEFAULT 0,
    can_modify INTEGER NOT NULL DEFAULT 0,
    must_change_password INTEGER NOT NULL DEFAULT 1,
    auth_version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_login_at TEXT,
    password_changed_at TEXT NOT NULL,

    CONSTRAINT users_login_unique UNIQUE (login),
    CONSTRAINT users_login_length CHECK (
        length(login) BETWEEN 3 AND 64
    ),
    CONSTRAINT users_type_valid CHECK (
        type IN ('root', 'user')
    ),
    CONSTRAINT users_status_valid CHECK (
        status IN ('active', 'suspended')
    ),
    CONSTRAINT users_can_view_boolean CHECK (
        can_view IN (0, 1)
    ),
    CONSTRAINT users_can_act_boolean CHECK (
        can_act IN (0, 1)
    ),
    CONSTRAINT users_can_modify_boolean CHECK (
        can_modify IN (0, 1)
    ),
    CONSTRAINT users_must_change_password_boolean CHECK (
        must_change_password IN (0, 1)
    ),
    CONSTRAINT users_auth_version_positive CHECK (
        auth_version >= 1
    ),
    CONSTRAINT users_permissions_require_view CHECK (
        can_view = 1 OR (can_act = 0 AND can_modify = 0)
    ),
    CONSTRAINT users_root_protected CHECK (
        type != 'root' OR (
            status = 'active'
            AND can_view = 1
            AND can_act = 1
            AND can_modify = 1
        )
    )
);

CREATE UNIQUE INDEX users_single_root
    ON users (type)
    WHERE type = 'root';

CREATE TRIGGER users_prevent_root_deletion
BEFORE DELETE ON users
FOR EACH ROW
WHEN OLD.type = 'root'
BEGIN
    SELECT RAISE(ABORT, 'Le compte root ne peut pas être supprimé.');
END;

CREATE TRIGGER users_prevent_root_demotion
BEFORE UPDATE OF type, status, can_view, can_act, can_modify ON users
FOR EACH ROW
WHEN OLD.type = 'root'
    AND (
        NEW.type != 'root'
        OR NEW.status != 'active'
        OR NEW.can_view != 1
        OR NEW.can_act != 1
        OR NEW.can_modify != 1
    )
BEGIN
    SELECT RAISE(ABORT, 'Le compte root ne peut pas être suspendu ou rétrogradé.');
END;
