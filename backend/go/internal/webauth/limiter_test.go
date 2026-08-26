package webauth

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	limiter := NewLoginLimiter()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	for attempts := 0; attempts < limiter.account; attempts++ {
		if !limiter.Allowed("192.0.2.1", "Root") {
			t.Fatal("login blocked too early")
		}
		limiter.Failure("192.0.2.1", "Root")
	}
	if limiter.Allowed("192.0.2.2", "root") {
		t.Fatal("account limit was bypassed with another address or case")
	}
	limiter.Success("ROOT")
	if !limiter.Allowed("192.0.2.2", "root") {
		t.Fatal("successful login did not clear account failures")
	}
	now = now.Add(limiter.window + time.Second)
	if !limiter.Allowed("192.0.2.1", "root") {
		t.Fatal("expired failures were not cleared")
	}
}
