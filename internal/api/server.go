package api

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/auth"
	"github.com/R1ddle1337/sb-manager-web/internal/config"
	"github.com/R1ddle1337/sb-manager-web/internal/helper"
	"github.com/R1ddle1337/sb-manager-web/internal/runner"
	"github.com/R1ddle1337/sb-manager-web/internal/storage"
	"github.com/R1ddle1337/sb-manager-web/internal/types"
	"github.com/R1ddle1337/sb-manager-web/web"
)

const (
	sessionCookie = "sbweb_session"
	csrfCookie    = "sbweb_csrf"
	maxBodyBytes  = 128 * 1024
)

type Server struct {
	cfg     config.Config
	store   *storage.Store
	auth    *auth.Manager
	runner  runner.Runner
	mu      sync.Mutex
	sem     chan struct{}
	loginMu sync.Mutex
	logins  map[string]loginAttempt
}

type loginAttempt struct {
	Count        int
	BlockedUntil time.Time
}

func New(cfg config.Config, store *storage.Store) (*Server, string, error) {
	authManager := auth.New(store)
	password, created, err := authManager.EnsureAdmin()
	if err != nil {
		return nil, "", err
	}
	if err := store.PutServer(types.Server{ID: types.ServerLocal, Name: "本机", Online: true, LastSeen: time.Now().UTC()}); err != nil {
		return nil, "", err
	}
	if !created {
		password = ""
	}
	return &Server{cfg: cfg, store: store, auth: authManager, runner: runner.Runner{Path: cfg.SBPath, Timeout: cfg.Tasks.DefaultTimeout}, sem: make(chan struct{}, cfg.Tasks.Concurrency), logins: make(map[string]loginAttempt)}, password, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.page)
	mux.HandleFunc("/login", s.login)
	mux.HandleFunc("/logout", s.logout)
	mux.HandleFunc("/static/", s.static)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, map[string]any{"ok": true}) })
	mux.HandleFunc("/api/v1/", s.api)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, ok := s.session(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	tmpl, err := template.ParseFS(web.Files, "templates/index.html")
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	_ = tmpl.Execute(w, map[string]any{"Version": "0.1.0"})
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/static/")
	if name == "" || name == "." || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := web.Files.ReadFile("static/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	_, _ = w.Write(data)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFS(web.Files, "templates/login.html")
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		_ = tmpl.Execute(w, nil)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	if !s.loginAllowed(ip) {
		http.Error(w, "登录尝试过多，请稍后再试", http.StatusTooManyRequests)
		return
	}
	if username == "" || len(password) > 256 || !s.auth.Authenticate(username, password) {
		s.recordLoginFailure(ip)
		http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
		return
	}
	s.clearLoginFailures(ip)
	session, err := s.auth.NewSession(username)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 43200})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: session.CSRF, Path: "/", HttpOnly: false, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 43200})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginAllowed(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	attempt := s.logins[ip]
	return !attempt.BlockedUntil.After(time.Now())
}

func (s *Server) recordLoginFailure(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	attempt := s.logins[ip]
	attempt.Count++
	if attempt.Count >= 5 {
		minutes := attempt.Count - 4
		if minutes > 15 {
			minutes = 15
		}
		attempt.BlockedUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
	}
	s.logins[ip] = attempt
}

func (s *Server) clearLoginFailures(ip string) {
	s.loginMu.Lock()
	delete(s.logins, ip)
	s.loginMu.Unlock()
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if session, ok := s.session(r); ok && s.checkCSRF(r, session) {
		_ = s.auth.DeleteSession(session.ID)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) session(r *http.Request) (types.Session, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return types.Session{}, false
	}
	return s.auth.ValidateSession(cookie.Value)
}

