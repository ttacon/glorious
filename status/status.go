package status

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

	shutdownRequested *abool.AtomicBool
	lock              *sync.Mutex
}

func NewRunningStatus(cmd *exec.Cmd, f *os.File) *Status {
	return &Status{
		CurrentStatus: Running,
		Cmd:           cmd,
		OutFile:       f,

		shutdownRequested: abool.New(),
		lock:              new(sync.Mutex),
	}
}

type StatusCallback func(*Status)

type UnitStatus int

const (
	NotStarted UnitStatus = iota
	Running
	Stopped
	Crashed
)

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

func (s *Status) Stop() {
	s.CurrentStatus = Stopped
	s.shutdownRequested.Set()
}

func (s *Status) WaitForCommandEnd() {
	if err := s.Cmd.Wait(); err != nil {
		s.CurrentStatus = Crashed
		s.Lock()
		s.Cmd = nil
		s.Unlock()
	}

	s.shutdownRequested.UnSet()
}

func (s *Status) MarkShutdownRequested() {
	s.shutdownRequested.Set()
}

func (s *Status) ClearShutdown() {
	s.shutdownRequested.UnSet()
}
