package mysqlsetup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const monitorLogin = "aegisadmin_monitor"
const inventorySchema = "aegisadmin_monitoring"
const inventoryProcedure = "database_inventory_v2"
const inventoryCheckSQL = "CALL aegisadmin_monitoring.database_inventory_v2()"

var ErrAdministratorAuthentication = errors.New("mysql administrator authentication failed")
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

type Administrator struct {
	Login, Password string
}

type Configurator struct {
	Clients         []string
	CredentialsPath string
}

func New() Configurator {
	return Configurator{
		Clients:         []string{"/usr/bin/mysql", "/usr/bin/mariadb", "/usr/local/bin/mysql", "/usr/local/bin/mariadb"},
		CredentialsPath: "/etc/aegisadmin-system/mysql-client.cnf",
	}
}

func (c Configurator) Configure(ctx context.Context, administrator *Administrator) (bool, error) {
	client, err := executable(c.Clients)
	if err != nil {
		return false, err
	}
	if secureRegular(c.CredentialsPath) {
		if err = run(ctx, client, "--defaults-extra-file="+c.CredentialsPath, "--batch", "--skip-column-names", "--execute", inventoryCheckSQL); err == nil {
			return false, nil
		}
	}
	password, err := randomPassword()
	if err != nil {
		return false, err
	}
	statement := monitoringSQL(password)
	arguments := []string{}
	temporary := ""
	if administrator != nil {
		if !loginPattern.MatchString(administrator.Login) {
			return false, errors.New("invalid MySQL administrator login")
		}
		temporary, err = temporaryCredentials(filepath.Dir(c.CredentialsPath), administrator.Login, administrator.Password)
		if err != nil {
			return false, err
		}
		defer os.Remove(temporary)
		arguments = append(arguments, "--defaults-extra-file="+temporary)
	}
	arguments = append(arguments, "--batch", "--protocol=socket", "--execute", statement)
	if err = run(ctx, client, arguments...); err != nil {
		return false, fmt.Errorf("%w: %v", ErrAdministratorAuthentication, err)
	}
	content := clientConfiguration(monitorLogin, password)
	if err = atomicWrite(c.CredentialsPath, []byte(content)); err != nil {
		return false, err
	}
	return true, nil
}

func executable(paths []string) (string, error) {
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("aucun client MySQL ou MariaDB n’est installé")
}

func run(ctx context.Context, client string, arguments ...string) error {
	commandContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(commandContext, client, arguments...)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := command.CombinedOutput()
	if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
		return errors.New("la commande MySQL a dépassé le délai autorisé")
	}
	if err != nil {
		message := strings.TrimSpace(strings.ToValidUTF8(string(output), "�"))
		if message == "" {
			message = "commande refusée"
		}
		return errors.New(message)
	}
	return nil
}

func randomPassword() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate MySQL monitoring password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func monitoringSQL(password string) string {
	return "CREATE USER IF NOT EXISTS '" + monitorLogin + "'@'localhost' IDENTIFIED BY '" + password + "';" +
		"ALTER USER '" + monitorLogin + "'@'localhost' IDENTIFIED BY '" + password + "';" +
		"CREATE DATABASE IF NOT EXISTS " + inventorySchema + ";" +
		"DROP VIEW IF EXISTS " + inventorySchema + ".database_inventory;" +
		"DROP PROCEDURE IF EXISTS " + inventorySchema + "." + inventoryProcedure + ";" +
		"CREATE PROCEDURE " + inventorySchema + "." + inventoryProcedure + "() SQL SECURITY DEFINER " +
		"SELECT s.SCHEMA_NAME AS database_name, COALESCE(SUM(t.DATA_LENGTH + t.INDEX_LENGTH), 0) AS size_bytes, COUNT(t.TABLE_NAME) AS table_count " +
		"FROM information_schema.SCHEMATA AS s LEFT JOIN information_schema.TABLES AS t ON t.TABLE_SCHEMA = s.SCHEMA_NAME " +
		"WHERE s.SCHEMA_NAME <> '" + inventorySchema + "' GROUP BY s.SCHEMA_NAME;" +
		"GRANT PROCESS ON *.* TO '" + monitorLogin + "'@'localhost';" +
		"GRANT EXECUTE ON PROCEDURE " + inventorySchema + "." + inventoryProcedure + " TO '" + monitorLogin + "'@'localhost';FLUSH PRIVILEGES;"
}

func clientConfiguration(login, password string) string {
	return "[client]\nuser=" + optionValue(login) + "\npassword=" + optionValue(password) + "\nprotocol=socket\n"
}

func optionValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + replacer.Replace(value) + `"`
}

func temporaryCredentials(directory, login, password string) (string, error) {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, ".mysql-administrator-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err = file.Chmod(0o600); err == nil {
		_, err = file.WriteString(clientConfiguration(login, password))
	}
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	return path, nil
}

func atomicWrite(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".mysql-client-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err = file.Chmod(0o600); err == nil {
		_, err = file.Write(content)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func secureRegular(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0
}
