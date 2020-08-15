package tailer

import (
	"net"
	"time"

	"github.com/ttacon/glorious/agent"
)

type Tailer interface {
	Handle(conn net.Conn)
}

type tailer struct {
	agnt *agent.Agent
}

func NewTailer(a *agent.Agent) Tailer {
	return &tailer{
		agnt: a,
	}
}

func (t *tailer) Handle(conn net.Conn) {
	closeConn := func() {
		_ = conn.Close()
	}

	lgr := t.agnt.Logger()
	// First off the wire will be the token session UUID
	var uuidBuf = make([]byte, 36)
	n, err := conn.Read(uuidBuf)
	if err != nil {
		lgr.Error("failed to read uuid buffer, err: ", err)
		closeConn()
		return
	} else if n != 36 {
		lgr.Error("failed to read all 16 bytes for uuid buffer, err: ", err)
		closeConn()
		return
	}

	token := string(uuidBuf)
	names, ok := t.agnt.ExchangeTailToken(token)
	if !ok {
		lgr.Error("tail token was not found, token: ", token)
		closeConn()
		return
	}

	closeChan := make(chan string, len(names))
	dataChan := make(chan []byte, len(names)*5)
	shutdownChan := make(chan struct{}, len(names))

	for _, name := range names {
		t.streamFor(name, dataChan, shutdownChan, closeChan)
	}

	// Read from dataChan

	// moar?
}

func (t *tailer) streamFor(
	name string,
	dataChan chan []byte,
	shutdownChan chan struct{},
	closeChan chan string,
) {
	//	stopFn, err := t.agnt.Config() // stopped here

	for {
		select {
		case <-shutdownChan:
			closeChan <- name
			return
		case <-time.After(time.Second):
			// Get data!!!!

		}
	}
}