func (s *Server) checkCSRF(r *http.Request, session types.Session) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	value := r.Header.Get("X-CSRF-Token")
	if value == "" {
		if cookie, err := r.Cookie(csrfCookie); err == nil {
			value = cookie.Value
		}
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(session.CSRF)) == 1
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (types.Session, bool) {
	session, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "需要登录", nil)
		return types.Session{}, false
	}
	if !s.checkCSRF(r, session) {
		writeError(w, http.StatusForbidden, "CSRF_FAILED", "CSRF 校验失败", nil)
		return types.Session{}, false
	}
	return session, true
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/agent/register" {
		s.agentRegister(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/agent/") {
		s.agentAPI(w, r)
		return
	}
	session, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.URL.Path {
	case "/api/v1/session":
		writeJSON(w, http.StatusOK, map[string]any{"username": session.Username, "csrf": session.CSRF})
	case "/api/v1/password":
		s.changePassword(w, r, session)
	case "/api/v1/status":
		s.singleJSONAction(w, r, "status")
	case "/api/v1/nodes":
		s.singleJSONAction(w, r, "nodes.list")
	case "/api/v1/capabilities":
		s.singleJSONAction(w, r, "core.capabilities")
	case "/api/v1/bbr/status":
		s.singleJSONAction(w, r, "bbr.status")
	case "/api/v1/hy2-buffer/status":
		s.singleJSONAction(w, r, "hy2-buffer.status")
	case "/api/v1/realm":
		s.singleJSONAction(w, r, "realm.status")
	case "/api/v1/certificates":
		s.singleJSONAction(w, r, "cert.list")
	case "/api/v1/logs":
		s.plainAction(w, r, "logs")
	case "/api/v1/servers":
		s.servers(w, r)
	case "/api/v1/enrollment":
		s.enrollment(w, r)
	case "/api/v1/tasks":
		s.tasks(w, r)
	case "/api/v1/audit":
		s.audit(w, r)
	case "/api/v1/batch/actions":
		s.batchAction(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/api/v1/servers/") {
			s.serverAPI(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") {
			s.taskAPI(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在", nil)
	}
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, session types.Session) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
		return
	}
	var request struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decodeBody(r, &request); err != nil || !s.auth.Authenticate(session.Username, request.Current) {
		writeError(w, http.StatusUnauthorized, "AUTH_FAILED", "当前密码错误", nil)
		return
	}
	if err := s.auth.SetPassword(session.Username, request.New); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
		return
	}
	_ = s.auth.DeleteSession(session.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login_required": true})
}

func (s *Server) plainAction(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 GET", nil)
		return
	}
	result, err := s.runLocal(action, nil)
	if err != nil {
		writeRunnerError(w, err, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"output": truncate(redact(result.Stdout), 65536)})
}

func (s *Server) batchAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
		return
	}
	var request struct {
		ServerIDs []string       `json:"server_ids"`
		Action    string         `json:"action"`
		Args      map[string]any `json:"args"`
	}
	if err := decodeBody(r, &request); err != nil || len(request.ServerIDs) == 0 || len(request.ServerIDs) > 100 {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "server_ids 必须包含 1-100 台服务器", nil)
		return
	}
	if request.Args == nil {
		request.Args = map[string]any{}
	}
	if _, err := runner.ActionCommand(request.Action, request.Args); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
		return
	}
	batchID, _ := auth.RandomToken(10)
	created := []types.Task{}
	for _, serverID := range request.ServerIDs {
		server, err := s.store.GetServer(serverID)
		if err != nil || (serverID != types.ServerLocal && !server.Online) {
			continue
		}
		id, err := auth.RandomToken(12)
		if err != nil {
			continue
		}
		task := types.Task{ID: "task_" + id, ServerID: serverID, Action: request.Action, Args: request.Args, Status: types.TaskPending, CreatedAt: time.Now().UTC(), IdempotencyKey: "batch_" + batchID + "_" + serverID, BatchID: "batch_" + batchID}
		if s.store.PutTask(task) != nil {
			continue
		}
		created = append(created, task)
		if serverID == types.ServerLocal {
			go s.executeLocal(task)
		}
	}
	if len(created) == 0 {
		writeError(w, http.StatusConflict, "NO_ELIGIBLE_SERVERS", "没有可执行的在线服务器", nil)
		return
	}
	taskIDs := make([]string, 0, len(created))
	for _, task := range created {
		taskIDs = append(taskIDs, task.ID)
	}
	s.recordAudit("admin", request.Action, request.ServerIDs, taskIDs, "accepted")
	writeJSON(w, http.StatusAccepted, map[string]any{"batch_id": "batch_" + batchID, "tasks": created})
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 GET", nil)
		return
	}
	events, err := s.store.ListAudit(200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) recordAudit(actor, action string, serverIDs, taskIDs []string, result string) {
	id, err := auth.RandomToken(10)
	if err != nil {
		return
	}
	_ = s.store.PutAudit(types.AuditEvent{ID: "evt_" + id, Actor: actor, Action: action, ServerIDs: serverIDs, TaskIDs: taskIDs, Result: result, CreatedAt: time.Now().UTC()})
}

