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
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func ResultJSON(result Result) json.RawMessage {
	data, _ := json.Marshal(result)
	return data
}
