package main

import (
	"fmt"
	"websocket/logutil"
	"websocket/ws"
)

var log = logutil.GetDefaultLogger()

func onRead(ws *ws.WsSock, data []byte) {
	fmt.Println(fmt.Sprintf("receive : %s", string(data)))
}

func main() {
	server := ws.NewServer()
	defer func() {
		if err := recover(); err != nil {
			server.StopServer()
		}
	}()
	server.SetOnRead(onRead)
	server.StartServer("127.0.0.1", 9607)
}
