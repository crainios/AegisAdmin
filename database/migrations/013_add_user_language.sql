ALTER TABLE user_preferences
ADD COLUMN language TEXT NOT NULL DEFAULT 'fr'
    CHECK (language IN ('fr', 'en'));
