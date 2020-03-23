package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/sirupsen/logrus"
	"github.com/ttacon/glorious/agent/client"
	"github.com/ttacon/glorious/config"
	"github.com/ttacon/glorious/context"
)

var (
	configFileLocation = flag.String("config", "glorious.glorious", "config file location")
	contextName        = flag.String(
		"name",
		"",
		"Unique name to use to identify loaded state",
	)

	contex context.Context
)

func init() {
	contex = context.NewContext()

	flag.Parse()
	if len(*contextName) == 0 {
		*contextName = *configFileLocation
	}
}

func main() {
	client, err := client.NewClient("localhost:8222")
	if err != nil {
		fmt.Println("agent is not started, please start agent first, err: ", err)
		os.Exit(1)
	}

	conf, err := config.LoadConfig(*configFileLocation)
	if err != nil {
		fmt.Println("failed to load config: ", err)
		os.Exit(1)
	}

	name := *configFileLocation

	if isLoaded, err := client.IsLoaded(name); err != nil {
		fmt.Println("failed to check if config is loaded, err: ", err)
		os.Exit(1)
	} else if !isLoaded {
		if err := client.LoadConfig(name, conf); err != nil {
			fmt.Println("failed to load config, err: ", err)
			os.Exit(1)
		}
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
			desc, err := client.DescribeConfig(name)
			if err != nil {
				c.Println("failed to retrieve config, err:", err)
				return
			}

			c.Printf("%-20s| %-9s| Description\n", "Name", "# units")
			for _, unit := range desc.Units {
				c.Printf("%-20s| %-9d| %s\n",
					unit.Name,
					unit.NumSlots,
					unit.Description,
				)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "status",
		Help: "Display status information",
		Func: func(c *ishell.Context) {
			units, err := client.GetStatus(name)
			if err != nil {
				c.Println("failed to retrieve units in config, err: ", err)
				return
			}

			c.Printf("%-20s|%-20s| %-9s\n", "Name", "Groups", "Status")
			for _, unit := range units {
				c.Printf("%-20s|%-20s| %-9s\n",
					unit.Name,
					strings.Join(unit.Groups, ", "),
					unit.Status,
				)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "reload",
		Help: "Reload the glorious config",
		Func: func(c *ishell.Context) {
			conf, err = config.LoadConfig(*configFileLocation)
			if err != nil {
				fmt.Println("failed to reload config from disk: ", err)
				return
			} else if err := client.LoadConfig(name, conf); err != nil {
				fmt.Println("failed to load config, err: ", err)
				return
			}
			c.Printf("Reloaded glorious config from %s\n", *configFileLocation)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "Start a given unit",
		Func: func(c *ishell.Context) {
			startResponses, err := client.StartUnits(name, c.Args)
			if err != nil {
				c.Println("failed to start units, err: ", err)
				return
			}

			for _, resp := range startResponses {
				c.Printf("starting %q...%s\n", resp.UnitName, resp.RequestStatus)
			}
		},
	})

	lgr := logrus.New()

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
				val, err := contex.InternalStore().GetInternalStoreVal(key)
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

			if err := contex.InternalStore().PutInternalStoreVal(
				c.Args[0],
				c.Args[1],
			); err != nil {
				c.Println(err)
				return
			}

			if err := conf.AssertKeyChange(c.Args[0], lgr); err != nil {
				c.Println(err)
			}
		},
	})
	shell.AddCmd(storeCmd)

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "Stops given units",
		Func: func(c *ishell.Context) {
			unitsToStart, err := conf.GetUnits(c.Args)
			if err != nil {
				fmt.Println(err)
				return
			}

			for _, unit := range unitsToStart {
				c.Printf("stopping %q...", unit.Name)

				if err := unit.Stop(lgr); err != nil {
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
			unit, ok := conf.GetUnit(unitName)
			if !ok {
				c.Printf("unknown unit %q, aborting\n", unitName)
				return
			}

			// if there are no more args, run a following tail, if
			// there's a number, run a number sliced tail
			if err := unit.Tail(lgr); err != nil {
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
