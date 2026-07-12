# Architecture Overview

## System Design

```
┌──────────────────────────────────────────────────────────────────┐
│                         Dashboard Server                          │
│                     (Railway/Your Server)                         │
│                                                                    │
│  ┌────────────────────────────────────────────────────────┐     │
│  │         WebSocket Server (/api/gateway/ws)             │     │
│  │                                                          │     │
│  │  • Accept gateway connections                           │     │
│  │  • Handle registration                                  │     │
│  │  • Send commands to gateways                           │     │
│  │  • Receive command responses                           │     │
│  └────────────────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────────────────┘
                              ▲
                              │ WSS (Secure WebSocket)
                              │ Persistent Connection
                              │
┌─────────────────────────────▼─────────────────────────────────────┐
│                      Gateway Client                                │
│                   (This Application)                               │
│                   (Railway/Home PC)                                │
│                                                                    │
│  ┌──────────────────┐  ┌─────────────────┐  ┌────────────────┐ │
│  │   main.go        │  │  ws_client.go   │  │  config.json   │ │
│  │                  │  │                 │  │                │ │
│  │ • Load config    │  │ • Dial WS      │  │ • Node list    │ │
│  │ • Handle msgs    │  │ • Register     │  │ • Credentials  │ │
│  │ • Execute SSH    │  │ • Ping/Pong    │  │                │ │
│  │ • Send response  │  │ • Reconnect    │  │                │ │
│  └──────────────────┘  └─────────────────┘  └────────────────┘ │
└────────────────────────────────────────────────────────────────────┘
                              │
                              │ SSH (Port 22)
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
    ┌────▼────┐          ┌────▼────┐         ┌────▼────┐
    │ Node 1  │          │ Node 2  │         │ Node N  │
    │ Linux   │          │ Windows │         │  ...    │
    │         │          │         │         │         │
    │ SSH     │          │ SSH     │         │ SSH     │
    │ Server  │          │ Server  │         │ Server  │
    └─────────┘          └─────────┘         └─────────┘
```

---

## Component Breakdown

### 1. Gateway Client (main.go)

**Responsibilities:**
- Load configuration from `config.json`
- Read credentials from environment variables
- Create and manage WebSocket connection
- Handle incoming command messages
- Execute SSH commands on target nodes
- Send responses back to dashboard

**Key Functions:**
- `main()` - Entry point, initializes and blocks on WS connection
- `handleIncomingMessage()` - Routes incoming messages by type
- `handleExecuteCommand()` - Executes SSH command on specified node
- `sendResponse()` - Sends command response to dashboard
- `loadConfig()` - Loads node configuration with env var credentials
- `sshClient()` - Creates SSH connection to node
- `runSSHCommand()` - Executes command on SSH session

**State:**
- `globalConfig` - Node configuration
- `globalClient` - WebSocket client instance

### 2. WebSocket Client (ws_client.go)

**Responsibilities:**
- Establish WebSocket connection
- Register gateway with server
- Maintain connection with ping/pong heartbeat
- Auto-reconnect with exponential backoff
- Send/receive messages

**Key Components:**
- `WSClient` struct - Connection state and configuration
- `Connect()` - Initial connection with retry logic
- `register()` - Send registration message
- `Run()` - Start read/write pumps (blocks)
- `readPump()` - Read incoming messages, handle pongs
- `writePump()` - Send outgoing messages, send pings
- `reconnect()` - Reconnection logic with backoff
- `Send()` - Queue message for sending

**Connection Management:**
- Initial backoff: 1 second
- Max backoff: 5 minutes
- Backoff factor: 2.0
- Ping interval: 54 seconds (90% of pong wait)
- Pong timeout: 60 seconds
- Max message size: 512 KB

### 3. Configuration (config.json)

**Structure:**
```json
{
  "nodes": [
    {
      "ip": "192.168.1.21",
      "username": "ratnadeep",
      "password": "",  // Load from env: NODE_192_168_1_21_PASSWORD
      "os": "linux"    // or "windows"
    }
  ]
}
```

**Credential Resolution:**
1. Check `password` field in config
2. If empty, check env var: `NODE_<IP_WITH_UNDERSCORES>_PASSWORD`
3. Example: `192.168.1.21` → `NODE_192_168_1_21_PASSWORD`

