package system

import (
	"context"
	"errors"
	"testing"
)

type fakeCollector struct {
	infoData       map[string]any
	infoError      error
	processData    map[string]any
	processesError error
}

func (f *fakeCollector) Info(context.Context) (map[string]any, error) {
	return f.infoData, f.infoError
}

func (f *fakeCollector) Processes(context.Context) (map[string]any, error) {
	return f.processData, f.processesError
}

func TestInfoSuccess(t *testing.T) {
	handler := New(&fakeCollector{infoData: map[string]any{"backend": backendVersion}})
	reply := handler.Handle(context.Background(), "info", nil)

	if reply.ExitCode != 0 || !reply.Response.Success || reply.Response.Data == nil {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	if (*reply.Response.Data)["backend"] != backendVersion {
		t.Fatalf("unexpected data: %#v", *reply.Response.Data)
	}
}

func TestInfoReadFailure(t *testing.T) {
	handler := New(&fakeCollector{infoError: errors.New("read failed")})
	reply := handler.Handle(context.Background(), "info", nil)

	assertError(t, reply.ExitCode, reply.Response.Error.Code, 3, "SYSTEM_INFO_READ_FAILED")
}

func TestProcessesReadFailure(t *testing.T) {
	handler := New(&fakeCollector{processesError: errors.New("read failed")})
	reply := handler.Handle(context.Background(), "processes", nil)

	assertError(t, reply.ExitCode, reply.Response.Error.Code, 3, "SYSTEM_PROCESSES_READ_FAILED")
}

func TestInvalidArgumentCount(t *testing.T) {
	handler := New(&fakeCollector{})
	reply := handler.Handle(context.Background(), "info", []string{"unexpected"})

	assertError(t, reply.ExitCode, reply.Response.Error.Code, 2, "INVALID_ARGUMENT_COUNT")
}

func TestMissingCommand(t *testing.T) {
	handler := New(&fakeCollector{})
	reply := handler.Handle(context.Background(), "", nil)

	assertError(t, reply.ExitCode, reply.Response.Error.Code, 2, "MISSING_COMMAND")
}

func TestUnknownCommand(t *testing.T) {
	handler := New(&fakeCollector{})
	reply := handler.Handle(context.Background(), "unknown", nil)

	assertError(t, reply.ExitCode, reply.Response.Error.Code, 4, "COMMAND_NOT_FOUND")
}

func assertError(t *testing.T, exitCode int, errorCode string, expectedExit int, expectedError string) {
	t.Helper()
	if exitCode != expectedExit || errorCode != expectedError {
		t.Fatalf("unexpected error: exit=%d code=%s", exitCode, errorCode)
	}
}
