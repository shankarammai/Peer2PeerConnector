package main

import (
	"log"
	"net/http"
	"runtime"

	"github.com/shankarammai/Peer2PeerConnector/internal/server"
)

func main() {
	log.Println("Starting Web Socket Server at port: 8080")
	http.HandleFunc("/", server.HandleWebSocketConnection)
	HandleErrorLine(http.ListenAndServe(":4416", nil))
}

func HandleErrorLine(err error) (b bool) {
	if err != nil {
		_, filename, line, _ := runtime.Caller(1)
		log.Printf("[error] %s:%d %v", filename, line, err)
		b = true
	}
	return
}
