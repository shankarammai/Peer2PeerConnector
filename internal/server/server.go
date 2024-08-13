package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"os"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/lithammer/shortuuid"
	"github.com/shankarammai/Peer2PeerConnector/internal/client"
	responsemessage "github.com/shankarammai/Peer2PeerConnector/internal/response"
	"github.com/shankarammai/Peer2PeerConnector/internal/room"
	"github.com/sirupsen/logrus"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting"
	"github.com/yuin/goldmark/renderer/html"
)

var logger = &logrus.Logger{
	Out:   os.Stderr,
	Level: logrus.DebugLevel,
	Formatter: &logrus.TextFormatter{
		DisableColors:   false,
		TimestampFormat: "2024-01-02 15:04:05",
		FullTimestamp:   true,
		ForceColors:     true,
	},
}

const (
	MsgTypeConnect    = "connect"
	MsgTypeCreateRoom = "create_room"
	MsgTypeJoinRoom   = "join_room"
	MsgTypeLeaveRoom  = "leave_room"
	MsgTypeEndRoom    = "end_room"
	MsgTypeOffer      = "offer"
	MsgTypeAnswer     = "answer"
	MsgTypeCandidate  = "candidate"
	MsgTypeMessage    = "message"
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

// ServerDocs serves the Markdown documentation as an HTML page.
// It reads the Markdown file located at "docs/docs.md", converts it to HTML using Goldmark,
// and then renders it using an HTML template located at "public/index.html".
func ServerDocs(writer http.ResponseWriter, request *http.Request) {
	// Load the Markdown file
	mdFile := "docs/docs.md"
	mdContent, err := os.ReadFile(mdFile)
	if err != nil {
		http.Error(writer, "Could not read Markdown file", http.StatusInternalServerError)
		return
	}

	// Convert Markdown to HTML using Goldmark
	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
		goldmark.WithExtensions(
			highlighting.NewHighlighting(
				highlighting.WithStyle("monokai"), // Change style as needed
				highlighting.WithFormatOptions(),
			),
		),
	)

	if err := md.Convert(mdContent, &buf); err != nil {
		http.Error(writer, "Could not convert Markdown to HTML", http.StatusInternalServerError)
		return
	}

	// Load and parse the HTML template
	tmpl, err := template.ParseFiles("public/index.html")
	if err != nil {
		http.Error(writer, "Could not parse template", http.StatusInternalServerError)
		return
	}

	// Execute the template with the HTML content
	data := struct {
		Content template.HTML
	}{
		Content: template.HTML(buf.String()), // Safely inject the HTML content
	}

	if err := tmpl.Execute(writer, data); err != nil {
		http.Error(writer, "Could not execute template", http.StatusInternalServerError)
		return
	}
}

// HandleWebSocketConnection handles WebSocket connections.
// It upgrades the HTTP connection to a WebSocket, assigns a unique client ID,
// and starts reading messages from the client. It also handles client disconnection
// and cleans up resources.
func HandleWebSocketConnection(writer http.ResponseWriter, request *http.Request) {
	connection, error := upgrader.Upgrade(writer, request, nil)
	if error != nil {
		logger.Error("Failed to upgrade connection")
		return
	}
	logger.Infof("Connection from: %s \n", connection.RemoteAddr())

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
	logger.Info("Client Added : ", clientId)

	//need and closed the connection and clean up
	defer func() {
		removeClientFromRoom(clientId, true)
		err := connection.Close()
		if err != nil {
			logger.Error("Failed to close WebSocket connection:", err)
		}
		logger.Info("WebSocket connection closed for client :", clientId)
	}()

	// send the clientId back to client
	error = connection.WriteJSON(responsemessage.InfoMessage(
		"client_details",
		map[string]interface{}{"id": clientId},
	))
	if error != nil {
		logger.Error("Write Json Error", error)
	}

	// Read messages from all the client and create go routines for them
	for {
		_, message, err := connection.ReadMessage()
		if err != nil {
			logger.Error("Read error:", err)
			break
		}
		// Handle all types of messages
		go handleMessage(client, message)
	}
}

