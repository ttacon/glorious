package main

import (
	"flag"
	"fmt"
	"net/rpc/jsonrpc"
	"os"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/sirupsen/logrus"
	"github.com/ttacon/glorious/agent"
	"github.com/ttacon/glorious/config"
	"github.com/ttacon/glorious/context"
)

var (
	configFileLocation = flag.String("config", "glorious.glorious", "config file location")
	debugMode          = flag.Bool("debug", false, "run in debug mode")
	daemonMode         = flag.Bool("daemon", false, "run as daemon")

	contex context.Context
)

func init() {
	contex = context.NewContext()
}

func main() {
	flag.Parse()

	if *debugMode {
		contex.Logger().SetLevel(logrus.DebugLevel)
	}

	lgr := contex.Logger()

	lgr.Debug("loading config: ", *configFileLocation)
	conf, err := config.LoadConfig(*configFileLocation)
	if err != nil {
		lgr.Error("failed to load config: ", err)
		os.Exit(1)
	}
	if errs := conf.Validate(); len(errs) > 0 {
		lgr.Debug("validation errors detected:")
		for i, err := range errs {
			lgr.Debug("[err %d] %s\n", i, err)
		}
		os.Exit(1)
	}

	conf.SetContext(contex)
	if err := conf.Init(); err != nil {
		lgr.Error("failed to initialize config, err: ", err)
		os.Exit(1)
	}

	shell := ishell.New()
	shell.SetHomeHistoryPath(".glorious/history")

	// display welcome info.
	shell.Println(banner)

	agnt := agent.NewAgent(conf, *configFileLocation, lgr)

	if *daemonMode {
		if err := runServer(agnt); err != nil {
			lgr.Error(err)
			os.Exit(1)
		}
		return
	}

	client, err := jsonrpc.Dial("tcp", ":7777")
	if err != nil {
		lgr.Error(err)
		os.Exit(1)
	}

	// register a function for "greet" command.
	shell.AddCmd(&ishell.Cmd{
		Name: "greet",
		Help: "greet user",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			var greeting string
			if err := client.Call("Agent.Greet", c.Args, &greeting); err != nil {
				lgr.Error(err)
				return
			}

			c.Println(greeting)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "config",
		Help: "Display config information",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			var units []agent.UnitConfig
			if err := client.Call("Agent.Config", struct{}{}, &units); err != nil {
				lgr.Error(err)
				return
			}

			c.Printf("%-20s| %-9s| Description\n", "Name", "# units")
			for _, unit := range units {
				c.Printf("%-20s| %-9s| %s\n",
					unit.Name,
					fmt.Sprintf("%d units", unit.NumSlots),
					unit.Description,
				)
			}

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "status",
		Help: "Display status information",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			var units []agent.UnitStatus
			if err := client.Call("Agent.Status", struct{}{}, &units); err != nil {
				lgr.Error(err)
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
			lgr.Debug("command invoked: ", c.Cmd.Name)

			var respErr error
			if err := client.Call("Agent.Reload", struct{}{}, &respErr); err != nil {
				lgr.Error(err)
				return
			} else if respErr != nil {
				lgr.Error(err)
				return
			}
			c.Printf("Reloaded glorious config from %s\n", *configFileLocation)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "Start a given unit",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			for _, arg := range c.Args {
				c.Printf("starting %q...\n", arg)

				var respErr string
				if err := client.Call(
					"Agent.StartUnit",
					arg,
					&respErr,
				); err != nil {
					c.Println(err)
				} else if len(respErr) > 0 {
					c.Println(respErr)
				} else {
					c.Println("done")
				}
			}
		},
	})

	storeCmd := &ishell.Cmd{
		Name: "store",
		Help: "Access the glorious store",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			c.Println(c.Cmd.HelpText())
		},
	}
	storeCmd.AddCmd(&ishell.Cmd{
		Name: "get",
		Help: "Return value for given key",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			if len(c.Args) == 0 {
				c.Println("must provide key to retrieve value for")
				return
			}

			req := agent.StoreGetValuesRequest{
				Keys: c.Args,
			}
			var resp agent.StoreGetValuesResponse

			if err := client.Call(
				"Agent.StoreGetValues",
				&req,
				&resp,
			); err != nil {
				c.Println(err)
				return
			}

			for key, value := range resp.Values {
				c.Printf("%q: %q\n", key, value)
			}
		},
	})
	storeCmd.AddCmd(&ishell.Cmd{
		Name: "set",
		Help: "Stores the key/value pair in the internal store",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			if len(c.Args) != 2 {
				c.Println("May only provide a single key value pair")
				return
			}

			req := agent.StorePutValueRequest{
				Key:   c.Args[0],
				Value: c.Args[1],
			}
			var resp agent.ErrResponse

			if err := client.Call(
				"Agent.StorePutValue",
				&req,
				&resp,
			); err != nil {
				c.Println(err)
				return
			} else if len(resp.Err) > 0 {
				c.Println(resp.Err)
				return
			}
		},
	})
	shell.AddCmd(storeCmd)

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "Stops given units",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			for _, arg := range c.Args {
				c.Printf("stopping %q...", arg)

				var respErr string
				if err := client.Call(
					"Agent.StopUnit",
					arg,
					&respErr,
				); err != nil {
					c.Println(err)
				} else if len(respErr) > 0 {
					c.Println(respErr)
				} else {
					c.Println("done")
				}
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "tail",
		Help: "Tails a unit",
		Func: func(c *ishell.Context) {
			lgr.Debug("command invoked: ", c.Cmd.Name)

			// be cheaper if we can
			unitName := c.Args[0]
			unit, ok := conf.GetUnit(unitName)
			if !ok {
				c.Printf("unknown unit %q, aborting\n", unitName)
				return
			}

			// if there are no more args, run a following tail, if
			// there's a number, run a number sliced tail
			if err := unit.Tail(); err != nil {
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
