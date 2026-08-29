package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Result struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	TimedOut bool   `json:"timed_out,omitempty"`
}

type Runner struct {
	Path    string
	Timeout time.Duration
}

var safeVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)
var safeID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,47}$`)
var safeProtocol = regexp.MustCompile(`^(vmess|ss|anytls|hy2|trojan|tuic|vless|naive|shadowtls|snell)$`)
var safeDomain = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$`)

func (r Runner) Run(ctx context.Context, args ...string) (Result, error) {
	if r.Path == "" {
		return Result{}, errors.New("sb path is empty")
	}
	if len(args) == 0 {
		return Result{}, errors.New("sb command is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, r.Path, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if runCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result, fmt.Errorf("sb command timed out after %s", timeout)
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("sb exited with status %d", result.ExitCode)
	}
	return result, err
}

func JSONArgs(command ...string) []string {
	return append([]string{"--json"}, command...)
}

func ActionCommand(action string, args map[string]any) ([]string, error) {
	switch action {
	case "status":
		return JSONArgs("status"), nil
	case "nodes.list":
		return JSONArgs("node", "list"), nil
	case "core.capabilities":
		return JSONArgs("core", "capabilities"), nil
	case "bbr.status":
		return JSONArgs("bbr", "status"), nil
	case "bbr.enable":
		return []string{"bbr", "enable"}, nil
	case "bbr.disable":
		return []string{"bbr", "disable"}, nil
	case "hy2-buffer.status":
		return JSONArgs("hy2", "buffer", "status"), nil
	case "hy2-buffer.enable":
		return []string{"hy2", "buffer", "enable"}, nil
	case "hy2-buffer.disable":
		return []string{"hy2", "buffer", "disable"}, nil
	case "core.check":
		return JSONArgs("core", "check"), nil
	case "core.update":
		version, _ := args["version"].(string)
		if version != "" {
			if !safeVersion.MatchString(version) {
				return nil, errors.New("invalid core version")
			}
			return []string{"core", "update", version}, nil
		}
		return []string{"core", "update"}, nil
	case "core.rollback":
		return []string{"core", "rollback"}, nil
	case "doctor":
		return JSONArgs("doctor"), nil
	case "doctor.repair-safe":
		return []string{"doctor", "--repair-safe"}, nil
	case "backup.create":
		return []string{"backup"}, nil
	case "health.check":
		return JSONArgs("health", "check"), nil
	case "logs":
		return []string{"logs", "all", "100"}, nil
	case "node.enable", "node.disable", "node.delete":
		id, ok := args["id"].(string)
		if !ok || !safeID.MatchString(id) {
			return nil, errors.New("invalid node id")
		}
		verb := strings.TrimPrefix(action, "node.")
		return []string{"node", verb, id}, nil
	case "node.add":
		return nodeAddCommand(args)
	case "node.set":
		return nodeSetCommand(args)
	case "node.show":
		id, err := requiredID(args, "id")
		if err != nil {
			return nil, err
		}
		return JSONArgs("node", "show", id), nil
	case "node.share":
		id, err := requiredID(args, "id")
		if err != nil {
			return nil, err
		}
		command := []string{"share", id}
		if userID, _ := args["user_id"].(string); userID != "" {
			if !safeID.MatchString(userID) {
				return nil, errors.New("invalid user id")
			}
			command = append(command, "--user", userID)
		}
		return command, nil
	case "users.list":
		nodeID, err := requiredID(args, "node_id")
		if err != nil {
			return nil, err
		}
		return JSONArgs("user", "list", nodeID), nil
	case "user.add":
		nodeID, err := requiredID(args, "node_id")
		if err != nil {
			return nil, err
		}
		userID, err := requiredID(args, "user_id")
		if err != nil {
			return nil, err
		}
		name, _ := args["name"].(string)
		if name == "" {
			name = userID
		}
		if len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
			return nil, errors.New("invalid user name")
		}
		return []string{"user", "add", nodeID, userID, name}, nil
	case "user.enable", "user.disable", "user.delete", "user.rotate":
		nodeID, err := requiredID(args, "node_id")
		if err != nil {
			return nil, err
		}
		userID, err := requiredID(args, "user_id")
		if err != nil {
			return nil, err
		}
		return []string{"user", strings.TrimPrefix(action, "user."), nodeID, userID}, nil
	case "cert.list":
		return JSONArgs("cert", "list"), nil
	case "cert.issue":
		domain, _ := args["domain"].(string)
		if !safeDomain.MatchString(domain) {
			return nil, errors.New("invalid certificate domain")
		}
		command := []string{"cert", "issue", domain}
		if email, _ := args["email"].(string); email != "" {
			if len(email) > 254 || !strings.Contains(email, "@") || strings.ContainsAny(email, "\r\n\x00") {
				return nil, errors.New("invalid email")
			}
			command = append(command, email)
		}
		return command, nil
	case "cert.renew":
		return []string{"cert", "renew"}, nil
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func requiredID(args map[string]any, key string) (string, error) {
	value, _ := args[key].(string)
	if !safeID.MatchString(value) {
		return "", fmt.Errorf("invalid %s", strings.ReplaceAll(key, "_", " "))
	}
	return value, nil
}

func nodeAddCommand(args map[string]any) ([]string, error) {
	protocol, _ := args["protocol"].(string)
	id, _ := args["id"].(string)
	if !safeProtocol.MatchString(protocol) || !safeID.MatchString(id) {
		return nil, errors.New("invalid node protocol or id")
	}
	command := []string{"node", "add", protocol, "--id", id}
	for _, field := range []struct{ key, flag string }{{"name", "--name"}, {"domain", "--domain"}, {"address", "--address"}, {"path", "--path"}, {"method", "--method"}, {"snell_version", "--snell-version"}, {"snell_mode", "--snell-mode"}, {"obfs", "--obfs"}, {"masquerade", "--masquerade"}} {
		if value, ok := args[field.key].(string); ok && value != "" {
			if len(value) > 253 || strings.ContainsAny(value, "\r\n\x00") {
				return nil, errors.New("invalid node value")
			}
			command = append(command, field.flag, value)
		}
	}
	if port, ok := args["port"].(float64); ok {
		if port < 1 || port > 65535 || port != float64(int(port)) {
			return nil, errors.New("invalid node port")
		}
		command = append(command, "--port", fmt.Sprintf("%d", int(port)))
	}
	if disabled, ok := args["disabled"].(bool); ok && disabled {
		command = append(command, "--disabled")
	}
	return command, nil
}

func nodeSetCommand(args map[string]any) ([]string, error) {
	id, _ := args["id"].(string)
	if !safeID.MatchString(id) {
		return nil, errors.New("invalid node id")
	}
	command := []string{"node", "set", id}
	for _, field := range []struct{ key, flag string }{{"name", "--name"}, {"address", "--address"}, {"domain", "--domain"}, {"path", "--path"}, {"remark", "--remark"}, {"region", "--region"}, {"purpose", "--purpose"}, {"line", "--line"}, {"tags", "--tags"}} {
		if value, ok := args[field.key].(string); ok {
			if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
				return nil, errors.New("invalid node value")
			}
			command = append(command, field.flag, value)
		}
	}
	if port, ok := args["port"].(float64); ok {
		if port < 1 || port > 65535 || port != float64(int(port)) {
			return nil, errors.New("invalid node port")
		}
		command = append(command, "--port", fmt.Sprintf("%d", int(port)))
	}
	if len(command) == 3 {
		return nil, errors.New("node.set requires at least one field")
	}
	return command, nil
}

func ResultJSON(result Result) json.RawMessage {
	data, _ := json.Marshal(result)
	return data
}
