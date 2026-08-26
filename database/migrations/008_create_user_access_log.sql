CREATE TABLE user_access_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    login TEXT NOT NULL,
    event TEXT NOT NULL CHECK (event IN (
        'login_success', 'login_failure', 'two_factor_success',
        'two_factor_failure', 'logout', 'password_changed',
        'two_factor_enabled', 'two_factor_disabled'
    )),
    success INTEGER NOT NULL CHECK (success IN (0, 1)),
    ip_address TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    occurred_at TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX user_access_log_occurred_at_idx ON user_access_log(occurred_at DESC);
CREATE INDEX user_access_log_user_id_idx ON user_access_log(user_id, occurred_at DESC);
CREATE INDEX user_access_log_ip_idx ON user_access_log(ip_address, occurred_at DESC);