---

## Message Protocol

### Registration (Gateway → Server)

Sent immediately after WebSocket connection established.

```json
{
  "type": "register",
  "gateway_id": "gateway-home-pc",
  "api_key": "optional-secret-key"
}
```

### Execute Command (Server → Gateway)

```json
{
  "type": "execute_command",
  "command_id": "550e8400-e29b-41d4-a716-446655440000",
  "node_ip": "192.168.1.21",
  "command": "uptime"
}
```

**Fields:**
- `type` - Always "execute_command"
- `command_id` - Unique identifier (UUID recommended)
- `node_ip` - IP address from config.json
- `command` - Shell command to execute

### Command Response (Gateway → Server)

```json
{
  "type": "command_response",
  "command_id": "550e8400-e29b-41d4-a716-446655440000",
  "success": true,
  "output": " 10:45:32 up 5 days...\n",
  "error": ""
}
```

**Success Response:**
- `success: true`
- `output` - Command stdout/stderr
- `error` - Empty string

**Failure Response:**
- `success: false`
- `output` - Partial output (if any)
- `error` - Error description

### Ping (Server → Gateway)

Optional application-level ping (separate from WebSocket ping frames).

```json
{
  "type": "ping"
}
```

---

## Connection Lifecycle

### 1. Startup

```
1. Load config.json
2. Load environment variables
3. Create WSClient instance
4. Call client.Connect()
   a. Dial WebSocket URL
   b. If failed, wait backoff, retry
   c. Once connected, send register message
   d. If register fails, close and retry
5. Call client.Run()
   a. Start writePump() goroutine
   b. Start readPump() (blocks in main)
```

### 2. Normal Operation

```
┌─ readPump goroutine ─────────────────┐
│                                       │
│  while true:                          │
│    message = conn.ReadMessage()       │
│    if pong frame:                     │
│      update deadline                  │
│    else if text message:              │
│      call onMessage handler           │
│        → handleIncomingMessage()      │
│        → handleExecuteCommand()       │
│        → SSH connect & execute        │
│        → Send response via sendCh     │
└───────────────────────────────────────┘

┌─ writePump goroutine ────────────────┐
│                                       │
│  while true:                          │
│    select:                            │
│      case msg from sendCh:            │
│        conn.WriteMessage(msg)         │
│      case <-ticker (every 54s):       │
│        conn.WriteMessage(Ping)        │
│      case <-done:                     │
│        return                         │
└───────────────────────────────────────┘
```

### 3. Disconnection & Reconnect

```
1. Connection error detected in readPump
2. Close connection
3. Set reconnecting flag
4. Start reconnect() goroutine
   a. Wait backoff duration
   b. Try to dial WebSocket
   c. If failed, increase backoff, retry
   d. If success, send register message
   e. If register fails, close and retry
   f. If register success:
      - Reset backoff
      - Clear reconnecting flag
      - Restart pumps
```

### 4. Graceful Shutdown

```
1. Receive SIGINT or SIGTERM
2. Call client.Close()
   a. Close done channel
   b. Stop writePump
   c. Close WebSocket connection
3. Exit process
```

---

## Error Handling

### Configuration Errors
- Missing config.json → Fatal error, exit
- No nodes in config → Fatal error, exit
- Missing WS_URL or GATEWAY_ID → Fatal error, exit

### Connection Errors
- Initial connection fails → Retry with exponential backoff
- Registration fails → Close, retry connection
- Connection lost → Auto-reconnect with exponential backoff

### SSH Errors
- Node not in config → Return error response
- SSH connection fails → Return error response
- Command execution fails → Return error response with partial output

### Message Errors
- Invalid JSON → Log warning, continue
- Unknown message type → Log warning, continue
- Missing required field → Log warning, continue

---

## Concurrency Model

### Goroutines

1. **Main goroutine** - Blocks in `readPump()`
2. **Write pump goroutine** - Sends messages and pings
3. **Reconnect goroutine** - Spawned when connection lost
4. **Signal handler goroutine** - Listens for SIGINT/SIGTERM
5. **Command handler goroutines** - One per incoming command (via `go c.onMessage(message)`)

### Synchronization

