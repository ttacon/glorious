package main

import (
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"

	"github.com/ttacon/glorious/agent"
)

func runServer(agnt *agent.Agent) error {
	server := rpc.NewServer()
	server.Register(agnt)
	listener, e := net.Listen("tcp", ":7777")
	if e != nil {
		log.Fatal("listen error:", e)
	}
	for {
		if conn, err := listener.Accept(); err != nil {
			log.Fatal("accept error: " + err.Error())
		} else {
			log.Printf("new connection established\n")
			go server.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}
}
