package downloadstats

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	requestPattern = regexp.MustCompile(`"GET ([^ ?"]+)(?:\?[^ "]*)? HTTP/[0-9.]+" ([0-9]{3}) ([0-9-]+)`)
	datePattern    = regexp.MustCompile(`\[([0-9]{2}/[A-Za-z]{3}/[0-9]{4}):`)
	packagePattern = regexp.MustCompile(`(?:^|/)aegisadmin_([^/_]+)_([^/_]+)\.deb$`)
)

type Day struct {
	Date      string `json:"date"`
	Downloads int64  `json:"downloads"`
}

type Release struct {
	Version         string `json:"version"`
	Architecture    string `json:"architecture"`
	APTDownloads    int64  `json:"apt_downloads"`
	GitHubDownloads int64  `json:"github_downloads"`
}

type Stats struct {
	GeneratedAt          string    `json:"generated_at"`
	TotalAPTDownloads    int64     `json:"total_apt_downloads"`
	TotalGitHubDownloads int64     `json:"total_github_downloads"`
	GitHubAvailable      bool      `json:"github_available"`
	GitHubMessage        string    `json:"github_message,omitempty"`
	Days                 []Day     `json:"days"`
	Releases             []Release `json:"releases"`
	FilesRead            int       `json:"files_read"`
}

type accumulator struct {
	days     map[string]int64
	releases map[string]int64
	total    int64
	files    int
}

func Collect(pattern string, now time.Time) (Stats, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return Stats{}, fmt.Errorf("invalid log pattern: %w", err)
	}
	if len(paths) == 0 {
		return Stats{}, fmt.Errorf("no Apache log matches %q", pattern)
	}
	sort.Strings(paths)
	a := accumulator{days: map[string]int64{}, releases: map[string]int64{}}
	for _, path := range paths {
		if err := a.read(path); err != nil {
			return Stats{}, err
		}
	}
	stats := Stats{
		GeneratedAt:       now.UTC().Format(time.RFC3339),
		TotalAPTDownloads: a.total,
		Days:              []Day{},
		Releases:          []Release{},
		FilesRead:         a.files,
	}
	for date, count := range a.days {
		stats.Days = append(stats.Days, Day{Date: date, Downloads: count})
	}
	sort.Slice(stats.Days, func(i, j int) bool { return stats.Days[i].Date < stats.Days[j].Date })
	for key, count := range a.releases {
		parts := strings.SplitN(key, "\x00", 2)
		stats.Releases = append(stats.Releases, Release{Version: parts[0], Architecture: parts[1], APTDownloads: count})
	}
	sort.Slice(stats.Releases, func(i, j int) bool {
		if stats.Releases[i].Version == stats.Releases[j].Version {
			return stats.Releases[i].Architecture < stats.Releases[j].Architecture
		}
		return versionLess(stats.Releases[j].Version, stats.Releases[i].Version)
	})
	return stats, nil
}

func (a *accumulator) read(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var reader io.Reader = file
	if strings.HasSuffix(path, ".gz") {
		compressed, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("read gzip %s: %w", path, err)
		}
		defer compressed.Close()
		reader = compressed
	}
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		a.add(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	a.files++
	return nil
}

func (a *accumulator) add(line string) {
	request := requestPattern.FindStringSubmatch(line)
	date := datePattern.FindStringSubmatch(line)
	if len(request) != 4 || len(date) != 2 || request[2] != "200" {
		return
	}
	name := packagePattern.FindStringSubmatch(request[1])
	if len(name) != 3 {
		return
	}
	parsed, err := time.Parse("02/Jan/2006", date[1])
	if err != nil {
		return
	}
	day := parsed.Format("2006-01-02")
	a.days[day]++
	a.releases[name[1]+"\x00"+name[2]]++
	a.total++
}

func WriteJSON(path string, stats Stats) error {
	content, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return writeAtomic(path, content, 0644)
}

