package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActionCommandWhitelist(t *testing.T) {
	if _, err := ActionCommand("not-allowed", nil); err == nil {
		t.Fatal("unsupported action was accepted")
	}
	if _, err := ActionCommand("core.update", map[string]any{"version": "1.14.0;id"}); err == nil {
		t.Fatal("unsafe version was accepted")
	}
	args, err := ActionCommand("hy2-buffer.enable", nil)
	if err != nil || len(args) != 3 || args[0] != "hy2" {
		t.Fatalf("unexpected action command: %#v, %v", args, err)
	}
}

func TestNodeAddCommandIncludes14AndSnellOptions(t *testing.T) {
	args, err := ActionCommand("node.add", map[string]any{
		"protocol":              "snell",
		"id":                    "snell-v6",
		"address":               "edge.example.com",
		"port":                  float64(6160),
		"snell_version":         "6",
		"snell_mode":            "default",
		"realm_id":              "slot-a",
		"realm_ip_version":      "6",
		"realm_port_mapping":    true,
		"disable_chrome_parrot": true,
		"brutal_debug":          true,
		"obfs_min_packet_size":  float64(64),
		"obfs_max_packet_size":  float64(1500),
	})
	if err != nil {
		t.Fatalf("node add rejected: %v", err)
	}
	want := []string{"node", "add", "snell", "--id", "snell-v6", "--address", "edge.example.com", "--snell-version", "6", "--snell-mode", "default", "--realm-id", "slot-a", "--realm-ip-version", "6", "--port", "6160", "--obfs-min-packet-size", "64", "--obfs-max-packet-size", "1500", "--disable-chrome-parrot", "--brutal-debug", "--realm-port-mapping"}
	if got := strings.Join(args, " "); got != strings.Join(want, " ") {
		t.Fatalf("unexpected node command: %v", args)
	}
}

func TestRealmEnableCommandValidation(t *testing.T) {
	args, err := ActionCommand("realm.enable", map[string]any{"port": float64(9443), "public_url": "https://relay.example.com", "listen": "::", "tls_domain": "relay.example.com", "max_realms": float64(10)})
	if err != nil {
		t.Fatalf("realm enable rejected: %v", err)
	}
	if got := strings.Join(args, " "); got != "realm enable 9443 https://relay.example.com --listen :: --tls-domain relay.example.com --max-realms 10" {
		t.Fatalf("unexpected Realm command: %v", args)
	}
	if _, err := ActionCommand("realm.enable", map[string]any{"port": float64(9443), "public_url": "https://relay.example.com/path"}); err == nil {
		t.Fatal("Realm URL with a path was accepted")
	}
}

func TestNodeSetCommandIncludesProtocolFields(t *testing.T) {
	args, err := ActionCommand("node.set", map[string]any{
		"id":                    "hy2-edit",
		"obfs":                  "gecko",
		"obfs_min_packet_size":  float64(600),
		"obfs_max_packet_size":  float64(1100),
		"disable_chrome_parrot": false,
		"bbr_profile":           "standard",
		"brutal_debug":          true,
		"realm_id":              "slot2",
		"realm_ip_version":      "6",
		"realm_port_mapping":    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, " ")
	for _, expected := range []string{"--obfs gecko", "--obfs-min-packet-size 600", "--obfs-max-packet-size 1100", "--enable-chrome-parrot", "--bbr-profile standard", "--brutal-debug", "--realm-id slot2", "--realm-ip-version 6", "--no-realm-port-mapping"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("node set missing %q: %s", expected, got)
		}
	}
}

func TestOperationsCommandMappings(t *testing.T) {
	tests := []struct {
		action string
		args   map[string]any
		want   string
	}{
		{"node.enable-all", map[string]any{"tag": "edge"}, "node enable-all --tag edge"},
		{"node.disable-all", map[string]any{"region": "hk"}, "node disable-all --region hk"},
		{"share.all", nil, "share all"},
		{"config.diff", nil, "--json config diff"},
		{"firewall.ufw-allow", nil, "firewall ufw-allow --yes"},
		{"service.restart", nil, "restart"},
		{"traffic.reconcile", nil, "traffic reconcile"},
		{"health.configure", map[string]any{"disk_free": "10"}, "health configure --disk-free 10"},
	}
	for _, test := range tests {
		args, err := ActionCommand(test.action, test.args)
		if err != nil || strings.Join(args, " ") != test.want {
			t.Fatalf("%s => %v (%v), want %s", test.action, args, err, test.want)
		}
	}
}

func TestRunnerTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-sb")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	r := Runner{Path: path, Timeout: 30 * time.Millisecond}
	result, err := r.Run(context.Background(), "status")
	if err == nil || !result.TimedOut {
		t.Fatalf("expected timeout, result=%#v err=%v", result, err)
	}
}
