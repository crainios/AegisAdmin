package protocol

import "aegisadmin/backend/internal/api"

const MaximumMessageBytes int64 = 64 * 1024 * 1024

type Request struct {
	Domain    string   `json:"domain"`
	Command   string   `json:"command"`
	Arguments []string `json:"arguments"`
}

type Reply struct {
	ExitCode int          `json:"exit_code"`
	Response api.Response `json:"response"`
}
