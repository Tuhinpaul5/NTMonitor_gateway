package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Node struct {
	IP       string `json:"ip"`
	Username string `json:"username"`
	Password string `json:"password"`
	OS       string `json:"os"`
}

type Config struct {
	Nodes []Node `json:"nodes"`
}

type NodeInfo struct {
	IP          string `json:"ip"`
	OS          string `json:"os"`
	Hostname    string `json:"hostname"`
	User        string `json:"user"`
	Kernel      string `json:"kernel"`
	Architecture string `json:"architecture"`
	Uptime      string `json:"uptime"`
	RawOutput   string `json:"raw_output,omitempty"`
	Error       string `json:"error,omitempty"`
}

func main() {
	configPath := "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	for _, node := range cfg.Nodes {
		info := gatherNodeInfo(node)
		output, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(output))
	}
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if len(cfg.Nodes) == 0 {
		return nil, errors.New("config must include at least one node")
	}

	return &cfg, nil
}

func gatherNodeInfo(node Node) NodeInfo {
	info := NodeInfo{
		IP: node.IP,
		OS: strings.ToLower(node.OS),
	}

	client, err := sshClient(node)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer client.Close()

	if info.OS == "windows" {
		info.RawOutput, err = runSSHCommand(client, `cmd.exe /C "systeminfo & echo --- & whoami"`)
		info.Hostname = parseWindowsField(info.RawOutput, "Host Name")
		info.User = parseWindowsField(info.RawOutput, "User Name")
		info.Kernel = parseWindowsField(info.RawOutput, "OS Version")
		info.Architecture = parseWindowsField(info.RawOutput, "System Type")
		info.Uptime = parseWindowsField(info.RawOutput, "System Boot Time")
	} else {
		info.Hostname, err = runSSHCommand(client, "hostname")
		if err != nil {
			info.Error = err.Error()
			return info
		}
		info.Hostname = strings.TrimSpace(info.Hostname)

		info.User, _ = runSSHCommand(client, "whoami")
		info.User = strings.TrimSpace(info.User)

		info.Kernel, _ = runSSHCommand(client, "uname -sr")
		info.Kernel = strings.TrimSpace(info.Kernel)

		info.Architecture, _ = runSSHCommand(client, "uname -m")
		info.Architecture = strings.TrimSpace(info.Architecture)

		info.Uptime, _ = runSSHCommand(client, "cat /proc/uptime | awk '{print $1}'")
		info.Uptime = strings.TrimSpace(info.Uptime)
	}

	return info
}

func sshClient(node Node) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: node.Username,
		Auth: []ssh.AuthMethod{ssh.Password(node.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", node.IP), config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func runSSHCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

func parseWindowsField(output, field string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, field) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}
