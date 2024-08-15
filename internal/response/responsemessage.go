package responsemessage

import (
	"time"

	"github.com/google/uuid"
)

// Define the WebSocketMessage struct
type Message struct {
	Type      string      `json:"type"`
	Event     string      `json:"event,omitempty"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	MessageID string      `json:"message_id"`
}

func NewMessage(messageType string, event string, data interface{}) Message {
	return Message{
		Type:      messageType,
		Data:      data,
		Event:     event,
		Timestamp: time.Now(),
		MessageID: uuid.New().String(), // Generate a new unique ID
	}
}

// Function to create an update message
func UpdateMessage(event string, data map[string]interface{}) Message {
	return NewMessage(
		"update",
		event,
		data,
	)
}

// Function to create an error message
func ErrorMessage(title string, data map[string]interface{}) Message {
	return NewMessage(
		"error",
		title,
		data,
	)
}

// Function to create an info message
func InfoMessage(event string, data map[string]interface{}) Message {
	return NewMessage(
		"info",
		event,
		data,
	)
}
