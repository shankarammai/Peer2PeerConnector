package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/lithammer/shortuuid"
	"github.com/shankarammai/Peer2PeerConnector/internal/client"
	responsemessage "github.com/shankarammai/Peer2PeerConnector/internal/response"
	"github.com/shankarammai/Peer2PeerConnector/internal/room"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,
	WriteBufferSize: 2048,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections by default
	},
}

var (
	clients = make(map[string]*client.Client)
	rooms   = make(map[string]*room.Room)
	mu      sync.Mutex
)

// HandleWebSocketConnection handles websocket connection
func HandleWebSocketConnection(writer http.ResponseWriter, request *http.Request) {
	if websocket.IsWebSocketUpgrade(request) {
		connection, error := upgrader.Upgrade(writer, request, nil)
		if error != nil {
			log.Println("Failed to upgrade connection")
			return
		}
		log.Printf("Connection from: %s \n", connection.RemoteAddr())

		// Client connected add to clients with new Id seperating all clients
		clientId := shortuuid.New()
		client := &client.Client{
			Id:         clientId,
			Connection: connection,
		}
		//Adding client to clients map.
		mu.Lock()
		clients[clientId] = client
		mu.Unlock()
		log.Println("Client Added : ", clientId)

		//need and closed the connection and clean up
		defer func() {
			log.Println("WebSocket connection closed by client :", clientId)
			removeClientFromRoom(clientId, true)
			err := connection.Close()
			if err != nil {
				log.Println("Failed to close WebSocket connection:", err)
			}
			log.Println("WebSocket connection closed for client :", clientId)
		}()

		// send the clientId back to client
		error = connection.WriteJSON(responsemessage.InfoMessage(
			"client_details",
			map[string]interface{}{"id": clientId},
		))
		if error != nil {
			log.Println("Write Json Error", error)
		}

		// Read messages from all the client and create go routines for them
		for {
			_, message, err := connection.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				break
			}
			// Handle all types of messages
			go handleMessage(client, message)
		}
	} else {
		http.ServeFile(writer, request, filepath.Join("public", "index.html"))
	}
}

// Function to remove a client from the map
func removeClient(clientID string) {
	mu.Lock()
	delete(clients, clientID)
	mu.Unlock()
	log.Printf("Client removed:  %s \n", clientID)
}

func handleMessage(client *client.Client, message []byte) {
	var json_msg map[string]interface{}
	parseErr := json.Unmarshal(message, &json_msg)
	if parseErr != nil {
		log.Println("Failed to parse JSON: ", message)
		return
	}

	log.Printf("Received message: %v \n", json_msg)
	switch json_msg["type"] {
	case "connect":
		handleConnectMessage(client, json_msg)
	case "create_room":
		handleCreateRoomMessage(client, json_msg)
	case "join_room":
		handleJoinRoomMessage(client, json_msg)
	case "leave_room":
		handleLeaveRoomMessage(client, json_msg)
	case "end_room":
		handleEndRoomMessage(client, json_msg)
	case "offer", "answer", "candidate", "message":
		relayMessageToTarget(client, json_msg)
	}
}

func handleConnectMessage(client *client.Client, message map[string]interface{}) {
	// check if message has target_id
	targetID, ok := message["to"].(string)
	if !ok {
		log.Println("Target ID not found in connect message.")
		return
	}

	// check if we have that target Id
	mu.Lock()
	targetClient, exists := clients[targetID]
	mu.Unlock()
	if !exists {
		log.Printf("Target client %s not found \n.", targetID)
		return
	}

	connectMsg := map[string]interface{}{
		"type":      "offer",
		"from":      client.Id,
		"sdp":       message["sdp"],
		"candidate": message["candidate"],
	}
	if err := targetClient.GetConnection().WriteJSON(responsemessage.InfoMessage("offer", connectMsg)); err != nil {
		log.Printf("Failed to send connect request to target client %s: %v \n.", targetID, err)
	}
}

func handleCreateRoomMessage(client *client.Client, msg map[string]interface{}) {
	roomId, exist := msg["room"].(string)
	if !exist {
		roomId = shortuuid.New()
	}

	from := client.GetClientId()

	//get the room name if exits, optional
	roomName, ok := msg["name"].(string)
	if !ok {
		roomName = ""
	}

	// check if room Id already exists
	// is it better to expose this id already exist or give new id?
	mu.Lock()
	myRoom, exists := rooms[roomId]
	mu.Unlock()
	if !exists {
		//if does not exist create one and add it
		mu.Lock()
		myRoom = room.NewRoom(roomId, roomName, from)
		rooms[roomId] = myRoom
		mu.Unlock()
		log.Println("Creating room with ID: ", roomId)
	} else {
		log.Println("Failed to create room (Already exists) ID: ", roomId)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("duplicate room", map[string]interface{}{"message": roomId + " already exist"}))
		return
	}

	// if we created room
	// now send all the client id in this room to all clients
	err := client.GetConnection().WriteJSON(responsemessage.InfoMessage("room_created", map[string]interface{}{"clients": myRoom.GetClients(), "roomId": roomId, "name": myRoom.GetName()}))
	if err != nil {
		log.Println("Failed to send all clients details to: ", client.Id)
	}

}

func handleEndRoomMessage(client *client.Client, msg map[string]interface{}) {
	if !checkRoomInJSON(client, msg) {
		return
	}
	from := client.GetClientId()
	roomId, _ := msg["room"].(string)
	room := rooms[roomId]

	if room.GetCreator() != from {
		log.Println("You don't have permissions to delete room: ", roomId)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("unauthorised", map[string]interface{}{"message": "You need to be creator of room to delete it."}))
		return
	}

	notifyUpdateIntheRoom(roomId, "room deleted")

	// after all the checks actually delete the room
	mu.Lock()
	delete(rooms, roomId)
	mu.Unlock()
	log.Println("Room Deleted: ", roomId)

}

