package main

import (
	"bytes"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"

	"github.com/ttacon/glorious/agent"
	"github.com/ttacon/glorious/context"
	"github.com/ttacon/glorious/tailer"
)

func runServer(agnt *agent.Agent, addr string, lgr context.Logger) error {
	server := rpc.NewServer()
	server.Register(agnt)
	listener, e := net.Listen("tcp", addr)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	for {
		if conn, err := listener.Accept(); err != nil {
			lgr.Fatal("accept error: " + err.Error())
		} else {
			lgr.Infof("new connection established\n")
			handleNewConnection(
				conn,
				server,
				tailer.NewTailer(agnt),
				lgr,
			)
		}
	}
}

func handleNewConnection(
	conn net.Conn,
	server *rpc.Server,
	tailr tailer.Tailer,
	lgr context.Logger,
) {
	lgr.Debug("parsing magic cookie")
	var buf = make([]byte, 4)
	if n, err := conn.Read(buf); err != nil {
		lgr.Error("failed to read magic cookie from connection, closing, err: ", err)
		_ = conn.Close()
		return
	} else if n != 4 {
		lgr.Error("did not read entire cookie frame, exiting (read %d of 4 bytes)\n", n)
		_ = conn.Close()
		return
	}

	if bytes.Equal(buf, MAGIC_COOKIE_V1) {
		lgr.Debug("creating agent based rpc connection")
		go server.ServeCodec(jsonrpc.NewServerCodec(conn))
		return
	} else if bytes.Equal(buf, MAGIC_COOKIE_V1_LOGS) {
		lgr.Debug("creating stream based connection")
		// Stream logs back.
		go tailr.Handle(conn)
		return
	}

	lgr.Error("unknown MAGIC_COOKIE received, closing, got: ", string(buf))
	_ = conn.Close()
}
