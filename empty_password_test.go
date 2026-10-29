package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmptyPasswordSSHConnectionAndTerminal(t *testing.T) {
	t.Cleanup(closeAllTermSessions)
	host, port, fingerprint := startFakeSSHServerWithPassword(t, "")
	for _, endpoint := range []string{"/api/commands/test", "/api/term/open"} {
		t.Run(endpoint, func(t *testing.T) {
			for _, auth := range []string{"password", "key"} {
				body, _ := json.Marshal(commandConnectionRequest{Host: host, Port: port, User: "tester", Auth: auth, Fingerprint: fingerprint})
				response := httptest.NewRecorder()
				newHTTPHandler(&server{}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body)))
				want := http.StatusOK
				if auth == "key" {
					want = http.StatusBadRequest
				}
				if response.Code != want {
					t.Fatalf("auth=%s: got %d, want %d: %s", auth, response.Code, want, response.Body.String())
				}
			}
		})
	}
	// An empty password still requires explicit trust of the host key.
	body, _ := json.Marshal(commandConnectionRequest{Host: host, Port: port, User: "tester", Auth: "password"})
	response := httptest.NewRecorder()
	(&server{}).handleCommandConnectionTest(response, httptest.NewRequest(http.MethodPost, "/api/commands/test", bytes.NewReader(body)))
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("untrusted host: got %d: %s", response.Code, response.Body.String())
	}
}

func TestEmptyPasswordCommandExecution(t *testing.T) {
	originalRun := commandExecutionRun
	t.Cleanup(func() { commandExecutionRun = originalRun })
	called := false
	commandExecutionRun = func(request commandExecutionRequest) (string, error) {
		called = true
		if request.Auth != "password" || request.Secret != "" {
			t.Fatal("empty password was not preserved")
		}
		return "ok", nil
	}
	for _, auth := range []string{"password", "key"} {
		called = false
		body, _ := json.Marshal(commandExecutionRequest{Host: "127.0.0.1", User: "tester", Auth: auth, Fingerprint: "SHA256:test", Command: "pwd"})
		response := httptest.NewRecorder()
		(&server{}).handleCommandExecute(response, httptest.NewRequest(http.MethodPost, "/api/commands/execute", bytes.NewReader(body)))
		want := http.StatusOK
		if auth == "key" {
			want = http.StatusBadRequest
		}
		if response.Code != want || called != (auth == "password") {
			t.Fatalf("auth=%s: got %d, called=%v: %s", auth, response.Code, called, response.Body.String())
		}
	}
}

func TestEmptyPasswordCommandUploadValidation(t *testing.T) {
	for _, auth := range []string{"password", "key"} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for key, value := range map[string]string{"host": "127.0.0.1", "port": "1", "user": "tester", "auth": auth, "secret": "", "fingerprint": "SHA256:test"} {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		part, err := writer.CreateFormFile("file", "test.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("test"))
		_ = writer.Close()
		request := httptest.NewRequest(http.MethodPost, "/api/commands/upload", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		(&server{}).handleCommandFileUpload(response, request)
		// Password authentication reaches SSH; the closed local port then fails.
		want := http.StatusBadGateway
		if auth == "key" {
			want = http.StatusBadRequest
		}
		if response.Code != want {
			t.Fatalf("auth=%s: got %d, want %d: %s", auth, response.Code, want, response.Body.String())
		}
	}
}