// handleJoinRoomMessage handles joining room message
func handleJoinRoomMessage(client *client.Client, msg map[string]interface{}) {
	if !checkRoomInJSON(client, msg) {
		return
	}
	// room exist here
	roomId, _ := msg["room"].(string)
	// room should exist as well
	myRoom := rooms[roomId]
	from := client.GetClientId()

	// check client already in the room.
	if slices.Contains(myRoom.GetClients(), roomId) {
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Already exists", map[string]interface{}{"message": "Client already exists in the room."}))
		return
	} else {
		mu.Lock()
		myRoom.AddClient(from)
		mu.Unlock()
		// notify all clients in this room about the new clients in the room.
		notifyUpdateIntheRoom(roomId, "client added")
	}
}

func handleLeaveRoomMessage(client *client.Client, msg map[string]interface{}) {
	if !checkRoomInJSON(client, msg) {
		return
	}
	from := client.GetClientId()
	// room should exist here
	roomId, _ := msg["room"].(string)
	//check if client in room
	room := rooms[roomId]
	if slices.Contains(room.GetClients(), from) {
		removeClientFromRoom(from, false, roomId)
	} else {
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Client not found", map[string]interface{}{"message": "Client does not exists in the room."}))
	}
	log.Printf("%s left room %s \n", from, room)

	//if room is empty delete it.
	if len(room.GetClients()) == 0 {
		mu.Lock()
		delete(rooms, roomId)
		mu.Unlock()
		log.Printf("room %s deleted as it was empty \n", roomId)
	}

}

// checkRoomInJSON checks if 'room' exist in JSON.
func checkRoomInJSON(client *client.Client, msg map[string]interface{}) bool {
	// check if room exist
	roomId, ok := msg["room"].(string)
	if !ok {
		log.Println("You need room Id to join room.")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'room' field is missing in the request."}))
		return false
	}

	// check if room with given exists, if yes then add.
	_, exists := rooms[roomId]
	if !exists {
		log.Printf("Failed to join room: %s\n", roomId)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Invalid Room", map[string]interface{}{"message": "Room with Id " + roomId + " does not exist."}))
		return false
	}
	return true

}

// relayMessageToTarget forwards message to the 'to' a client when message received from one client
func relayMessageToTarget(client *client.Client, msg map[string]interface{}) {
	targetID, ok := msg["to"].(string)
	if !ok {
		log.Println("'to' not found in message.")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("missing fields", map[string]interface{}{"message": "'to' field not found"}))
		return
	}

	msgtype, ok2 := msg["type"].(string)
	if !ok2 {
		log.Println("'type' not found in message.")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("missing fields", map[string]interface{}{"message": "'type' field not found"}))
		return
	}

	mu.Lock()
	targetClient, exists := clients[targetID]
	mu.Unlock()
	if !exists {
		log.Printf("Target client %s not found. \n", targetID)
		return
	}

	switch msgtype {
	case "offer", "answer", "candidate", "message":
		delete(msg, "to")
		msg["from"] = client.GetClientId()
		if err := targetClient.GetConnection().WriteJSON(msg); err != nil {
			log.Printf("Failed to relay message to target client %s: %v \n", targetID, err)
		}
	default:
		log.Println("Unsupported message type: ", msg["type"])
	}
}

// notifyUpdateIntheRoom notifies update to all the client in the room.
func notifyUpdateIntheRoom(roomId string, message string) {
	room, ok := rooms[roomId]
	if !ok {
		log.Println("Room Id not found: ", roomId)
	}
	// notify all clients in this room about the update
	for _, clientIdItem := range room.GetClients() {
		clientInRoom, ok := clients[clientIdItem]
		if ok {
			clientInRoom.GetConnection().WriteJSON(responsemessage.UpdateMessage(message, map[string]interface{}{"clients": room.GetClients()}))
		}
	}
}

// removeClientFromRoom removes clients from room
func removeClientFromRoom(clientId string, deleteClient bool, roomIds ...string) (bool, error) {
	if len(roomIds) > 2 {
		return false, errors.New("invalid args passed, second argument should be roomId.")
	}
	if len(roomIds) == 1 {
		room, ok := rooms[roomIds[0]]
		if !ok {
			log.Println("Room Id not found: ", roomIds[0])
		}
		_, ok2 := clients[clientId]
		if !ok2 {
			log.Println("Client Id not found", clientId)
		}
		// first remove client from the room
		mu.Lock()
		room.RemoveClient(clientId)
		mu.Unlock()

		// notify all clients in this room about the update
		notifyUpdateIntheRoom(roomIds[0], "client removed")
	}
	// if we did not pass room Id we have to find from which room to delete
	// if client closed it's connection, we need to find of they are in room if yes delete
	if len(roomIds) == 0 && len(rooms) > 0 {
		log.Println("Searching and deleting client from room")
		for _, roomItem := range rooms {
			for _, clientInRoom := range roomItem.GetClients() {
				if clientInRoom == clientId {
					notifyUpdateIntheRoom(roomItem.GetId(), "client removed")
					mu.Lock()
					roomItem.RemoveClient(clientId)
					mu.Unlock()
					//delete room if clients empty
					if len(roomItem.GetClients()) == 0 {
						mu.Lock()
						delete(rooms, roomItem.GetId())
						mu.Unlock()
						log.Printf("Room %s deleted because it was empty", roomItem.GetId())
					}
					break
				}
			}

		}
	}
	if deleteClient {
		removeClient(clientId)
	}
	return true, nil
}
