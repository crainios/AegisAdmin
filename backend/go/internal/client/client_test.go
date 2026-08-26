package client

import (
	"encoding/json"
	"net"
	"syscall"
	"testing"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestExecuteWaitsForDaemonSocket(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	attempts := 0
	client := New("/run/aegisadmin-system/backend.sock")
	client.dial = func(_, _ string, _ time.Duration) (net.Conn, error) {
		attempts++
		if attempts < 3 {
			return nil, syscall.ENOENT
		}
		return clientConnection, nil
	}
	client.sleep = func(time.Duration) {}

	done := make(chan error, 1)
	go func() {
		defer serverConnection.Close()
		var request protocol.Request
		if err := json.NewDecoder(serverConnection).Decode(&request); err != nil {
			done <- err
			return
		}
		reply := protocol.Reply{Response: api.Success(map[string]any{"domain": request.Domain})}
		done <- json.NewEncoder(serverConnection).Encode(reply)
	}()

	reply, err := client.Execute(protocol.Request{Domain: "storage", Command: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Response.Success || reply.Response.Data == nil || (*reply.Response.Data)["domain"] != "storage" {
		t.Fatalf("reply = %#v", reply)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d", attempts)
	}
}
