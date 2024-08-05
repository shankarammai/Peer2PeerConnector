package room

import (
	"golang.org/x/exp/slices"
)

type Room struct {
	Id      string
	Name    string
	Clients []string
	Creator string
}

func NewRoom(Id string, Name string, Creator string) *Room {
	return &Room{
		Id:      Id,
		Name:    Name,
		Creator: Creator,
		Clients: []string{Creator},
	}
}

func (room Room) GetId() string {
	return room.Id
}

func (room Room) GetCreator() string {
	return room.Creator
}

func (room Room) GetName() string {
	return room.Name
}

func (room *Room) SetName(name string) {
	room.Name = name
}

func (room Room) GetClients() []string {
	return room.Clients
}

func (room *Room) AddClient(clientId string) {
	room.Clients = append(room.Clients, clientId)
}

func (room *Room) RemoveClient(clientId string) []string {
	indexToRemove := slices.Index(room.Clients, clientId)
	if indexToRemove != -1 {
		room.Clients = slices.Delete(room.Clients, indexToRemove, indexToRemove+1)
	}
	return room.Clients
}
