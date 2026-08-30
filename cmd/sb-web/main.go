package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/agent"
	"github.com/R1ddle1337/sb-manager-web/internal/api"
	"github.com/R1ddle1337/sb-manager-web/internal/auth"
	"github.com/R1ddle1337/sb-manager-web/internal/config"
	"github.com/R1ddle1337/sb-manager-web/internal/helper"
	"github.com/R1ddle1337/sb-manager-web/internal/runner"
	"github.com/R1ddle1337/sb-manager-web/internal/storage"
)

const defaultConfigPath = "/etc/sb-manager-web/config.json"

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "sb-web: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return serve(args)
	}
	switch args[0] {
	case "server", "enable":
		if args[0] == "enable" {
			return serviceAction("start")
		}
		return serve(args[1:])
	case "disable", "restart", "logs":
		return serviceAction(args[0])
	case "agent":
		return runAgent(args[1:])
	case "helper":
		return runHelper(args[1:])
	case "join":
		return join(args[1:])
	case "status":
		return status(args[1:])
	case "reset-admin-password":
		return resetPassword(args[1:])
	case "init":
		return initialize(args[1:])
	case "version", "--version", "-V":
		fmt.Printf("sb-manager-web %s\n", version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "配置文件")
	listen := fs.String("listen", "", "覆盖监听地址")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.LogDir, 0750); err != nil {
		return err
	}
	if _, err := os.Stat(*configPath); errors.Is(err, os.ErrNotExist) {
		if err := config.Save(*configPath, cfg); err != nil {
			return err
		}
	}
	store, err := storage.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	handler, initialCredential, err := api.New(cfg, store)
	if err != nil {
		return err
	}
	if initialCredential.Password != "" {
		fmt.Printf("首次管理员账号：%s\n首次管理员密码：%s\n请登录后立即修改密码。\n", initialCredential.Username, initialCredential.Password)
	}
	httpServer := &http.Server{Addr: cfg.Listen, Handler: handler.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	scheme := "http"
	if cfg.TLS.Enabled {
		scheme = "https"
	}
	fmt.Printf("sb-manager-web listening on %s\n", cfg.Listen)
	fmt.Printf("面板访问地址：%s://%s\n", scheme, cfg.Listen)
	if cfg.TLS.Enabled {
		if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
			return errors.New("tls enabled but cert_file/key_file are empty")
		}
		tlsConfig, tlsErr := handler.TLSConfig()
		if tlsErr != nil {
			return fmt.Errorf("prepare agent mTLS: %w", tlsErr)
		}
		httpServer.TLSConfig = tlsConfig
		err = httpServer.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		err = httpServer.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func initialize(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "配置文件")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return err
	}
	store, err := storage.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	_, credential, err := api.New(cfg, store)
	if err != nil {
		return err
	}
	if credential.Password == "" {
		username, usernameErr := auth.New(store).AdminUsername()
		if usernameErr == nil {
			fmt.Printf("管理员账号：%s\n", username)
		}
		fmt.Println("管理员密码已存在且不会回显；需要重置时运行：sb-web reset-admin-password")
	} else {
		fmt.Printf("首次管理员账号：%s\n首次管理员密码：%s\n请登录后立即修改密码。\n", credential.Username, credential.Password)
	}
	return config.Save(*configPath, cfg)
}

func serviceAction(action string) error {
	return serviceActionFor(action, "sb-manager-web.service", "sb-manager-web")
}

func serviceActionFor(action, systemdUnit, openRCService string) error {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		cmd := "systemctl"
		args := []string{action, systemdUnit}
		if action == "start" {
			args = []string{"enable", "--now", systemdUnit}
		}
		if action == "disable" {
			args = []string{"disable", "--now", systemdUnit}
		}
		if action == "restart" {
			args = []string{"restart", systemdUnit}
		}
		if action == "logs" {
			cmd, args = "journalctl", []string{"-u", systemdUnit, "-n", "100", "--no-pager"}
		}
		return runExternal(cmd, args...)
	}
	if action == "logs" {
		return runExternal("tail", "-n", "100", "/var/log/sb-manager-web/server.log")
	}
	return runExternal("rc-service", openRCService, action)
}

