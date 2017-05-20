package main

import (
	"log"

	"container/list"

	"github.com/tidwall/gjson"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
)

type ClientConnectionServer struct {
	WebSocketServer websocket.Server
	HubConnections  *list.List
}

//New client connection server
func NewClientEndpoint(hubConnections *list.List) *ClientConnectionServer {
	server := ClientConnectionServer{}
	server.HubConnections = hubConnections

	server.WebSocketServer = websocket.New(websocket.Config{
		Endpoint:       "/connectClient",
		MaxMessageSize: 102400,
	})
	server.WebSocketServer.OnConnection(func(c websocket.Connection) {
		onClientConnect(c, server.HubConnections)
	})
	return &server
}

func onClientConnect(c websocket.Connection, hubConnections *list.List) {
	log.Println("New client connection", c.ID())
	newConnection := &ClientConnection{
		Connection: c,
		Mid:        1,
		Callbacks:  make(map[int64]RequestCallback)}

	c.OnMessage(func(messageBytes []byte) {
		message := string(messageBytes)
		messageJson := gjson.Parse(message)

		mid := gjson.Get(message, "mid").Int()

		callback := newConnection.Callbacks[mid]
		if callback != nil {
			callback(message)
			delete(newConnection.Callbacks, mid)
		}

		eventName := messageJson.Get("name").String()

		log.Println("Event: " + eventName)
		log.Println("Event: " + messageJson.String())
		if eventName == "RequestAuthorize" {
			token := messageJson.Get("payload.token").String()
			userInfo, err := GetUserInfo(token, getAuthData(AUTH_WEB))
			if err != nil {
				log.Println(err)
				return
			}
			if userInfo.Username == "" {
				log.Println("Connection not authorized")
				c.Disconnect()
				return
			}
			log.Println("New connection authorized for " + userInfo.Username)
		} else if eventName == "RequestGetDevices" {
			handleGetDeviceList(newConnection)
		}

	})

	c.OnDisconnect(func() {
		log.Println("Connection with ID: " + c.ID() + " has been disconnected!")
	})
}

func handleGetDeviceList(conn *ClientConnection) {

}
