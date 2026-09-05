CREATE TABLE user_preferences (
    user_id INTEGER PRIMARY KEY,
    theme TEXT NOT NULL DEFAULT 'dark',
    updated_at TEXT NOT NULL,

    CONSTRAINT user_preferences_user_fk
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT user_preferences_theme_valid
        CHECK (theme IN ('dark', 'light', 'bootstrap', 'neon'))
);