func (s *Server) singleJSONAction(w http.ResponseWriter, r *http.Request, action string) {
	s.jsonAction(w, r, action, nil)
}

func (s *Server) jsonAction(w http.ResponseWriter, r *http.Request, action string, args map[string]any) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 GET", nil)
		return
	}
	result, err := s.runLocal(action, args)
	if err != nil {
		writeRunnerError(w, err, result)
		return
	}
	if !json.Valid([]byte(result.Stdout)) {
		writeError(w, http.StatusBadGateway, "INVALID_SB_JSON", "sb 返回了无效 JSON", map[string]any{"stderr": result.Stderr})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = io.WriteString(w, result.Stdout)
}

func (s *Server) servers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 GET", nil)
		return
	}
	servers, err := s.store.ListServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	now := time.Now().UTC()
	for index := range servers {
		if servers[index].ID != types.ServerLocal && now.Sub(servers[index].LastSeen) > 2*time.Minute {
			servers[index].Online = false
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (s *Server) enrollment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
		return
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RANDOM_FAILED", err.Error(), nil)
		return
	}
	expires := time.Now().UTC().Add(10 * time.Minute)
	if err := s.store.PutEnrollment(auth.HashEnrollmentToken(token), types.EnrollmentToken{Hash: auth.HashEnrollmentToken(token), ExpiresAt: expires}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	baseURL := "http://" + r.Host
	if r.TLS != nil {
		baseURL = "https://" + r.Host
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"expires_at":   expires,
		"token":        token,
		"join_command": fmt.Sprintf("sb-web join %s %s", baseURL, token),
	})
}

type actionRequest struct {
	Action         string         `json:"action"`
	Args           map[string]any `json:"args"`
	IdempotencyKey string         `json:"idempotency_key"`
}