- **sendCh** - Buffered channel (256) for outgoing messages
- **done** - Unbuffered channel for shutdown signal
- **No explicit locks** - State managed via channels

### Message Flow

```
Dashboard → readPump → onMessage handler goroutine → SSH execution
                                                    ↓
Dashboard ← writePump ← sendCh ← sendResponse() ← SSH result
```

---

## Security Considerations

### 1. Credentials
- Passwords in environment variables, not config file
- Config file should be in `.gitignore`
- Consider SSH key authentication for production

### 2. Transport
- Use WSS (TLS) in production, not WS
- Validate server certificate (not using `InsecureIgnoreHostKey` in production SSH)

### 3. Authentication
- API key in registration message
- Server should validate API key before accepting commands

### 4. Command Execution
- No command validation in gateway (trust server)
- Server should sanitize/validate commands before sending
- Consider implementing command whitelist

### 5. Network
- Gateway should run in trusted network
- Use firewall to restrict SSH access to gateway IP only
- Consider VPN for remote gateway deployments

---

## Performance Characteristics

### Latency
- WebSocket overhead: ~5-10ms
- SSH connection establishment: ~100-500ms (cached after first command)
- Command execution: Depends on command
- Total roundtrip: ~200ms-2s for most commands

### Throughput
- Limited by SSH serial execution
- One command at a time per node
- Could parallelize across nodes
- Send channel buffer: 256 messages

### Resource Usage
- Memory: ~10-20 MB base + command output buffers
- CPU: Minimal when idle, spikes during command execution
- Network: Persistent WebSocket + SSH connections
- Goroutines: ~5 base + 1 per concurrent command

---

## Extension Points

### Adding New Message Types

1. Add to `handleIncomingMessage()` switch statement
2. Create handler function
3. Define request/response structs
4. Implement logic

### Adding New Node Types

1. Update `config.json` schema
2. Modify `gatherNodeInfo()` for new OS detection
3. Add OS-specific command execution logic
4. Update `sshClient()` if auth changes needed

### Adding Metrics

1. Add Prometheus client library
2. Define metrics (counters, histograms, gauges)
3. Instrument key functions
4. Expose `/metrics` HTTP endpoint

### Adding Logging

1. Replace `log` with structured logger (e.g., zap, zerolog)
2. Add log levels
3. Add context fields (gateway_id, command_id, node_ip)
4. Add log aggregation integration

---

## Testing Strategy

### Unit Tests
- Test message parsing
- Test configuration loading
- Test credential resolution
- Mock WebSocket for client tests

### Integration Tests
- Test with example_server.go
- Test command execution on real nodes
- Test reconnection logic
- Test graceful shutdown

### Load Tests
- Multiple concurrent commands
- Sustained connection duration
- Reconnection under load
- Message buffer overflow

---

## Deployment Options

### Railway (Recommended)
- Auto-detect Go build
- Environment variables in dashboard
- Auto-restart on failure
- Built-in logging

### Docker
- Multi-stage build
- Alpine base image (~20 MB)
- Mount config or use env vars
- Health check endpoint recommended

### Systemd (Linux Server)
- Create `.service` file
- Auto-start on boot
- Log to journald
- Restart policy

### Windows Service
- Use tools like NSSM
- Run as background service
- Log to Event Log
- Auto-restart configuration

---

## Monitoring & Observability

### Key Metrics
- Connection status (up/down)
- Reconnection count
- Command execution rate
- Command success/failure rate
- Command execution duration
- SSH connection errors

### Logging
- Connection events
- Registration success/failure
- Incoming commands
- SSH execution results
- Errors and warnings

### Alerting
- Connection down for > 5 minutes
- High command failure rate
- SSH authentication failures
- Repeated reconnections

---

## Future Enhancements

1. **Command Queue** - Queue multiple commands per node
2. **Parallel Execution** - Execute on multiple nodes simultaneously
3. **Response Caching** - Cache results for idempotent commands
4. **Rate Limiting** - Limit command execution rate
5. **SSH Key Auth** - Support SSH key-based authentication
6. **Command Timeout** - Add configurable timeout per command
7. **Streaming Output** - Stream long-running command output
8. **File Transfer** - Support SCP/SFTP file operations
9. **Node Discovery** - Auto-discover nodes on network
10. **Web UI** - Built-in web interface for testing