func WriteHTML(path string) error {
	return writeAtomic(path, []byte(dashboardHTML), 0644)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".stats-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Chmod(mode)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func versionLess(left, right string) bool {
	l := strings.Split(left, ".")
	r := strings.Split(right, ".")
	for i := 0; i < len(l) || i < len(r); i++ {
		var lv, rv int
		if i < len(l) {
			lv, _ = strconv.Atoi(l[i])
		}
		if i < len(r) {
			rv, _ = strconv.Atoi(r[i])
		}
		if lv != rv {
			return lv < rv
		}
	}
	return left < right
}

const dashboardHTML = `<!doctype html>
<html lang="fr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Téléchargements AegisAdmin</title><style>
:root{color-scheme:dark;font-family:system-ui,sans-serif;background:#081322;color:#e8f3ff}body{margin:0}.shell{max-width:1100px;margin:auto;padding:32px 20px}.muted{color:#91a9c2}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:16px}.card,.panel{background:#111f32;border:1px solid #28415e;border-radius:16px;padding:20px}.value{font-size:2rem;font-weight:750;color:#3bc1ff}table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:12px;border-bottom:1px solid #28415e}th:last-child,td:last-child{text-align:right}.bar{display:inline-block;height:10px;border-radius:6px;background:#36d399;min-width:2px}.error{color:#ff7d87}</style></head>
<body><main class="shell"><h1>Téléchargements AegisAdmin</h1><p class="muted">Statistiques agrégées du dépôt APT — aucune adresse IP n’est enregistrée dans ce tableau.</p>
<section class="cards"><article class="card"><div class="muted">Dépôt APT</div><div class="value" id="aptTotal">—</div></article><article class="card"><div class="muted">GitHub Releases</div><div class="value" id="githubTotal">—</div><small class="muted" id="githubState"></small></article><article class="card"><div class="muted">Dernier jour enregistré</div><div class="value" id="latest">—</div></article><article class="card"><div class="muted">Dernière génération</div><div id="generated">—</div></article></section>
<section class="panel"><h2>Par version</h2><table><thead><tr><th>Version</th><th>Architecture</th><th>APT</th><th>GitHub</th><th>Total indicatif</th></tr></thead><tbody id="releases"></tbody></table></section>
<section class="panel"><h2>Par jour</h2><table><thead><tr><th>Date</th><th>Volume</th><th>Téléchargements</th></tr></thead><tbody id="days"></tbody></table></section><p class="error" id="error"></p></main>
<script>const byId=id=>document.getElementById(id);fetch('stats.json',{cache:'no-store'}).then(r=>{if(!r.ok)throw new Error('HTTP '+r.status);return r.json()}).then(s=>{byId('aptTotal').textContent=s.total_apt_downloads.toLocaleString('fr-FR');byId('githubTotal').textContent=s.github_available?s.total_github_downloads.toLocaleString('fr-FR'):'—';byId('githubState').textContent=s.github_available?'Compteur disponible':s.github_message||'Indisponible';byId('generated').textContent=new Date(s.generated_at).toLocaleString('fr-FR');const entries=[...(s.days||[])].reverse();const releaseEntries=s.releases||[];byId('latest').textContent=(entries[0]?.downloads??0).toLocaleString('fr-FR');const max=Math.max(1,...entries.map(d=>d.downloads));byId('releases').innerHTML=releaseEntries.map(r=>'<tr><td>'+esc(r.version)+'</td><td>'+esc(r.architecture)+'</td><td>'+r.apt_downloads.toLocaleString('fr-FR')+'</td><td>'+(s.github_available?r.github_downloads.toLocaleString('fr-FR'):'—')+'</td><td>'+(s.github_available?(r.apt_downloads+r.github_downloads).toLocaleString('fr-FR'):'—')+'</td></tr>').join('');byId('days').innerHTML=entries.map(d=>'<tr><td>'+esc(d.date)+'</td><td><span class="bar" style="width:'+Math.round(100*d.downloads/max)+'%"></span></td><td>'+d.downloads.toLocaleString('fr-FR')+'</td></tr>').join('')}).catch(e=>byId('error').textContent='Impossible de charger les statistiques : '+e.message);function esc(v){const e=document.createElement('span');e.textContent=v;return e.innerHTML}</script></body></html>`
