package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	IP           string `json:"ip"`
	OS           string `json:"os"`
	Hostname     string `json:"hostname"`
	User         string `json:"user"`
	Kernel       string `json:"kernel"`
	Architecture string `json:"architecture"`
	Uptime       string `json:"uptime"`
	RawOutput    string `json:"raw_output,omitempty"`
	Error        string `json:"error,omitempty"`
}

var (
	globalConfig *Config
	globalClient *WSClient
)

func main() {
	// Load configuration
	configPath := getEnv("CONFIG_PATH", "config.json")
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	globalConfig = cfg
	log.Printf("Loaded configuration with %d nodes", len(cfg.Nodes))

	// Get WebSocket configuration from environment
	wsURL := getEnv("WS_URL", "ws://localhost:8080/api/gateway/ws")
	gatewayID := getEnv("GATEWAY_ID", "gateway-1")
	apiKey := getEnv("API_KEY", "")

	if wsURL == "" || gatewayID == "" {
		log.Fatal("WS_URL and GATEWAY_ID must be set")
	}

	log.Printf("Starting gateway client: %s connecting to %s", gatewayID, wsURL)

	// Create WebSocket client
	globalClient = NewWSClient(wsURL, gatewayID, apiKey, handleIncomingMessage)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutdown signal received, closing connection...")
		globalClient.Close()
		os.Exit(0)
	}()

	// Connect and run (blocks here)
	if err := globalClient.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Run the client (blocks in read loop)
	globalClient.Run()
}

// handleIncomingMessage processes messages received from the WebSocket server
func handleIncomingMessage(data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		log.Println("Message missing 'type' field")
		return
	}

	switch msgType {
	case "execute_command":
		handleExecuteCommand(data)
	case "ping":
		log.Println("Received ping from server")
	default:
		log.Printf("Unknown message type: %s", msgType)
	}
}

// handleExecuteCommand executes SSH command on specified node
func handleExecuteCommand(data []byte) {
	var cmd CommandMessage
	if err := json.Unmarshal(data, &cmd); err != nil {
		log.Printf("Failed to parse command message: %v", err)
		return
	}

	log.Printf("Executing command on node %s: %s", cmd.NodeIP, cmd.Command)

	// Find the node in config
	var targetNode *Node
	for _, node := range globalConfig.Nodes {
		if node.IP == cmd.NodeIP {
			targetNode = &node
			break
		}
	}

	response := ResponseMessage{
		Type:      "command_response",
		CommandID: cmd.CommandID,
	}

	if targetNode == nil {
		response.Success = false
		response.Error = fmt.Sprintf("Node with IP %s not found in configuration", cmd.NodeIP)
		log.Printf("Error: %s", response.Error)
		sendResponse(response)
		return
	}

	// Execute SSH command
	client, err := sshClient(*targetNode)
	if err != nil {
		response.Success = false
		response.Error = fmt.Sprintf("SSH connection failed: %v", err)
		log.Printf("Error: %s", response.Error)
		sendResponse(response)
		return
	}
	defer client.Close()

	output, err := runSSHCommand(client, cmd.Command)
	if err != nil {
		response.Success = false
		response.Error = fmt.Sprintf("Command execution failed: %v", err)
		response.Output = output // Include partial output if available
		log.Printf("Command failed: %s", response.Error)
	} else {
		response.Success = true
		response.Output = output
		log.Printf("Command executed successfully, output length: %d bytes", len(output))
	}

	sendResponse(response)
}

// sendResponse sends a response back to the WebSocket server
func sendResponse(response ResponseMessage) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	if globalClient != nil {
		globalClient.Send(data)
		log.Printf("Sent response for command %s", response.CommandID)
	} else {
		log.Println("Cannot send response: client not initialized")
	}
}

// getEnv returns environment variable value or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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

	// Load credentials from environment variables if not in config
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Password == "" {
			envKey := fmt.Sprintf("NODE_%s_PASSWORD", strings.ReplaceAll(cfg.Nodes[i].IP, ".", "_"))
			cfg.Nodes[i].Password = os.Getenv(envKey)
		}
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
		User:            node.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(node.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
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
