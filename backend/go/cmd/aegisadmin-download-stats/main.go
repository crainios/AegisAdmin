package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"aegisadmin/backend/internal/downloadstats"
)

func main() {
	logs := flag.String("logs", "/var/log/apache2/aegisadmin-packages-access.log*", "Apache access log glob")
	output := flag.String("output", "/var/lib/aegisadmin-download-stats/public", "dashboard output directory")
	githubRepository := flag.String("github", "crainios/AegisAdmin", "public GitHub owner/repository, or empty to disable")
	flag.Parse()
	stats, err := downloadstats.Collect(*logs, time.Now())
	if err == nil && *githubRepository != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		githubErr := downloadstats.AddGitHub(ctx, http.DefaultClient, "https://api.github.com/repos/"+*githubRepository+"/releases?per_page=100", os.Getenv("GITHUB_TOKEN"), &stats)
		cancel()
		if githubErr != nil {
			stats.GitHubMessage = githubErr.Error()
		}
	}
	if err == nil {
		err = downloadstats.WriteJSON(filepath.Join(*output, "stats.json"), stats)
	}
	if err == nil {
		err = downloadstats.WriteHTML(filepath.Join(*output, "index.html"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "aegisadmin-download-stats:", err)
		os.Exit(1)
	}
}
