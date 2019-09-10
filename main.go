package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/hashicorp/hcl"
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
	shell.SetHomeHistoryPath(".ishell_history/glorious")

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
		Name: "reload",
		Help: "Reload the glorious config",
		Func: func(c *ishell.Context) {
			config, err = loadConfig(*configFileLocation)
			if err != nil {
				fmt.Println("failed to reload config: ", err)
				return
			}
			c.Printf("Reloaded glorious config from %s\n", *configFileLocation)
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
	CurrentSlot   *Slot

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
