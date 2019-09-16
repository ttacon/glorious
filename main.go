package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/abiosoft/ishell"
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
	if errs := config.Validate(); len(errs) > 0 {
		fmt.Println("validation errors detected:")
		for i, err := range errs {
			fmt.Printf("[err %d] %s\n", i, err)
		}
		os.Exit(1)
	}

	shell := ishell.New()
	shell.SetHomeHistoryPath(".glorious/history")

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
			c.Printf("%-20s|%-20s| %-9s\n", "Name", "Groups", "Status")
			for _, unit := range config.Units {
				c.Printf("%-20s|%-20s| %-9s\n",
					unit.Name,
					strings.Join(unit.Groups, ", "),
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
			unitsToStart, err := config.GetUnits(c.Args)
			if err != nil {
				fmt.Println(err)
				return
			}

			for _, unit := range unitsToStart {
				c.Printf("starting %q...\n", unit.Name)

				if err := unit.Start(c); err != nil {
					fmt.Println(err)
				}

				c.Println("done")
			}
		},
	})

	storeCmd := &ishell.Cmd{
		Name: "store",
		Help: "Access the glorious store",
		Func: func(c *ishell.Context) {
			c.Println(c.Cmd.HelpText())
		},
	}
	storeCmd.AddCmd(&ishell.Cmd{
		Name: "get",
		Help: "Return value for given key",
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Println("must provide key to retrieve value for")
				return
			}

			for _, key := range c.Args {
				val, err := getInternalStoreVal(key)
				if err != nil {
					val = "(not found)"
				}
				c.Printf("%q: %q\n", key, val)
			}
		},
	})
	storeCmd.AddCmd(&ishell.Cmd{
		Name: "set",
		Help: "Stores the key/value pair in the internal store",
		Func: func(c *ishell.Context) {
			if len(c.Args) != 2 {
				c.Println("May only provide a single key value pair")
				return
			}

			if err := putInternalStoreVal(c.Args[0], c.Args[1]); err != nil {
				c.Println(err)
				return
			}

			if err := config.assertKeyChange(c.Args[0], c); err != nil {
				c.Println(err)
			}
		},
	})
	shell.AddCmd(storeCmd)

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "Stops given units",
		Func: func(c *ishell.Context) {
			unitsToStart, err := config.GetUnits(c.Args)
			if err != nil {
				fmt.Println(err)
				return
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
		Help: "Tails a unit",
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

const banner = `
       _            _
  __ _| | ___  _ __(_) ___  _   _ ___
 / _  | |/ _ \| '__| |/ _ \| | | / __|
| (_| | | (_) | |  | | (_) | |_| \__ \
 \__, |_|\___/|_|  |_|\___/ \__,_|___/
 |___/
`
