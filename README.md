# NTMonitor Gateway

A WebSocket-based gateway client that connects to your monitoring dashboard and executes SSH commands on remote nodes.

## Features

- **Persistent WebSocket Connection**: Maintains a persistent connection to your dashboard with automatic reconnection
- **Exponential Backoff Reconnect**: Intelligent reconnection strategy with exponential backoff
- **Ping/Pong Heartbeat**: Keeps connection alive with automatic heartbeat mechanism
- **SSH Command Execution**: Execute commands on configured nodes via SSH
- **Multi-OS Support**: Works with both Linux and Windows nodes
- **Secure Configuration**: Credentials stored in environment variables, not in committed files

## Architecture

```
┌─────────────┐         WSS          ┌──────────────┐
│  Dashboard  │◄────────────────────►│   Gateway    │
│   Server    │                      │    Client    │
└─────────────┘                      └──────┬───────┘
                                            │
                                            │ SSH
                                            │
                          ┌─────────────────┼─────────────────┐
                          │                 │                 │
                     ┌────▼────┐       ┌────▼────┐      ┌────▼────┐
                     │  Node 1 │       │  Node 2 │      │  Node N │
                     │ (Linux) │       │(Windows)│      │  (...)  │
                     └─────────┘       └─────────┘      └─────────┘
```

## Setup

### 1. Install Dependencies

```bash
go mod download
```

### 2. Configure Environment Variables

Copy the example environment file:

```bash
cp .env.example .env
```

Edit `.env` with your settings:

```bash
# WebSocket Dashboard URL
WS_URL=wss://your-dashboard.railway.app/api/gateway/ws

# Unique Gateway Identifier
GATEWAY_ID=gateway-home-pc

# Optional API Key for authentication
API_KEY=your-secret-api-key-here

# Path to node configuration file
CONFIG_PATH=config.json
```

### 3. Configure Nodes

Edit `config.json` to add your nodes:

```json
{
  "nodes": [
    {
      "ip": "192.168.1.21",
      "username": "your-username",
      "password": "",
      "os": "linux"
    }
  ]
}
```

**Important**: Don't commit passwords to git! Instead, set them via environment variables:

```bash
export NODE_192_168_1_21_PASSWORD="your-password"
export NODE_192_168_1_101_PASSWORD="another-password"
```

The pattern is: `NODE_<IP_WITH_UNDERSCORES>_PASSWORD`

### 4. Run the Gateway

```bash
# Load environment variables and run
source .env
go run .
```

Or build and run:

```bash
go build -o ntmonitor_gateway
./ntmonitor_gateway
```

## Message Protocol

### Registration (Gateway → Server)

```json
{
  "type": "register",
  "gateway_id": "gateway-home-pc",
  "api_key": "optional-api-key"
}
```

### Execute Command (Server → Gateway)

```json
{
  "type": "execute_command",
  "command_id": "uuid-v4",
  "node_ip": "192.168.1.21",
  "command": "uptime"
}
```

### Command Response (Gateway → Server)

```json
{
  "type": "command_response",
  "command_id": "uuid-v4",
  "success": true,
  "output": "10:30:45 up 5 days...",
  "error": ""
}
```

## Deployment

### Railway

1. Create a new project on Railway
2. Connect your GitHub repository
3. Set environment variables in Railway dashboard:
   - `WS_URL`
   - `GATEWAY_ID`
   - `API_KEY`
   - Node passwords (e.g., `NODE_192_168_1_21_PASSWORD`)
4. Railway will automatically build and deploy

### Docker

```bash
docker build -t ntmonitor-gateway .
docker run -e WS_URL=wss://... -e GATEWAY_ID=gateway-1 ntmonitor-gateway
```

## Development

### Project Structure

```
.
├── main.go           # Main application entry point
├── ws_client.go      # WebSocket client with reconnect logic
├── config.json       # Node configuration (no passwords!)
├── .env.example      # Environment variable template
└── README.md         # This file
```

### Key Components

- **main.go**: 
  - Loads configuration
  - Creates WebSocket client
  - Handles incoming commands
  - Executes SSH commands on nodes

- **ws_client.go**:
  - WebSocket connection management
  - Registration with server
  - Ping/pong heartbeat
  - Automatic reconnection with exponential backoff
  - Message send/receive

## Security Considerations

1. **Credentials**: Never commit passwords to version control
2. **SSH Keys**: Consider using SSH key-based authentication instead of passwords
3. **TLS**: Always use WSS (not WS) in production
4. **API Keys**: Use strong, randomly generated API keys
5. **Network**: Run gateway in a trusted network environment
6. **Command Validation**: Dashboard should validate/sanitize commands before sending

## Troubleshooting

### Connection Issues

- Verify `WS_URL` is correct and accessible
- Check if dashboard server is running
- Ensure network allows WebSocket connections
- Check firewall rules

### SSH Issues

- Verify node IPs are correct and reachable
- Test SSH connection manually: `ssh user@ip`
- Ensure SSH server is running on nodes
- Check that credentials are correct

### Environment Variables Not Loading

```bash
# Check if variables are set
echo $WS_URL
echo $GATEWAY_ID

# Load from .env file
export $(cat .env | xargs)
```

## License

MIT
