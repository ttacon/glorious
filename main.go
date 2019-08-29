package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/hashicorp/hcl"
	"github.com/hpcloud/tail"
	"github.com/tevino/abool"
)

var (
	configFileLocation = flag.String("config", "glorious.glorious", "config file location")
)

func main() {
	flag.Parse()

	config, err := loadConfig(*configFileLocation)
	if err != nil {
		fmt.Println("failed to load config: ", err)
		os.Exit(1)

	}

	shell := ishell.New()

	// display welcome info.
	shell.Println(banner)

	// register a function for "greet" command.
	shell.AddCmd(&ishell.Cmd{
		Name: "greet",
		Help: "greet user",
		Func: func(c *ishell.Context) {
			c.Println("Hello", strings.Join(c.Args, " "))
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "config",
		Help: "Display config information",
		Func: func(c *ishell.Context) {
			c.Printf("%-20s| %-9s| Description\n", "Name", "# units")
			for _, unit := range config.Units {
				c.Printf("%-20s| %-9s| %s\n",
					unit.Name,
					fmt.Sprintf("%d units", len(unit.Slots)),
					unit.Description,
				)
			}

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "status",
		Help: "Display status information",
		Func: func(c *ishell.Context) {
			c.Printf("%-20s| %-9s\n", "Name", "Status")
			for _, unit := range config.Units {
				c.Printf("%-20s| %-9s\n",
					unit.Name,
					unit.ProcessStatus(),
				)
			}

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "Start a given unit",
		Func: func(c *ishell.Context) {
			var unitsToStart []*Unit
			for _, unitName := range c.Args {
				unit, ok := config.GetUnit(unitName)
				if !ok {
					c.Printf("unknown unit %q, aborting\n", unitName)
					return
				}

				unitsToStart = append(unitsToStart, unit)
			}

			for _, unit := range unitsToStart {
				c.Printf("starting %q...", unit.Name)

				if err := unit.Start(c); err != nil {
					fmt.Println(err)
				}

				c.Println("done")
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "Stops given units",
		Func: func(c *ishell.Context) {
			var unitsToStart []*Unit
			for _, unitName := range c.Args {
				unit, ok := config.GetUnit(unitName)
				if !ok {
					c.Printf("unknown unit %q, aborting\n", unitName)
					return
				}

				unitsToStart = append(unitsToStart, unit)
			}

			for _, unit := range unitsToStart {
				c.Printf("stopping %q...", unit.Name)

				if err := unit.Stop(c); err != nil {
					fmt.Println(err)
				}

				c.Println("done")
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "tail",
		Help: "tails a unit",
		Func: func(c *ishell.Context) {
			// be cheaper if we can
			unitName := c.Args[0]
			unit, ok := config.GetUnit(unitName)
			if !ok {
				c.Printf("unknown unit %q, aborting\n", unitName)
				return
			}

			// if there are no more args, run a following tail, if
			// there's a number, run a number sliced tail
			if err := unit.Tail(c); err != nil {
				c.Println(err)
			}
		},
	})

	// run shell
	shell.Run()
}

const banner = `       _            _                 
  __ _| | ___  _ __(_) ___  _   _ ___ 
 / _  | |/ _ \| '__| |/ _ \| | | / __|
| (_| | | (_) | |  | | (_) | |_| \__ \
 \__, |_|\___/|_|  |_|\___/ \__,_|___/
 |___/
`

func loadConfig(configFileLocation string) (*GloriousConfig, error) {
	data, err := ioutil.ReadFile(configFileLocation)
	if err != nil {
		return nil, err
	}

	var m GloriousConfig
	if err := hcl.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

type GloriousConfig struct {
	Units []*Unit `hcl:"unit"`
}

func (g *GloriousConfig) GetUnit(name string) (*Unit, bool) {
	for _, unit := range g.Units {
		if unit.Name == name {
			return unit, true
		}
	}
	return nil, false
}

type Unit struct {
	Name        string `hcl:"name"`
	Description string `hcl:"description"`
	Slots       []Slot `hcl:"slot"`

	Status *Status
}

type UnitStatus int

const (
	NotStarted UnitStatus = iota
	Running
	Stopped
	Crashed
)

type Status struct {
	CurrentStatus UnitStatus
	Cmd           *exec.Cmd
	OutFile       *os.File

	shutdownRequested *abool.AtomicBool
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

func (u *Unit) ProcessStatus() string {
	if u.Status == nil {
		return "not started"
	}
	return u.Status.String()
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

func (u *Unit) Start(ctxt *ishell.Context) error {
	// now for some tomfoolery
	slot := u.Slots[0]
	pieces := strings.Split(slot.Cmd, " ")
	c := exec.Cmd{}
	c.Dir = slot.WorkingDir
	c.Path = pieces[0]
	if len(pieces) > 1 {
		c.Args = pieces[1:]
	}

	outputFile, err := u.OutputFile()
	if err != nil {
		return err
	}

	c.Stdout = outputFile
	c.Stderr = outputFile

	if err := c.Start(); err != nil {
		return err
	}

	u.Status = &Status{
		CurrentStatus: Running,
		Cmd:           &c,
		OutFile:       outputFile,

		shutdownRequested: abool.New(),
	}
	u.Status.shutdownRequested.UnSet()

	go func(u *Unit) {
		if err := u.Status.Cmd.Wait(); err != nil {
			u.Status.CurrentStatus = Crashed
			u.Status.Cmd = nil
		}

		u.Status.shutdownRequested.UnSet()
	}(u)

	ctxt.Printf("begun as pid %d...", c.Process.Pid)
	return nil
}

var ErrStopStopped = errors.New("cannot stop stopped unit")

func (u *Unit) Stop(ctxt *ishell.Context) error {
	if u.Status == nil {
		return ErrStopStopped
	}

	u.Status.shutdownRequested.Set()

	if err := u.Status.Cmd.Process.Kill(); err != nil {
		return err
	}

	if err := u.Status.OutFile.Close(); err != nil {
		return err
	}

	u.Status.Cmd.Stdout = nil
	u.Status.Cmd.Stderr = nil

	return nil
}

func (u *Unit) Tail(ctxt *ishell.Context) error {

	t, err := tail.TailFile(u.Status.OutFile.Name(), tail.Config{Follow: true})
	if err != nil {
		return err
	}
	for line := range t.Lines {
		ctxt.Println(line.Text)
	}

	return nil

	/*
		// let's be cheeky for now and smarter later (implementing `tail`)
		data, err := exec.Command("tail", "-100", u.Status.OutFile.Name()).Output()
		if err != nil {
			return err
		}
		ctxt.Println(string(data))
		return nil
	*/
}

type Slot struct {
	Cmd        string `hcl:"cmd"`
	WorkingDir string `hcl:"workingDir"`
}