func (s *Server) serverAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "servers" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器接口不存在", nil)
		return
	}
	serverID := parts[3]
	if len(parts) == 4 && r.Method == http.MethodGet {
		server, err := s.store.GetServer(serverID)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器不存在", nil)
			return
		}
		server.AgentPublicKey = ""
		writeJSON(w, http.StatusOK, server)
		return
	}
	if len(parts) == 5 && parts[4] == "status" && r.Method == http.MethodGet {
		if serverID == types.ServerLocal {
			s.singleJSONAction(w, r, "status")
			return
		}
		server, err := s.store.GetServer(serverID)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器不存在", nil)
			return
		}
		writeJSON(w, http.StatusOK, server.Status)
		return
	}
	if len(parts) == 5 && parts[4] == "actions" && r.Method == http.MethodPost {
		s.createAction(w, r, serverID)
		return
	}
	if len(parts) == 5 && parts[4] == "nodes" && r.Method == http.MethodPost {
		var fields map[string]any
		if err := decodeBody(r, &fields); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
			return
		}
		fields["protocol"] = firstNonEmptyString(fields["protocol"])
		request := actionRequest{Action: "node.add", Args: fields, IdempotencyKey: r.Header.Get("Idempotency-Key")}
		s.enqueueAction(w, serverID, request)
		return
	}
	if len(parts) == 5 && parts[4] == "nodes" && r.Method == http.MethodGet {
		if serverID == types.ServerLocal {
			s.singleJSONAction(w, r, "nodes.list")
			return
		}
		server, err := s.store.GetServer(serverID)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器不存在", nil)
			return
		}
		writeJSON(w, http.StatusOK, server.Status)
		return
	}
	if len(parts) == 6 && parts[4] == "nodes" && r.Method == http.MethodPatch {
		var fields map[string]any
		if err := decodeBody(r, &fields); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
			return
		}
		fields["id"] = parts[5]
		request := actionRequest{Action: "node.set", Args: fields, IdempotencyKey: r.Header.Get("Idempotency-Key")}
		s.enqueueAction(w, serverID, request)
		return
	}
	if len(parts) == 6 && parts[4] == "nodes" && r.Method == http.MethodGet {
		if serverID != types.ServerLocal {
			writeError(w, http.StatusConflict, "AGENT_ASYNC_ONLY", "远程节点详情请通过任务获取", nil)
			return
		}
		s.jsonAction(w, r, "node.show", map[string]any{"id": parts[5]})
		return
	}
	if len(parts) == 7 && parts[4] == "nodes" && parts[6] == "share" && r.Method == http.MethodGet {
		if serverID != types.ServerLocal {
			writeError(w, http.StatusConflict, "AGENT_ASYNC_ONLY", "远程分享链接请通过任务获取", nil)
			return
		}
		var args map[string]any
		if user := r.URL.Query().Get("user"); user != "" {
			args = map[string]any{"id": parts[5], "user_id": user}
		} else {
			args = map[string]any{"id": parts[5]}
		}
		result, err := s.runLocal("node.share", args)
		if err != nil {
			writeRunnerError(w, err, result)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"share": strings.TrimSpace(result.Stdout)})
		return
	}
	if len(parts) == 7 && parts[4] == "nodes" && r.Method == http.MethodPost {
		if parts[6] != "enable" && parts[6] != "disable" && parts[6] != "delete" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "节点操作不存在", nil)
			return
		}
		request := actionRequest{Action: "node." + parts[6], Args: map[string]any{"id": parts[5]}, IdempotencyKey: r.Header.Get("Idempotency-Key")}
		s.enqueueAction(w, serverID, request)
		return
	}
	if len(parts) == 7 && parts[4] == "nodes" && parts[6] == "users" && r.Method == http.MethodGet {
		if serverID != types.ServerLocal {
			writeError(w, http.StatusConflict, "AGENT_ASYNC_ONLY", "远程用户列表请通过任务获取", nil)
			return
		}
		s.jsonAction(w, r, "users.list", map[string]any{"node_id": parts[5]})
		return
	}
	if len(parts) == 7 && parts[4] == "nodes" && parts[6] == "users" && r.Method == http.MethodPost {
		var fields map[string]any
		if err := decodeBody(r, &fields); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
			return
		}
		fields["node_id"], fields["user_id"] = parts[5], firstNonEmptyString(fields["user_id"])
		s.enqueueAction(w, serverID, actionRequest{Action: "user.add", Args: fields, IdempotencyKey: r.Header.Get("Idempotency-Key")})
		return
	}
	if len(parts) == 9 && parts[4] == "nodes" && parts[6] == "users" && r.Method == http.MethodPost {
		verb := parts[8]
		if verb != "enable" && verb != "disable" && verb != "delete" && verb != "rotate" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "用户操作不存在", nil)
			return
		}
		s.enqueueAction(w, serverID, actionRequest{Action: "user." + verb, Args: map[string]any{"node_id": parts[5], "user_id": parts[7]}, IdempotencyKey: r.Header.Get("Idempotency-Key")})
		return
	}
	if len(parts) == 5 && parts[4] == "certificates" && r.Method == http.MethodPost {
		var fields map[string]any
		if err := decodeBody(r, &fields); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
			return
		}
		request := actionRequest{Action: "cert.issue", Args: fields, IdempotencyKey: r.Header.Get("Idempotency-Key")}
		s.enqueueAction(w, serverID, request)
		return
	}
	if len(parts) == 5 && parts[4] == "backup" && r.Method == http.MethodPost {
		s.enqueueAction(w, serverID, actionRequest{Action: "backup.create", Args: map[string]any{}, IdempotencyKey: r.Header.Get("Idempotency-Key")})
		return
	}
	if len(parts) == 5 && parts[4] == "capabilities" && r.Method == http.MethodGet {
		if serverID == types.ServerLocal {
			s.singleJSONAction(w, r, "core.capabilities")
			return
		}
		server, err := s.store.GetServer(serverID)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器不存在", nil)
			return
		}
		writeJSON(w, http.StatusOK, server.Capabilities)
		return
	}
	writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器接口不存在", nil)
}

func (s *Server) createAction(w http.ResponseWriter, r *http.Request, serverID string) {
	var request actionRequest
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
		return
	}
	if request.Args == nil {
		request.Args = map[string]any{}
	}
	s.enqueueAction(w, serverID, request)
}

