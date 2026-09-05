package server

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aegisadmin/backend/internal/protocol"
)

func TestPrepareSocketRefusesRunningServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("Unix sockets are not permitted in this test sandbox")
		}
		t.Fatal(err)
	}
	defer listener.Close()
	if err = prepareSocket(path); err == nil || !strings.Contains(err.Error(), "running server") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareSocketRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("Unix sockets are not permitted in this test sandbox")
		}
		t.Fatal(err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err = prepareSocket(path); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale socket still exists: %v", err)
	}
}

func TestDecodeRequestUsesNewlineFrameWithoutWaitingForEOF(t *testing.T) {
	reader, writer := newBlockingReader()
	done := make(chan protocol.Request, 1)
	errors := make(chan error, 1)

	go func() {
		request, err := decodeRequest(reader)
		if err != nil {
			errors <- err
			return
		}
		done <- request
	}()

	writer <- []byte("{\"domain\":\"system\",\"command\":\"info\",\"arguments\":[]}\n")

	select {
	case err := <-errors:
		t.Fatal(err)
	case request := <-done:
		if request.Domain != "system" || request.Command != "info" {
			t.Fatalf("unexpected request: %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("decoder waited for EOF instead of completing the newline frame")
	}
}

func TestDecodeRequestRejectsTrailingJSON(t *testing.T) {
	_, err := decodeRequest(strings.NewReader("{} {}\n"))
	if err == nil {
		t.Fatal("trailing JSON must be rejected")
	}
}

func TestDecodeRequestRejectsUnknownFields(t *testing.T) {
	_, err := decodeRequest(strings.NewReader("{\"domain\":\"system\",\"unknown\":true}\n"))
	if err == nil {
		t.Fatal("unknown fields must be rejected")
	}
}

type blockingReader struct {
	chunks <-chan []byte
	buffer []byte
}

func newBlockingReader() (*blockingReader, chan<- []byte) {
	chunks := make(chan []byte)
	return &blockingReader{chunks: chunks}, chunks
}

func (r *blockingReader) Read(destination []byte) (int, error) {
	if len(r.buffer) == 0 {
		r.buffer = <-r.chunks
	}
	read := copy(destination, r.buffer)
	r.buffer = r.buffer[read:]
	return read, nil
}
