ALTER TABLE users ADD COLUMN two_factor_required INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN totp_secret TEXT;
ALTER TABLE users ADD COLUMN totp_enabled_at TEXT;

CREATE TRIGGER users_validate_two_factor_insert
BEFORE INSERT ON users
FOR EACH ROW
WHEN NEW.two_factor_required NOT IN (0, 1)
BEGIN
    SELECT RAISE(ABORT, 'Le paramètre de double authentification est invalide.');
END;

CREATE TRIGGER users_validate_two_factor_update
BEFORE UPDATE OF two_factor_required, totp_secret, totp_enabled_at ON users
FOR EACH ROW
WHEN NEW.two_factor_required NOT IN (0, 1)
  OR (NEW.totp_enabled_at IS NOT NULL AND NEW.totp_secret IS NULL)
BEGIN
    SELECT RAISE(ABORT, 'La configuration de double authentification est invalide.');
END;
