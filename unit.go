package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/abiosoft/ishell"
	"github.com/hpcloud/tail"
)

var (
	NOT_STARTED = "not started"
)

type Unit struct {
	Name        string `hcl:"name"`
	Description string `hcl:"description"`
	Slots       []Slot `hcl:"slot"`

	Status *Status
}

func (u *Unit) Start(ctxt *ishell.Context) error {
	// now for some tomfoolery
	slot, err := u.identifySlot()
	if err != nil {
		return err
	}

	return slot.Start(u, ctxt)
}

func (u *Unit) OutputFile() (*os.File, error) {
	home := os.Getenv("HOME")
	if len(home) == 0 {
		return nil, errors.New("cannot determine home directory")
	}

	outputDir := filepath.Join(home, ".glorious", "output")

	if err := os.MkdirAll(
		outputDir,
		0744,
	); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(
		filepath.Join(outputDir, u.Name),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	return f, err
}

func (u *Unit) ProcessStatus() string {
	if u.Status == nil {
		return NOT_STARTED
	}
	return u.Status.String()
}

func (u *Unit) Stop(ctxt *ishell.Context) error {
	if u.Status == nil {
		return ErrStopStopped
	}

	u.Status.shutdownRequested.Set()

	// TODO(ttacon): move to refactored function
	if u.Status.CurrentSlot.Provider.Type == "bash/local" {
		return u.Status.CurrentSlot.stopBash(u, ctxt, false)
	} else if u.Status.CurrentSlot.Provider.Type == "bash/remote" {
		return u.Status.CurrentSlot.stopBash(u, ctxt, true)
	} else if u.Status.CurrentSlot.Provider.Type == "docker/local" {
		return u.Status.CurrentSlot.stopDocker(u, ctxt, false)
	} else if u.Status.CurrentSlot.Provider.Type == "docker/remote" {
		return u.Status.CurrentSlot.stopDocker(u, ctxt, true)
	} else {
		return errors.New("unknown provider for unit, cannot stop, also probably wasn't started")
	}

	return nil
}

func (u *Unit) Tail(ctxt *ishell.Context) error {
	if u.ProcessStatus() == NOT_STARTED {
		return errors.New("cannot tail a stopped process")
	}

	t, err := tail.TailFile(u.Status.OutFile.Name(), tail.Config{Follow: true})
	if err != nil {
		return err
	}
	for line := range t.Lines {
		ctxt.Println(line.Text)
	}

	return nil
}

func (u *Unit) identifySlot() (*Slot, error) {
	// Short circuit if only a single slot exists.
	if len(u.Slots) == 0 {
		return nil, errors.New("unit has no slots")
	} else if len(u.Slots) == 1 {
		return &(u.Slots[0]), nil
	}

	// Always first identify the default slot
	var defaultSlot *Slot
	var resolverResults = make([]bool, len(u.Slots))
	for i, slot := range u.Slots {
		if slot.IsDefault() {
			if defaultSlot != nil {
				// Two slots are defined as default, result: barf.
				return nil, errors.New("there can only be one default slot")
			}
			defaultSlot = &slot
		}

		val, err := slot.Resolve(u)
		if err != nil {
			return nil, err
		}
		resolverResults[i] = val

	}

	// If no slot is defined as the default, grab the first one.
	defaultSlot = &(u.Slots[0])

	// Run through resolvers. Take the last resolved value if any.
	var resolvedSlot *Slot
	for i := len(resolverResults) - 1; i >= 0; i-- {
		if resolverResults[i] {
			resolvedSlot = &(u.Slots[i])
			break
		}
	}

	if resolvedSlot != nil {
		return resolvedSlot, nil
	}
	return defaultSlot, nil
}