func (s *Server) enqueueAction(w http.ResponseWriter, serverID string, request actionRequest) {
	if _, err := runner.ActionCommand(request.Action, request.Args); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
		return
	}
	server, err := s.store.GetServer(serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "服务器不存在", nil)
		return
	}
	if serverID != types.ServerLocal && !server.Online {
		writeError(w, http.StatusConflict, "AGENT_OFFLINE", "服务器当前离线", nil)
		return
	}
	if existing, err := s.store.FindTaskByIdempotency(request.IdempotencyKey); err == nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	id, err := auth.RandomToken(12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RANDOM_FAILED", err.Error(), nil)
		return
	}
	task := types.Task{ID: "task_" + id, ServerID: serverID, Action: request.Action, Args: request.Args, Status: types.TaskPending, CreatedAt: time.Now().UTC(), IdempotencyKey: request.IdempotencyKey}
	if err := s.store.PutTask(task); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	if serverID == types.ServerLocal {
		go s.executeLocal(task)
	}
	s.recordAudit("admin", request.Action, []string{serverID}, []string{task.ID}, "accepted")
	writeJSON(w, http.StatusAccepted, task)
}

func firstNonEmptyString(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func (s *Server) executeLocal(task types.Task) {
	s.sem <- struct{}{}
	defer func() { <-s.sem }()
	if stop, _ := s.store.BatchShouldStop(task.BatchID, s.cfg.Tasks.FailureStopPct); stop {
		s.finishTask(task.ID, false, "", "batch stopped after reaching failure threshold")
		return
	}
	result, err := s.runLocal(task.Action, task.Args)
	if err != nil {
		s.finishTask(task.ID, false, result.Stdout, redact(result.Stderr+"\n"+err.Error()))
		return
	}
	s.finishTask(task.ID, true, redact(result.Stdout), redact(result.Stderr))
}

func (s *Server) finishTask(id string, success bool, output, problem string) {
	_, _ = s.store.UpdateTask(id, func(task *types.Task) error {
		now := time.Now().UTC()
		task.FinishedAt = &now
		task.Output = truncate(redact(output), 32768)
		task.Error = truncate(redact(problem), 8192)
		if success {
			task.Status = types.TaskSuccess
		} else {
			task.Status = types.TaskFailed
		}
		return nil
	})
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 GET", nil)
		return
	}
	tasks, err := s.store.ListTasks(100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) taskAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 GET", nil)
		return
	}
	id := path.Base(r.URL.Path)
	task, err := s.store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "任务不存在", nil)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

type registerRequest struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Region    string `json:"region"`
	Arch      string `json:"arch"`
}

func (s *Server) agentRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
		return
	}
	var request registerRequest
	if err := decodeBody(r, &request); err != nil || request.Token == "" || request.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "token 和 public_key 必填", nil)
		return
	}
	hash := auth.HashEnrollmentToken(request.Token)
	enrollment, err := s.store.GetEnrollment(hash)
	if err != nil || enrollment.Used || enrollment.ExpiresAt.Before(time.Now().UTC()) {
		writeError(w, http.StatusUnauthorized, "ENROLLMENT_INVALID", "加入令牌无效或已过期", nil)
		return
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(request.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "public_key 不是有效 Ed25519 公钥", nil)
		return
	}
	id, err := auth.RandomToken(10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RANDOM_FAILED", err.Error(), nil)
		return
	}
	server := types.Server{ID: "srv_" + id, Name: firstNonEmpty(request.Name, "远程服务器"), Address: request.Address, Region: request.Region, Arch: request.Arch, AgentPublicKey: request.PublicKey, Online: true, LastSeen: time.Now().UTC()}
	if err := s.store.ConsumeEnrollment(hash, server); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	s.recordAudit("enrollment", "agent.register", []string{server.ID}, nil, "success")
	writeJSON(w, http.StatusCreated, map[string]any{"server_id": server.ID, "heartbeat_interval": "30s"})
}

