package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var idPattern = regexp.MustCompile(`^[a-f0-9-]{36}$`)
var (
	hostPattern         = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	userPattern         = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	remotePathPattern   = regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`)
	databaseListPattern = regexp.MustCompile(`^[A-Za-z0-9_$-]+(?:,[A-Za-z0-9_$-]+)*$`)
)

type Task struct {
	ID, Name, Kind, Source, Destination        string
	RemoteHost, RemoteUser, RemotePath, SSHKey string
	RemotePort, RetentionDays                  int
	RemoveLocal                                bool
}

func Run(id, configDirectory string) error {
	if !idPattern.MatchString(id) {
		return errors.New("identifiant de sauvegarde invalide")
	}
	data, err := os.ReadFile(filepath.Join(configDirectory, id+".json"))
	if err != nil {
		return err
	}
	var task Task
	if err = json.Unmarshal(data, &task); err != nil || task.ID != id {
		return errors.New("configuration de sauvegarde invalide")
	}
	if err = Validate(task); err != nil {
		return err
	}
	if err = os.MkdirAll(task.Destination, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(task.Destination, ".aegisadmin-backup.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("une sauvegarde est déjà en cours")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	stamp := time.Now().UTC().Format("20060102T150405Z")
	base := safeName(task.Name) + "-" + stamp
	var archive string
	switch task.Kind {
	case "mysql":
		archive, err = mysqlDump(task, base)
	case "apache", "sites":
		archive, err = tarSource(task, base)
	default:
		err = errors.New("type de sauvegarde inconnu")
	}
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(archive); statErr != nil || info.Size() == 0 {
		return errors.New("l’archive produite est vide ou illisible")
	}
	if task.RemoteHost != "" {
		if err = transfer(task, archive); err != nil {
			return err
		}
		if task.RemoveLocal {
			if err = os.Remove(archive); err != nil {
				return err
			}
		}
	}
	return purge(task)
}

func Validate(t Task) error {
	if !idPattern.MatchString(t.ID) || t.Name == "" || t.RetentionDays < 0 || t.RetentionDays > 3650 {
		return errors.New("paramètres de sauvegarde invalides")
	}
	for _, v := range []string{t.Source, t.Destination, t.RemoteHost, t.RemoteUser, t.RemotePath, t.SSHKey} {
		if strings.ContainsAny(v, "\x00\r\n") {
			return errors.New("paramètres de sauvegarde invalides")
		}
	}
	if !filepath.IsAbs(t.Destination) || (t.Kind != "mysql" && !filepath.IsAbs(t.Source)) {
		return errors.New("les chemins locaux doivent être absolus")
	}
	cleanDestination := filepath.Clean(t.Destination)
	if cleanDestination != "/var/backups/aegisadmin" && !strings.HasPrefix(cleanDestination, "/var/backups/aegisadmin/") {
		return errors.New("le stockage local doit se trouver sous /var/backups/aegisadmin")
	}
	if t.Kind == "apache" && filepath.Clean(t.Source) != "/etc/apache2" {
		return errors.New("la source Apache doit être /etc/apache2")
	}
	if t.Kind == "sites" && filepath.Clean(t.Source) != "/var/www" && !strings.HasPrefix(filepath.Clean(t.Source), "/var/www/") {
		return errors.New("la source du site doit se trouver sous /var/www")
	}
	if t.Kind == "mysql" && t.Source != "" && t.Source != "all" && !databaseListPattern.MatchString(t.Source) {
		return errors.New("la liste des bases est invalide")
	}
	if t.RemoteHost != "" && (!hostPattern.MatchString(t.RemoteHost) || !userPattern.MatchString(t.RemoteUser) || !remotePathPattern.MatchString(t.RemotePath) || t.RemotePort < 1 || t.RemotePort > 65535) {
		return errors.New("destination distante incomplète")
	}
	cleanKey := filepath.Clean(t.SSHKey)
	if !strings.HasPrefix(cleanKey, "/etc/aegisadmin-system/backup-ssh/") {
		return errors.New("la clé SSH doit se trouver dans le répertoire sécurisé AegisAdmin")
	}
	return nil
}

func safeName(v string) string {
	r := regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(v, "-")
	r = strings.Trim(r, "-.")
	if r == "" {
		return "backup"
	}
	return r
}

func mysqlDump(t Task, base string) (string, error) {
	path := filepath.Join(t.Destination, base+".sql.gz")
	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(out)
	args := []string{"--single-transaction", "--routines", "--events", "--triggers", "--all-databases"}
	if t.Source != "" && t.Source != "all" {
		args = []string{"--single-transaction", "--routines", "--events", "--triggers", "--databases"}
		args = append(args, strings.Split(t.Source, ",")...)
	}
	cmd := exec.Command("/usr/bin/mysqldump", args...)
	cmd.Stdout = gz
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("mysqldump: %w", err)
	}
	err = os.Rename(tmp, path)
	return path, err
}

func tarSource(t Task, base string) (string, error) {
	info, err := os.Stat(t.Source)
	if err != nil || !info.IsDir() {
		return "", errors.New("source absente ou invalide")
	}
	path := filepath.Join(t.Destination, base+".tar.gz")
	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	err = filepath.Walk(t.Source, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(filepath.Dir(t.Source), p)
		if e != nil {
			return e
		}
		h, e := tar.FileInfoHeader(i, "")
		if e != nil {
			return e
		}
		h.Name = rel
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		if !i.Mode().IsRegular() {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = io.Copy(tw, f)
		return e
	})
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := out.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	err = os.Rename(tmp, path)
	return path, err
}

func transfer(t Task, archive string) error {
	destination := t.RemoteUser + "@" + t.RemoteHost + ":" + strings.TrimRight(t.RemotePath, "/") + "/"
	ssh := fmt.Sprintf("ssh -i %s -p %d -o BatchMode=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/etc/aegisadmin-system/backup-ssh/known_hosts", t.SSHKey, t.RemotePort)
	cmd := exec.Command("/usr/bin/rsync", "--archive", "--partial", "--protect-args", "--rsh", ssh, "--", archive, destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("transfert rsync: %w", err)
	}
	return nil
}

func purge(t Task) error {
	if t.RetentionDays == 0 {
		return nil
	}
	entries, err := os.ReadDir(t.Destination)
	if err != nil {
		return err
	}
	limit := time.Now().Add(-time.Duration(t.RetentionDays) * 24 * time.Hour)
	prefix := safeName(t.Name) + "-"
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		i, er := e.Info()
		if er == nil && i.Mode().IsRegular() && i.ModTime().Before(limit) {
			_ = os.Remove(filepath.Join(t.Destination, e.Name()))
		}
	}
	return nil
}

func ParseBool(v string) bool { b, _ := strconv.ParseBool(v); return b }
