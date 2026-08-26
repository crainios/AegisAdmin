package fail2ban

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var fail2banUnitPattern = regexp.MustCompile(`^fail2ban(?:[-@][A-Za-z0-9_.-]+)?\.service$`)

func (b *Backend) loadProfile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "client":
			if value == "/usr/bin/fail2ban-client" || value == "/usr/local/bin/fail2ban-client" {
				b.client = value
			}
		case "unit":
			if fail2banUnitPattern.MatchString(value) {
				b.unit = value
				b.service = strings.TrimSuffix(value, ".service")
			}
		}
	}
}
