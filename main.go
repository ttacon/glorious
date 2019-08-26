package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/abiosoft/ishell"
)

func main() {

	config, err := loadConfig()
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
				c.Println("done")
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

func loadConfig() (*GloriousConfig, error) {
	data, err := ioutil.ReadFile("./glorious.toml")
	if err != nil {
		return nil, err
	}

	var m GloriousConfig
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

type GloriousConfig struct {
	Units []Unit `toml:"unit"`
}

func (g *GloriousConfig) GetUnit(name string) (*Unit, bool) {
	for _, unit := range g.Units {
		if unit.Name == name {
			return &unit, true
		}
	}
	return nil, false
}

type Unit struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Slots       []Slot `toml:"slot"`
}

type Slot struct {
	Cmd string `toml:"cmd"`
}
