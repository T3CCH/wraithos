package api

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow requests without Origin header (non-browser clients)
		}
		// Validate that the Origin matches the request Host
		host := r.Host
		return origin == "http://"+host || origin == "https://"+host
	},
}

// handleComposeTerminal streams compose command output over a WebSocket.
// The client sends a JSON message with {"action": "deploy|stop|restart|pull|logs"}
// and receives streaming text output.
func (s *Server) handleComposeTerminal(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	type wsMessage struct {
		Action string `json:"action"`
	}

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket read error: %v", err)
			}
			return
		}

		ctx, cancel := context.WithCancel(r.Context())

		// Stream output line by line over the WebSocket
		outputHandler := func(line string) {
			if err := conn.WriteJSON(map[string]string{
				"type": "output",
				"data": line,
			}); err != nil {
				log.Printf("websocket write error: %v", err)
				cancel()
			}
		}

		var cmdErr error
		switch msg.Action {
		case "deploy":
			cmdErr = s.Compose.Deploy(ctx, outputHandler)
		case "stop":
			cmdErr = s.Compose.Stop(ctx, outputHandler)
		case "restart":
			cmdErr = s.Compose.Restart(ctx, outputHandler)
		case "pull":
			cmdErr = s.Compose.Pull(ctx, outputHandler)
		case "logs":
			logs, err := s.Compose.Logs(ctx, 100)
			if err != nil {
				cmdErr = err
			} else {
				conn.WriteJSON(map[string]string{
					"type": "output",
					"data": logs,
				})
			}
		default:
			conn.WriteJSON(map[string]string{
				"type":  "error",
				"data":  "unknown action: " + msg.Action,
			})
			cancel()
			continue
		}

		if cmdErr != nil {
			conn.WriteJSON(map[string]string{
				"type": "error",
				"data": cmdErr.Error(),
			})
		} else {
			conn.WriteJSON(map[string]string{
				"type": "done",
				"data": msg.Action + " completed",
			})
		}

		cancel()
	}
}