func (s *Server) agentAPI(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil)
		return
	}
	server, ok := s.verifyAgent(r, body)
	if !ok {
		writeError(w, http.StatusUnauthorized, "AGENT_AUTH_FAILED", "Agent 身份验证失败", nil)
		return
	}
	switch r.URL.Path {
	case "/api/v1/agent/heartbeat":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
			return
		}
		var heartbeat struct {
			AgentVersion     string `json:"agent_version"`
			SBManagerVersion string `json:"sb_manager_version"`
			CoreVersion      string `json:"core_version"`
			Backend          string `json:"backend"`
			Status           any    `json:"status"`
			Capabilities     any    `json:"capabilities"`
		}
		if json.Unmarshal(body, &heartbeat) != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "心跳 JSON 无效", nil)
			return
		}
		server.AgentVersion, server.SBManagerVersion, server.CoreVersion, server.Backend = heartbeat.AgentVersion, heartbeat.SBManagerVersion, heartbeat.CoreVersion, heartbeat.Backend
		server.Status, server.Capabilities, server.Online, server.LastSeen = heartbeat.Status, heartbeat.Capabilities, true, time.Now().UTC()
		if err := s.store.PutServer(server); err != nil {
			writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "/api/v1/agent/poll":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
			return
		}
		task, err := s.store.ClaimPendingTask(server.ID, s.cfg.Tasks.FailureStopPct)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"task": nil})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": task})
	case "/api/v1/agent/result":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "只支持 POST", nil)
			return
		}
		var result struct {
			TaskID string `json:"task_id"`
			Status string `json:"status"`
			Output string `json:"output"`
			Error  string `json:"error"`
		}
		if json.Unmarshal(body, &result) != nil || result.TaskID == "" || (result.Status != types.TaskSuccess && result.Status != types.TaskFailed) {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "任务结果无效", nil)
			return
		}
		task, err := s.store.GetTask(result.TaskID)
		if err != nil || task.ServerID != server.ID {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "任务不存在或目标不匹配", nil)
			return
		}
		_, err = s.store.UpdateTask(result.TaskID, func(task *types.Task) error {
			now := time.Now().UTC()
			task.Status, task.FinishedAt = result.Status, &now
			task.Output, task.Error = truncate(redact(result.Output), 32768), truncate(redact(result.Error), 8192)
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Agent 接口不存在", nil)
	}
}

func (s *Server) verifyAgent(r *http.Request, body []byte) (types.Server, bool) {
	id := r.Header.Get("X-Agent-ID")
	timestamp, err := strconv.ParseInt(r.Header.Get("X-Agent-Timestamp"), 10, 64)
	if id == "" || err != nil || time.Since(time.Unix(timestamp, 0)) > 2*time.Minute || time.Until(time.Unix(timestamp, 0)) > 2*time.Minute {
		return types.Server{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Agent-Signature"))
	if err != nil {
		return types.Server{}, false
	}
	server, err := s.store.GetServer(id)
	if err != nil {
		return types.Server{}, false
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(server.AgentPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return types.Server{}, false
	}
	message := r.Method + "\n" + r.URL.Path + "\n" + r.Header.Get("X-Agent-Timestamp") + "\n" + string(body)
	return server, ed25519.Verify(ed25519.PublicKey(publicKey), []byte(message), signature)
}

func (s *Server) runLocal(action string, args map[string]any) (runner.Result, error) {
	if s.cfg.HelperSocket != "" {
		if _, err := os.Stat(s.cfg.HelperSocket); err == nil {
			return helper.Client{Socket: s.cfg.HelperSocket, Timeout: s.cfg.Tasks.DefaultTimeout}.Run(context.Background(), action, args)
		}
	}
	command, err := runner.ActionCommand(action, args)
	if err != nil {
		return runner.Result{}, err
	}
	return s.runner.Run(context.Background(), command...)
}

func writeRunnerError(w http.ResponseWriter, err error, result runner.Result) {
	status := http.StatusBadGateway
	code := "COMMAND_FAILED"
	if result.TimedOut {
		status, code = http.StatusGatewayTimeout, "COMMAND_TIMEOUT"
	}
	writeError(w, status, code, err.Error(), map[string]any{"exit_code": result.ExitCode, "stderr": redact(result.Stderr)})
}

func decodeBody(r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(nilResponseWriter{}, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

// nilResponseWriter is only used so MaxBytesReader can enforce a request limit.
type nilResponseWriter struct{}

func (nilResponseWriter) Header() http.Header       { return make(http.Header) }
func (nilResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (nilResponseWriter) WriteHeader(int)           {}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n[truncated]"
}

func redact(value string) string {
	return secretJSONPattern.ReplaceAllString(value, `"$1":"[REDACTED]"`)
}

var secretJSONPattern = regexp.MustCompile(`(?i)"(password|token|psk|private_key|uuid)"\s*:\s*"[^"]*"`)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
