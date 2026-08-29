package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/config"
	"github.com/R1ddle1337/sb-manager-web/internal/storage"
)

func fakeSB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sb")
	content := `#!/bin/sh
case "$*" in
  *"realm status"*) printf '%s\n' '{"enabled":false,"listen":"::","port":9443}' ;;
  *"status"*) printf '%s\n' '{"services":{"sing_box":{"active":true}},"nodes":[]}' ;;
  *"node list"*) printf '%s\n' '{"nodes":[]}' ;;
  *"capabilities"*) printf '%s\n' '{"version":"1.14.0-rc.2","tags":[]}' ;;
  *"bbr status"*) printf '%s\n' '{"enabled":false}' ;;
  *"hy2 buffer status"*) printf '%s\n' '{"enabled":false}' ;;
  version) printf '%s\n' 'sb-manager 0.1.0-alpha.27' ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoginStatusAndAction(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.DataDir, cfg.Database, cfg.SBPath = dir, filepath.Join(dir, "web.db"), fakeSB(t)
	cfg.Tasks.DefaultTimeout = time.Second
	store, err := storage.Open(cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h, initial, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Handler())
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	form := url.Values{"username": {"admin"}, "password": {initial}}
	login, err := client.PostForm(server.URL+"/login", form)
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status: %d", login.StatusCode)
	}
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	client.Jar.SetCookies(mustURL(server.URL), login.Cookies())
	csrf := ""
	for _, cookie := range login.Cookies() {
		if cookie.Name == csrfCookie {
			csrf = cookie.Value
		}
	}
	if csrf == "" {
		t.Fatal("login did not issue csrf cookie")
	}
	response, err := client.Get(server.URL + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(data), `"sing_box"`) {
		t.Fatalf("status response: %d %s", response.StatusCode, data)
	}
	realmResponse, err := client.Get(server.URL + "/api/v1/realm")
	if err != nil {
		t.Fatal(err)
	}
	var realm map[string]any
	if err := json.NewDecoder(realmResponse.Body).Decode(&realm); err != nil {
		t.Fatal(err)
	}
	realmResponse.Body.Close()
	if realmResponse.StatusCode != http.StatusOK || realm["enabled"] != false {
		t.Fatalf("realm response: %d %#v", realmResponse.StatusCode, realm)
	}
	for _, page := range []string{"/", "/servers/local", "/servers/local/nodes/demo", "/settings/users"} {
		pageResponse, pageErr := client.Get(server.URL + page)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		pageBody, _ := io.ReadAll(pageResponse.Body)
		pageResponse.Body.Close()
		if pageResponse.StatusCode != http.StatusOK || len(pageBody) == 0 {
			t.Fatalf("page %s: status=%d body=%d", page, pageResponse.StatusCode, len(pageBody))
		}
	}
	body := strings.NewReader(`{"action":"bbr.enable","idempotency_key":"test-action-1"}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/servers/local/actions", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	actionResponse, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var task map[string]any
	if err := json.NewDecoder(actionResponse.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	actionResponse.Body.Close()
	if actionResponse.StatusCode != http.StatusAccepted || task["id"] == nil {
		t.Fatalf("action response: %d %#v", actionResponse.StatusCode, task)
	}
	userBody := strings.NewReader(`{"username":"operator","password":"correct horse battery staple","role":"operator"}`)
	userRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/users", userBody)
	userRequest.Header.Set("Content-Type", "application/json")
	userRequest.Header.Set("X-CSRF-Token", csrf)
	userResponse, err := client.Do(userRequest)
	if err != nil {
		t.Fatal(err)
	}
	userResponse.Body.Close()
	if userResponse.StatusCode != http.StatusCreated {
		t.Fatalf("user create status: %d", userResponse.StatusCode)
	}
	roleBody := strings.NewReader(`{"role":"viewer"}`)
	roleRequest, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/users/operator", roleBody)
	roleRequest.Header.Set("Content-Type", "application/json")
	roleRequest.Header.Set("X-CSRF-Token", csrf)
	roleResponse, err := client.Do(roleRequest)
	if err != nil {
		t.Fatal(err)
	}
	roleResponse.Body.Close()
	if roleResponse.StatusCode != http.StatusOK {
		t.Fatalf("role update status: %d", roleResponse.StatusCode)
	}
	deleteRequest, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/users/operator", nil)
	deleteRequest.Header.Set("X-CSRF-Token", csrf)
	deleteResponse, err := client.Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	deleteResponse.Body.Close()
	if deleteResponse.StatusCode != http.StatusOK {
		t.Fatalf("user delete status: %d", deleteResponse.StatusCode)
	}
}

func mustURL(raw string) *url.URL { value, _ := url.Parse(raw); return value }