// removeClient removes a client from the clients map by its client ID.
// It locks the mutex to ensure thread-safe access to the clients map
// and logs the removal of the client.
func removeClient(clientID string) {
	mu.Lock()
	delete(clients, clientID)
	mu.Unlock()
	logger.Info("Client removed:  %s \n", clientID)
}

// handleMessage processes incoming messages from clients based on their type.
// It routes the messages to appropriate handlers for connection, room management, and relaying messages.
func handleMessage(client *client.Client, message []byte) {
	var json_msg map[string]interface{}
	parseErr := json.Unmarshal(message, &json_msg)
	if parseErr != nil {
		logger.Error("Failed to parse JSON: ", message)
		return
	}

	switch json_msg["type"] {
	case MsgTypeConnect:
		handleConnectMessage(client, json_msg)
	case MsgTypeCreateRoom:
		handleCreateRoomMessage(client, json_msg)
	case MsgTypeJoinRoom:
		handleJoinRoomMessage(client, json_msg)
	case MsgTypeLeaveRoom:
		handleLeaveRoomMessage(client, json_msg)
	case MsgTypeEndRoom:
		handleEndRoomMessage(client, json_msg)
	case MsgTypeOffer, MsgTypeAnswer, MsgTypeCandidate, MsgTypeMessage:
		relayMessageToTarget(client, json_msg)
	}
}

// handleConnectMessage processes a "connect" message.
// It checks if the target client exists, validates required fields,
// and sends a connection offer to the target client.
func handleConnectMessage(client *client.Client, message map[string]interface{}) {
	// check if message has target_id
	targetID, ok := message["to"].(string)
	if !ok {
		logger.Debug("Target ID not found in connect message.")
		return
	}

	// check if we have that target Id
	mu.Lock()
	targetClient, exists := clients[targetID]
	mu.Unlock()
	if !exists {
		logger.Debugf("Target client %s not found \n.", targetID)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("client missing", map[string]interface{}{"message": "Client with given " + targetID + " not found"}))
		return
	}

	// Check if "data" exists and is a map
	data, ok := message["data"].(map[string]interface{})
	if !ok {
		logger.Debugf("'data' field is missing or not a map")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'data' field is missing or is not object in the request."}))
		return
	}

	// Check if "sdp" exists
	sdp, sdpExists := data["sdp"]
	if !sdpExists {
		logger.Debug("'data''sdp' field is missing or nil")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'data''sdp' field is missing in the request."}))
		return
	}

	// Check if "candidate" exists
	candidate, candidateExists := data["candidate"]
	if !candidateExists {
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'data''sdp' field is missing in the request."}))
		logger.Debug("'data''candidate' field is missing or nil")
		return
	}

	connectMsg := map[string]interface{}{
		"type": "offer",
		"from": client.Id,
		"data": map[string]interface{}{
			"sdp":       sdp,
			"candidate": candidate,
		},
	}
	if err := targetClient.GetConnection().WriteJSON(responsemessage.InfoMessage("offer", connectMsg)); err != nil {
		logger.Debugf("Failed to send connect request to target client %s: %v \n.", targetID, err)
	}
}

// handleCreateRoomMessage processes a "create_room" message.
// It creates a new room if it doesn't already exist, adds the room to the rooms map,
// and notifies the client about the room creation.
func handleCreateRoomMessage(client *client.Client, msg map[string]interface{}) {

	// Check if "data" exists and is a map
	data, dataOk := msg["data"].(map[string]interface{})
	if !dataOk {
		logger.Debug("'data' field is missing or not a map")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'data' field is missing or is not object in the request."}))
		return
	}

	roomId, exist := data["room"].(string)
	if !exist {
		roomId = shortuuid.New()
	}

	from := client.GetClientId()

	//get the room name if exits, optional
	roomName, ok := data["name"].(string)
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
		logger.Info("Creating room with ID: ", roomId)
	} else {
		logger.Debug("Failed to create room (Already exists) ID: ", roomId)
		client.GetConnection().WriteJSON(
			responsemessage.ErrorMessage(
				" duplicate room", map[string]interface{}{"message": roomId + " already exist"}))
		return
	}

	// if we created room
	// now send all the client id in this room to all clients
	err := client.GetConnection().WriteJSON(responsemessage.InfoMessage("room_created", map[string]interface{}{"clients": myRoom.GetClients(), "room": roomId, "name": myRoom.GetName()}))
	if err != nil {
		logger.Debug("Failed to send all clients details to: ", client.Id)
	}

}

