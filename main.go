package main

import (
	"log"
	"net/http"
	"runtime"

	"github.com/gorilla/websocket"
	"github.com/shankarammai/Peer2PeerConnector/internal/server"
)

func main() {
	log.Println("Starting Web Server at port: 8080")
	http.HandleFunc("/", handleRequest)
	HandleErrorLine(http.ListenAndServe(":8080", nil))
}

// handleWebRequest serves WebSocket on wss:// and Swagger docs on http://
func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Check if the request is using WebSocket
	if websocket.IsWebSocketUpgrade(r) {
		server.HandleWebSocketConnection(w, r)
	} else {
		server.ServerDocs(w, r)
	}
}

func HandleErrorLine(err error) (b bool) {
	if err != nil {
		_, filename, line, _ := runtime.Caller(1)
		log.Printf("[error] %s:%d %v", filename, line, err)
		b = true
	}
	return
}