func runExternal(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func runAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "配置文件")
	controller := fs.String("controller", "", "控制端 URL")
	token := fs.String("token", "", "加入令牌")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *controller != "" {
		cfg.Agent.ControllerURL = *controller
	}
	if *token != "" {
		cfg.Agent.EnrollmentToken = *token
	}
	a, err := agent.New(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err = a.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runHelper(args []string) error {
	fs := flag.NewFlagSet("helper", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "配置文件")
	socket := fs.String("socket", "", "覆盖 Unix Socket")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *socket != "" {
		cfg.HelperSocket = *socket
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return helper.Server{Socket: cfg.HelperSocket, Runner: runner.Runner{Path: cfg.SBPath, Timeout: cfg.Tasks.DefaultTimeout}}.Serve(ctx)
}

func join(args []string) error {
	if len(args) < 2 {
		return errors.New("用法：sb-web join CONTROLLER_URL TOKEN [--config PATH]")
	}
	controller, token := args[0], args[1]
	if err := validateControllerURL(controller); err != nil {
		return err
	}
	configPath := defaultConfigPath
	for i := 2; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			configPath = args[i+1]
			i++
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	cfg.Agent.Enabled, cfg.Agent.ControllerURL, cfg.Agent.EnrollmentToken = true, controller, token
	if cfg.Agent.IdentityFile == "" {
		cfg.Agent.IdentityFile = filepath.Join(cfg.DataDir, "agent-identity", "ed25519.json")
	}
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("已保存 Agent 配置：%s\n", configPath)
	a, err := agent.New(cfg)
	if err != nil {
		return err
	}
	if err := a.Register(context.Background()); err != nil {
		return err
	}
	cfg.Agent.EnrollmentToken = ""
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}
	if err := serviceActionFor("start", "sb-manager-web-agent.service", "sb-manager-web-agent"); err != nil {
		fmt.Println("Agent 已注册，但未能自动启动服务；请手动运行 sb-web agent 或检查 init 系统。")
		return err
	}
	fmt.Println("Agent 已注册并启动。")
	return nil
}

func validateControllerURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("控制端 URL 无效")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1") {
		return nil
	}
	return errors.New("远程控制端必须使用 HTTPS（本机 localhost 可使用 HTTP）")
}

func status(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listen := fs.String("listen", "127.0.0.1:9091", "WebUI 地址")
	if err := fs.Parse(args); err != nil {
		return err
	}
	response, err := http.Get("http://" + *listen + "/healthz")
	if err != nil {
		return fmt.Errorf("WebUI 不可用：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("WebUI 返回 HTTP %d", response.StatusCode)
	}
	fmt.Printf("sb-manager-web：运行中 (%s)\n", *listen)
	return nil
}

func resetPassword(args []string) error {
	if len(args) > 2 {
		return errors.New("用法：sb-web reset-admin-password [用户名] [新密码]")
	}
	cfg, err := config.Load(defaultConfigPath)
	if err != nil {
		return err
	}
	store, err := storage.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	manager := auth.New(store)
	username, err := manager.AdminUsername()
	if err != nil {
		return err
	}
	password := ""
	if len(args) == 2 {
		username, password = args[0], args[1]
	} else if len(args) == 1 {
		password = args[0]
	} else {
		password, err = auth.RandomToken(18)
		if err != nil {
			return err
		}
	}
	if err := manager.SetPassword(username, password); err != nil {
		return err
	}
	fmt.Printf("账号 %s 的新管理员密码：%s\n", username, password)
	return nil
}

func usage() {
	fmt.Print(`sb-manager-web：Go WebUI 控制层

用法：
  sb-web server [--config PATH] [--listen HOST:PORT]
  sb-web enable                         启用并启动 WebUI 服务
  sb-web disable|restart|logs           服务管理
  sb-web agent [--config PATH]
  sb-web join CONTROLLER_URL TOKEN
  sb-web status [--listen HOST:PORT]
  sb-web init [--config PATH]           初始化配置和管理员账号
  sb-web reset-admin-password [USERNAME] [PASSWORD]
  sb-web version
`)
}
