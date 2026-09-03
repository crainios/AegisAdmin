package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSplitListenAddresses(t *testing.T) {
	items := splitListenAddresses("127.0.0.1:8443, [::1]:8443")
	if len(items) != 2 || items[0] != "127.0.0.1:8443" || items[1] != "[::1]:8443" {
		t.Fatalf("addresses=%#v", items)
	}
}

func TestRestrictClients(t *testing.T) {
	handler, err := restrictClients(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}), "192.0.2.0/24")
	if err != nil {
		t.Fatal(err)
	}
	for address, wanted := range map[string]int{"192.0.2.10:1234": http.StatusNoContent, "198.51.100.10:1234": http.StatusForbidden} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = address
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != wanted {
			t.Fatalf("address=%s status=%d", address, response.Code)
		}
	}
}
