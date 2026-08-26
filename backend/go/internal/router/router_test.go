package router

import (
	"context"
	"testing"

	"aegisadmin/backend/internal/protocol"
)

func TestKnownDomainIsNotAvailableUntilRegistered(t *testing.T) {
	reply := New().Route(context.Background(), protocol.Request{
		Domain:  "system",
		Command: "info",
	})

	if reply.ExitCode != ExitFeatureNotAvailable {
		t.Fatalf("unexpected exit code: %d", reply.ExitCode)
	}
	if reply.Response.Success {
		t.Fatal("an unavailable domain cannot succeed")
	}
	if reply.Response.Error == nil || reply.Response.Error.Code != "FEATURE_NOT_AVAILABLE" {
		t.Fatalf("unexpected error: %#v", reply.Response.Error)
	}
}

func TestHyphenatedKnownDomainIsAccepted(t *testing.T) {
	reply := New().Route(context.Background(), protocol.Request{
		Domain:  "admin-access",
		Command: "status",
	})

	if reply.ExitCode != ExitFeatureNotAvailable {
		t.Fatalf("unexpected exit code: %d", reply.ExitCode)
	}
	if reply.Response.Error == nil || reply.Response.Error.Code != "FEATURE_NOT_AVAILABLE" {
		t.Fatalf("unexpected error: %#v", reply.Response.Error)
	}
}

func TestInvalidDomain(t *testing.T) {
	reply := New().Route(context.Background(), protocol.Request{
		Domain:  "../system",
		Command: "info",
	})

	if reply.ExitCode != ExitInvalidDomain {
		t.Fatalf("unexpected exit code: %d", reply.ExitCode)
	}
	if reply.Response.Error == nil || reply.Response.Error.Code != "INVALID_DOMAIN_NAME" {
		t.Fatalf("unexpected error: %#v", reply.Response.Error)
	}
}

func TestUnknownDomain(t *testing.T) {
	reply := New().Route(context.Background(), protocol.Request{
		Domain:  "unknown",
		Command: "info",
	})

	if reply.ExitCode != ExitDomainNotFound {
		t.Fatalf("unexpected exit code: %d", reply.ExitCode)
	}
	if reply.Response.Error == nil || reply.Response.Error.Code != "DOMAIN_NOT_FOUND" {
		t.Fatalf("unexpected error: %#v", reply.Response.Error)
	}
}
