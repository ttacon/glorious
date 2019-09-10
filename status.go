package main

import (
	"os"
	"os/exec"
	"sync"

	"github.com/tevino/abool"
)

type Status struct {
	CurrentStatus UnitStatus
	Cmd           *exec.Cmd
	OutFile       *os.File
	CurrentSlot   *Slot

	shutdownRequested *abool.AtomicBool
	lock              *sync.Mutex
}

func (s Status) String() string {
	var status string
	switch s.CurrentStatus {
	case NotStarted:
		status = "not started"
	case Running:
		status = "running"
	case Stopped:
		status = "stopped"
	case Crashed:
		status = "crashed"
	}
	return status
}

func (s *Status) Lock() {
	s.lock.Lock()
}

func (s *Status) Unlock() {
	s.lock.Unlock()
}