// handleEndRoomMessage processes an "end_room" message.
// It verifies the client's permission to delete the room, sends a notification to
// all clients in the room, and removes the room from the rooms map if it is empty.
func handleEndRoomMessage(client *client.Client, msg map[string]interface{}) {
	if !checkRoomInJSON(client, msg) {
		return
	}
	data := msg["data"].(map[string]interface{})
	from := client.GetClientId()
	roomId, _ := data["room"].(string)
	room := rooms[roomId]

	if room.GetCreator() != from {
		logger.Debug("You don't have permissions to delete room: ", roomId)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("unauthorised", map[string]interface{}{"message": "You need to be creator of room to delete it."}))
		return
	}

	notifyUpdateIntheRoom(roomId, "room_deleted")

	// after all the checks actually delete the room
	mu.Lock()
	delete(rooms, roomId)
	mu.Unlock()
	logger.Info("Room Deleted: ", roomId)

}

// handleJoinRoomMessage processes a "join_room" message.
// It checks if the room exists, verifies that the client is not already in the room,
// adds the client to the room, and notifies all clients in the room about the new client.
func handleJoinRoomMessage(client *client.Client, msg map[string]interface{}) {
	if !checkRoomInJSON(client, msg) {
		return
	}
	data := msg["data"].(map[string]interface{})
	// room exist here
	roomId, _ := data["room"].(string)
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
		notifyUpdateIntheRoom(roomId, "client_added")
	}
}

// handleLeaveRoomMessage processes a "leave_room" message.
// It verifies that the client is in the room, removes the client from the room,
// and deletes the room if it is empty. It also sends a notification to all clients in the room.
func handleLeaveRoomMessage(client *client.Client, msg map[string]interface{}) {
	if !checkRoomInJSON(client, msg) {
		return
	}
	data := msg["data"].(map[string]interface{})
	from := client.GetClientId()
	// room should exist here
	roomId, _ := data["room"].(string)
	//check if client in room
	room := rooms[roomId]
	if slices.Contains(room.GetClients(), from) {
		removeClientFromRoom(from, false, roomId)
		client.GetConnection().WriteJSON(responsemessage.InfoMessage("room_left", map[string]interface{}{"room": roomId}))
	} else {
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Client not found", map[string]interface{}{"message": "Client does not exists in the room."}))
	}
	logger.Infof("%s left room %s \n", from, room.GetId())

	//if room is empty delete it.
	if len(room.GetClients()) == 0 {
		mu.Lock()
		delete(rooms, roomId)
		mu.Unlock()
		logger.Infof("room %s deleted as it was empty \n", roomId)
	}
}

// checkRoomInJSON checks if the room ID exists in the message JSON.
// It validates that the "data" field contains a valid room ID and checks if the room exists.
// Returns true if the room is valid, false otherwise.
func checkRoomInJSON(client *client.Client, msg map[string]interface{}) bool {
	// Check if "data" exists and is a map
	data, dataOk := msg["data"].(map[string]interface{})
	if !dataOk {
		logger.Debug("'data' field is missing or not a map")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'data' field is missing or is not object in the request."}))
		return false
	}
	// check if room exist
	roomId, ok := data["room"].(string)
	if !ok {
		logger.Debug("You need room Id to join room.")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Missing fields", map[string]interface{}{"message": "'room' field is missing in the request."}))
		return false
	}

	// check if room with given exists, if yes then add.
	_, exists := rooms[roomId]
	if !exists {
		logger.Debugf("Room does not exist: %s\n", roomId)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("Invalid Room", map[string]interface{}{"message": "Room with Id " + roomId + " does not exist."}))
		return false
	}
	return true

}

