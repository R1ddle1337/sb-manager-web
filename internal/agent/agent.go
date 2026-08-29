package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/config"
	"github.com/R1ddle1337/sb-manager-web/internal/runner"
	"github.com/R1ddle1337/sb-manager-web/internal/types"
)

type identity struct {
	PrivateKey string `json:"private_key"`
	ServerID   string `json:"server_id"`
}

type Agent struct {
	cfg    config.Config
	key    ed25519.PrivateKey
	server string
	client *http.Client
	runner runner.Runner
}

var Version = "dev"

func New(cfg config.Config) (*Agent, error) {
	identityPath := cfg.Agent.IdentityFile
	if identityPath == "" {
		identityPath = filepath.Join(cfg.DataDir, "agent-identity", "ed25519.json")
	}
	data, err := os.ReadFile(identityPath)
	var saved identity
	if err == nil {
		if json.Unmarshal(data, &saved) != nil {
			return nil, errors.New("agent identity file is invalid")
		}
	}
	var key ed25519.PrivateKey
	if saved.PrivateKey != "" {
		raw, decodeErr := base64.RawURLEncoding.DecodeString(saved.PrivateKey)
		if decodeErr != nil || len(raw) != ed25519.PrivateKeySize {
			return nil, errors.New("agent private key is invalid")
		}
		key = ed25519.PrivateKey(raw)
	} else {
		var generateErr error
		_, key, generateErr = ed25519.GenerateKey(rand.Reader)
		if generateErr != nil {
			return nil, generateErr
		}
	}
	cfg.Agent.IdentityFile = identityPath
	a := &Agent{cfg: cfg, key: key, server: saved.ServerID, client: &http.Client{Timeout: 35 * time.Second}, runner: runner.Runner{Path: cfg.SBPath, Timeout: cfg.Tasks.DefaultTimeout}}
	if err := a.saveIdentity(identityPath); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Agent) saveIdentity(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(identity{PrivateKey: base64.RawURLEncoding.EncodeToString(a.key), ServerID: a.server})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".identity-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (a *Agent) Run(ctx context.Context) error {
	if strings.TrimSpace(a.cfg.Agent.ControllerURL) == "" {
		return errors.New("agent controller_url is empty")
	}
	if a.server == "" {
		if a.cfg.Agent.EnrollmentToken == "" {
			return errors.New("agent enrollment_token is empty")
		}
		if err := a.register(ctx); err != nil {
			return err
		}
	}
	if err := a.heartbeat(ctx); err != nil {
		// A transient controller failure must not stop the Agent forever.
		fmt.Fprintf(os.Stderr, "sb-web agent heartbeat: %v\n", err)
	}
	interval := 30 * time.Second
	if value, err := time.ParseDuration(a.cfg.Agent.HeartbeatInterval); err == nil && value >= 5*time.Second {
		interval = value
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.heartbeat(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "sb-web agent heartbeat: %v\n", err)
				continue
			}
			if err := a.poll(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "sb-web agent poll: %v\n", err)
			}
		}
	}
}

func (a *Agent) register(ctx context.Context) error {
	public := a.key.Public().(ed25519.PublicKey)
	body := map[string]any{"token": a.cfg.Agent.EnrollmentToken, "public_key": base64.RawURLEncoding.EncodeToString(public), "name": hostname(), "arch": runtime.GOARCH}
	data, err := a.post(ctx, "/api/v1/agent/register", body, false)
	if err != nil {
		return err
	}
	var response struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.ServerID == "" {
		return errors.New("controller returned invalid registration response")
	}
	a.server = response.ServerID
	if err := a.saveIdentity(a.cfg.Agent.IdentityFile); err != nil {
		return err
	}
	fmt.Printf("sb-web agent registered as %s\n", a.server)
	return nil
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "remote-server"
	}
	return name
}

func (a *Agent) heartbeat(ctx context.Context) error {
	status, _ := a.localJSON(ctx, "status")
	caps, _ := a.localJSON(ctx, "core.capabilities")
	managerResult, _ := a.runner.Run(ctx, "version")
	body := map[string]any{
		"agent_version":      Version,
		"sb_manager_version": strings.TrimSpace(managerResult.Stdout),
		"core_version":       capsValue(caps, "version"),
		"backend":            "",
		"status":             status,
		"capabilities":       caps,
	}
	_, err := a.post(ctx, "/api/v1/agent/heartbeat", body, true)
	return err
}

func capsValue(data json.RawMessage, key string) string {
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		return ""
	}
	value, _ := object[key].(string)
	return value
}

func (a *Agent) localJSON(ctx context.Context, action string) (json.RawMessage, error) {
	command, err := runner.ActionCommand(action, nil)
	if err != nil {
		return nil, err
	}
	result, err := a.runner.Run(ctx, command...)
	if err != nil {
		return nil, err
	}
	if !json.Valid([]byte(result.Stdout)) {
		return nil, errors.New("sb returned invalid JSON")
	}
	return json.RawMessage(result.Stdout), nil
}

func (a *Agent) poll(ctx context.Context) error {
	data, err := a.post(ctx, "/api/v1/agent/poll", map[string]any{}, true)
	if err != nil {
		return err
	}
	var response struct {
		Task *types.Task `json:"task"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if response.Task == nil {
		return nil
	}
	command, commandErr := runner.ActionCommand(response.Task.Action, response.Task.Args)
	result := runner.Result{}
	if commandErr == nil {
		result, commandErr = a.runner.Run(ctx, command...)
	}
	status := types.TaskSuccess
	problem := ""
	if commandErr != nil {
		status, problem = types.TaskFailed, commandErr.Error()+"\n"+result.Stderr
	}
	_, err = a.post(ctx, "/api/v1/agent/result", map[string]any{"task_id": response.Task.ID, "status": status, "output": result.Stdout, "error": problem}, true)
	return err
}

func (a *Agent) post(ctx context.Context, endpoint string, value any, signed bool) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(a.cfg.Agent.ControllerURL, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if signed {
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		message := req.Method + "\n" + endpoint + "\n" + timestamp + "\n" + string(body)
		signature := ed25519.Sign(a.key, []byte(message))
		req.Header.Set("X-Agent-ID", a.server)
		req.Header.Set("X-Agent-Timestamp", timestamp)
		req.Header.Set("X-Agent-Signature", base64.RawURLEncoding.EncodeToString(signature))
	}
	response, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := ioReadAllLimit(response, 128*1024)
	if err != nil {
		return nil, err
	}
	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("controller returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func ioReadAllLimit(response *http.Response, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("controller response is too large")
	}
	return data, nil
}
