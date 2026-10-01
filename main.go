package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultHostname = "karo.salkaro.com"

type config struct {
	TunnelID        string `json:"tunnel_id"`
	CredentialsFile string `json:"credentials_file"`
	Hostname        string `json:"hostname"`
}

type options struct {
	Protocol string
	Port     int
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "karo: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	cloudflared, err := exec.LookPath("cloudflared")
	if err != nil {
		return errors.New("cloudflared is not installed or is not on PATH")
	}

	if err := checkLocalService(opts.Port); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	configPath, err := writeTunnelConfig(cfg, opts)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	publicURL := "https://" + cfg.Hostname
	localTarget := fmt.Sprintf("%s://localhost:%d", opts.Protocol, opts.Port)

	cmd := exec.Command(cloudflared, "tunnel", "--config", configPath, "run", cfg.TunnelID)
	// Keep cloudflared out of the terminal process group so signals are forwarded once.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.New("could not capture cloudflared output")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.New("could not capture cloudflared output")
	}
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return errors.New("could not start cloudflared")
	}

	return runTunnelUI(cmd, stdout, stderr, publicURL, localTarget)
}

func parseArgs(args []string) (options, error) {
	if len(args) != 2 {
		return options{}, errors.New("usage: karo <http|https> <port>")
	}
	if args[0] != "http" && args[0] != "https" {
		return options{}, fmt.Errorf("unsupported protocol %q: use http or https", args[0])
	}

	port, err := validatePort(args[1])
	if err != nil {
		return options{}, err
	}
	return options{Protocol: args[0], Port: port}, nil
}

func validatePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q: use a number from 1 to 65535", value)
	}
	return port, nil
}

func checkLocalService(port int) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return fmt.Errorf("local service at localhost:%d is not reachable", port)
	}
	return conn.Close()
}

func loadConfig() (config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, errors.New("could not determine the home directory")
	}

	path := filepath.Join(home, ".config", "karo", "config.json")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config{}, fmt.Errorf("configuration not found: create %s", path)
		}
		return config{}, errors.New("could not read configuration")
	}
	defer file.Close()

	var cfg config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return config{}, errors.New("configuration is not valid JSON")
	}
	if strings.TrimSpace(cfg.TunnelID) == "" {
		return config{}, errors.New("configuration is missing tunnel_id")
	}
	if strings.TrimSpace(cfg.CredentialsFile) == "" {
		return config{}, errors.New("configuration is missing credentials_file")
	}
	if cfg.Hostname == "" {
		cfg.Hostname = defaultHostname
	}
	if cfg.Hostname != defaultHostname {
		return config{}, fmt.Errorf("configuration hostname must be %s", defaultHostname)
	}
	if info, err := os.Stat(cfg.CredentialsFile); err != nil || info.IsDir() {
		return config{}, errors.New("configured tunnel credentials file is not accessible")
	}
	return cfg, nil
}

func writeTunnelConfig(cfg config, opts options) (string, error) {
	file, err := os.CreateTemp("", "karo-*.yml")
	if err != nil {
		return "", errors.New("could not create temporary tunnel configuration")
	}
	path := file.Name()

	service := fmt.Sprintf("%s://localhost:%d", opts.Protocol, opts.Port)
	contents := fmt.Sprintf("tunnel: %s\ncredentials-file: %s\ningress:\n  - hostname: %s\n    service: %s\n", yamlString(cfg.TunnelID), yamlString(cfg.CredentialsFile), yamlString(cfg.Hostname), yamlString(service))
	if opts.Protocol == "https" {
		contents += "    originRequest:\n      noTLSVerify: true\n"
	}
	contents += "  - service: http_status:404\n"

	if _, err := file.WriteString(contents); err != nil {
		file.Close()
		os.Remove(path)
		return "", errors.New("could not write temporary tunnel configuration")
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", errors.New("could not finalize temporary tunnel configuration")
	}
	return path, nil
}

func yamlString(value string) string {
	return strconv.Quote(value)
}
