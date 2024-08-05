package client

import (
	"github.com/gorilla/websocket"
)

type Client struct {
	Id         string
	Connection *websocket.Conn
}

func (client Client) GetClientId() string {
	return client.Id
}

func (client Client) GetConnection() *websocket.Conn {
	return client.Connection
}
