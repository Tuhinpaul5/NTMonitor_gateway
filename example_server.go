//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for testing
	},
}

type Gateway struct {
	ID   string
	Conn *websocket.Conn
}

var gateways = make(map[string]*Gateway)

func main() {
	http.HandleFunc("/api/gateway/ws", handleGateway)
	http.HandleFunc("/", handleIndex)

	log.Println("Example dashboard server starting on :8080")
	log.Println("Open http://localhost:8080 to test")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>NTMonitor Dashboard - Test</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .gateway { border: 1px solid #ccc; padding: 10px; margin: 10px 0; }
        .online { background-color: #e8f5e9; }
        .offline { background-color: #ffebee; }
        input, button { padding: 8px; margin: 5px 0; }
        #log { background: #f5f5f5; padding: 10px; height: 300px; overflow-y: auto; font-family: monospace; font-size: 12px; }
    </style>
</head>
<body>
    <h1>NTMonitor Dashboard - Test Server</h1>
    <div id="gateways"></div>
    
    <h3>Send Command</h3>
    <input type="text" id="gatewayId" placeholder="Gateway ID" value="gateway-1">
    <input type="text" id="nodeIp" placeholder="Node IP" value="192.168.1.21">
    <input type="text" id="command" placeholder="Command" value="uptime">
    <button onclick="sendCommand()">Execute Command</button>
    
    <h3>Log</h3>
    <div id="log"></div>

    <script>
        const log = document.getElementById('log');
        const gatewaysDiv = document.getElementById('gateways');
        
        function addLog(msg) {
            const time = new Date().toLocaleTimeString();
            log.innerHTML += time + ' - ' + msg + '<br>';
            log.scrollTop = log.scrollHeight;
        }
        
        function updateGateways() {
            fetch('/api/gateways')
                .then(r => r.json())
                .then(data => {
                    gatewaysDiv.innerHTML = data.map(g => 
                        '<div class="gateway online">' +
                        '<strong>' + g.id + '</strong> - Connected' +
                        '</div>'
                    ).join('');
                });
        }
        
        function sendCommand() {
            const gatewayId = document.getElementById('gatewayId').value;
            const nodeIp = document.getElementById('nodeIp').value;
            const command = document.getElementById('command').value;
            
            fetch('/api/command', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({gateway_id: gatewayId, node_ip: nodeIp, command: command})
            })
            .then(r => r.json())
            .then(data => {
                addLog('Command sent: ' + JSON.stringify(data));
            });
        }
        
        // Poll for updates
        setInterval(updateGateways, 2000);
        updateGateways();
        
        addLog('Dashboard ready');
    </script>
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func handleGateway(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	log.Printf("New WebSocket connection from %s", r.RemoteAddr)

	// Wait for registration message
	_, data, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Failed to read registration: %v", err)
		conn.Close()
		return
	}

	var regMsg map[string]interface{}
	if err := json.Unmarshal(data, &regMsg); err != nil {
		log.Printf("Failed to parse registration: %v", err)
		conn.Close()
		return
	}

	gatewayID, ok := regMsg["gateway_id"].(string)
	if !ok || regMsg["type"] != "register" {
		log.Println("Invalid registration message")
		conn.Close()
		return
	}

	log.Printf("Gateway registered: %s", gatewayID)

	gateway := &Gateway{
		ID:   gatewayID,
		Conn: conn,
	}
	gateways[gatewayID] = gateway

	// Send a test command after 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		testCmd := map[string]interface{}{
			"type":       "execute_command",
			"command_id": "test-123",
			"node_ip":    "192.168.1.21",
			"command":    "echo 'Hello from dashboard!'",
		}
		data, _ := json.Marshal(testCmd)
		conn.WriteMessage(websocket.TextMessage, data)
		log.Printf("Sent test command to %s", gatewayID)
	}()

	// Read loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Gateway %s disconnected: %v", gatewayID, err)
			delete(gateways, gatewayID)
			return
		}

		log.Printf("Received from %s: %s", gatewayID, string(message))

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			if msg["type"] == "command_response" {
				log.Printf("Command response: success=%v, output=%v",
					msg["success"], msg["output"])
			}
		}
	}
}
