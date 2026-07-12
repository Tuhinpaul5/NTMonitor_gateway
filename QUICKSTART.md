# Quick Start Guide

## Test Locally (Before Deploying to Railway)

### 1. Run the Example Dashboard Server

In one terminal:

```bash
go run example_server.go
```

This starts a test WebSocket server on `http://localhost:8080`

### 2. Configure Your Gateway

Create a `.env` file:

```bash
cat > .env << 'EOF'
WS_URL=ws://localhost:8080/api/gateway/ws
GATEWAY_ID=gateway-test-1
API_KEY=test-key
CONFIG_PATH=config.json
EOF
```

### 3. Set Node Passwords

```bash
export NODE_192_168_1_21_PASSWORD="your-linux-password"
export NODE_192_168_1_101_PASSWORD="your-windows-password"
```

### 4. Run the Gateway

In another terminal:

```bash
source .env
go run .
```

You should see:

```
2024/01/15 10:30:00 Loaded configuration with 2 nodes
2024/01/15 10:30:00 Starting gateway client: gateway-test-1 connecting to ws://localhost:8080/api/gateway/ws
2024/01/15 10:30:00 Attempting to connect to ws://localhost:8080/api/gateway/ws...
2024/01/15 10:30:00 WebSocket connected successfully
2024/01/15 10:30:00 Gateway registered successfully
```

### 5. Test Command Execution

The example server automatically sends a test command after 5 seconds. Watch both terminals for the execution.

Open `http://localhost:8080` in your browser to see the web interface and manually send commands.

---

## Deploy to Railway

### 1. Push to GitHub

```bash
git init
git add .
git commit -m "Initial commit: NTMonitor Gateway"
git branch -M main
git remote add origin https://github.com/yourusername/ntmonitor-gateway.git
git push -u origin main
```

### 2. Create Railway Project

1. Go to [railway.app](https://railway.app)
2. Click "New Project" → "Deploy from GitHub repo"
3. Select your repository
4. Railway will auto-detect Go and build

### 3. Configure Environment Variables

In Railway dashboard, add these variables:

```
WS_URL=wss://your-dashboard.com/api/gateway/ws
GATEWAY_ID=gateway-home-pc
API_KEY=your-secret-key
NODE_192_168_1_21_PASSWORD=linux-password
NODE_192_168_1_101_PASSWORD=windows-password
```

### 4. Deploy

Railway will automatically deploy. Check logs to verify connection:

```
Starting gateway client: gateway-home-pc connecting to wss://...
WebSocket connected successfully
Gateway registered successfully
```

---

## Testing SSH Commands

### From the Dashboard, Send:

```json
{
  "type": "execute_command",
  "command_id": "test-001",
  "node_ip": "192.168_1.21",
  "command": "uptime"
}
```

### Expected Response:

```json
{
  "type": "command_response",
  "command_id": "test-001",
  "success": true,
  "output": " 10:45:32 up 5 days,  2:34,  1 user,  load average: 0.15, 0.10, 0.08\n"
}
```

---

## Example Commands to Test

### Linux:
- `uptime` - System uptime
- `df -h` - Disk usage
- `free -h` - Memory usage
- `ps aux | head -n 10` - Top processes
- `cat /etc/os-release` - OS information

### Windows:
- `systeminfo` - System information
- `ipconfig` - Network configuration
- `tasklist` - Running processes
- `dir C:\` - Directory listing

---

## Troubleshooting

### Gateway won't connect
- Check `WS_URL` is correct
- Verify dashboard server is running
- Check firewall/network settings
- Use `ws://` for local, `wss://` for production

### SSH connection fails
- Verify node IP is reachable: `ping 192.168.1.21`
- Test SSH manually: `ssh username@192.168.1.21`
- Check password environment variables are set
- Ensure SSH server is running on target node

### Password not found
```bash
# Check if environment variable is set
echo $NODE_192_168_1_21_PASSWORD

# If empty, export it
export NODE_192_168_1_21_PASSWORD="your-password"
```

### Reconnection issues
Gateway automatically reconnects with exponential backoff. Check logs:
- Initial backoff: 1 second
- Max backoff: 5 minutes
- Will retry indefinitely until connected

---

## Security Checklist

- [ ] Use `wss://` (not `ws://`) in production
- [ ] Set strong `API_KEY`
- [ ] Never commit passwords to git
- [ ] Use environment variables for all credentials
- [ ] Consider SSH key authentication instead of passwords
- [ ] Run gateway in trusted network
- [ ] Implement command validation on dashboard
- [ ] Use firewall to restrict SSH access
- [ ] Monitor gateway logs for suspicious activity
- [ ] Rotate credentials regularly

---

## Next Steps

1. **Build Dashboard Server**: Create the WebSocket server that the gateway connects to
2. **Add Authentication**: Implement API key validation
3. **Command Queueing**: Add command queue for multiple requests
4. **Response Caching**: Cache command responses
5. **Rate Limiting**: Limit command execution rate
6. **Logging**: Add structured logging
7. **Metrics**: Add Prometheus metrics
8. **SSH Keys**: Support SSH key authentication
9. **Multi-Gateway**: Support multiple gateways
10. **Web UI**: Build a web interface for the dashboard