// relayMessageToTarget forwards a message to the target client specified in the message.
// It ensures that the target client exists and relays the message, handling various types of messages.
func relayMessageToTarget(client *client.Client, msg map[string]interface{}) {
	targetID, ok := msg["to"].(string)
	if !ok {
		logger.Debug("'to' not found in message.")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("missing fields", map[string]interface{}{"message": "'to' field not found"}))
		return
	}

	msgtype, ok2 := msg["type"].(string)
	if !ok2 {
		logger.Debug("'type' not found in message.")
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("missing fields", map[string]interface{}{"message": "'type' field not found"}))
		return
	}

	mu.Lock()
	targetClient, exists := clients[targetID]
	mu.Unlock()
	if !exists {
		logger.Debugf("Target client %s not found. \n", targetID)
		client.GetConnection().WriteJSON(responsemessage.ErrorMessage("client missing", map[string]interface{}{"message": "Client with given " + targetID + " not found"}))

		return
	}

	switch msgtype {
	case "offer", "answer", "candidate", "message":
		delete(msg, "to")
		msg["from"] = client.GetClientId()
		if err := targetClient.GetConnection().WriteJSON(msg); err != nil {
			logger.Debugf("Failed to relay message to target client %s: %v \n", targetID, err)
		}
	default:
		logger.Debug("Unsupported message type: ", msg["type"])
	}
}

// notifyUpdateIntheRoom sends an update notification to all clients in the specified room.
// It informs clients about changes such as client addition or removal.
func notifyUpdateIntheRoom(roomId string, message string) {
	room, ok := rooms[roomId]
	if !ok {
		logger.Debug("Room Id not found: ", roomId)
	}
	// notify all clients in this room about the update
	for _, clientIdItem := range room.GetClients() {
		clientInRoom, ok := clients[clientIdItem]
		if ok {
			clientInRoom.GetConnection().WriteJSON(responsemessage.UpdateMessage(
				message,
				map[string]interface{}{"clients": room.GetClients(), "room": room.GetId(), "name": room.GetName()}))
		}
	}
}

// removeClientFromRoom removes a client from a specified room or all rooms if no room ID is provided.
// It handles client removal from rooms and optionally removes the client itself if specified.
// If the client is removed from a room and the room becomes empty, the room is deleted.
func removeClientFromRoom(clientId string, deleteClient bool, roomIds ...string) (bool, error) {
	if len(roomIds) > 2 {
		return false, errors.New("invalid args passed, second argument should be roomId.")
	}
	if len(roomIds) == 1 {
		room, ok := rooms[roomIds[0]]
		if !ok {
			logger.Debug("Room Id not found: ", roomIds[0])
		}
		_, ok2 := clients[clientId]
		if !ok2 {
			logger.Debug("Client Id not found", clientId)
		}
		// first remove client from the room
		mu.Lock()
		room.RemoveClient(clientId)
		mu.Unlock()

		// notify all clients in this room about the update
		notifyUpdateIntheRoom(roomIds[0], "client_removed")
	}
	// if we did not pass room Id we have to find from which room to delete
	// if client closed it's connection, we need to find of they are in room if yes delete
	if len(roomIds) == 0 && len(rooms) > 0 {
		logger.Debug("Searching and deleting client from room")
		for _, roomItem := range rooms {
			for _, clientInRoom := range roomItem.GetClients() {
				if clientInRoom == clientId {
					notifyUpdateIntheRoom(roomItem.GetId(), "client_removed")
					mu.Lock()
					roomItem.RemoveClient(clientId)
					mu.Unlock()
					//delete room if clients empty
					if len(roomItem.GetClients()) == 0 {
						mu.Lock()
						delete(rooms, roomItem.GetId())
						mu.Unlock()
						logger.Infof("Room %s deleted because it was empty", roomItem.GetId())
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
