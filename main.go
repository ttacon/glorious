package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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

func (u *Unit) Start(ctxt *ishell.Context) error {
	// now for some tomfoolery
	slot, err := u.identifySlot()
	if err != nil {
		return err
	}

	return slot.Start(u, ctxt)
}

func (s *Slot) Start(u *Unit, ctxt *ishell.Context) error {
	providerType := s.Provider.Type
	if len(providerType) == 0 {
		return errors.New("no provider given")
	}

	switch providerType {
	case "bash/local":
		return s.startBashLocal(u, ctxt)
	case "docker/local":
		return s.startDockerLocal(u, ctxt)
	default:
		return errors.New("unknown provider")
	}
}

func (s *Slot) startDockerLocal(u *Unit, ctxt *ishell.Context) error {
	image := s.Provider.Image
	if len(image) == 0 {
		return errors.New("no image provided")
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return err
	}

	// first see if the image exists
	_, _, err = cli.ImageInspectWithRaw(ctx, image)
	if err != nil {
		if client.IsErrNotFound(err) {
			ctxt.Println("\nimage not found locally, trying to pull...")
			_, pullErr := cli.ImagePull(
				ctx,
				image,
				types.ImagePullOptions{},
			)
			if pullErr != nil {
				return pullErr
			}
		} else {
			return err
		}
	}

	hostConfig := &container.HostConfig{}
	if len(s.Provider.Ports) > 0 {
		bindings := nat.PortMap{}
		for _, port := range s.Provider.Ports {
			vals, err := nat.ParsePortSpec(port)
			if err != nil {
				return err
			}
			for _, val := range vals {
				bindings[val.Port] = []nat.PortBinding{val.Binding}
			}
		}
		hostConfig.PortBindings = bindings
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
	}, hostConfig, nil, u.Name)
	if err != nil {
		return err
	}

	if err := cli.ContainerStart(
		ctx,
		resp.ID,
		types.ContainerStartOptions{},
	); err != nil {
		return err
	}

	u.Status = &Status{
		CurrentStatus: Running,
		CurrentSlot:   s,

		shutdownRequested: abool.New(),
	}
	u.Status.shutdownRequested.UnSet()

	ctxt.Println("begun as container ", resp.ID)

	return nil
}

func (s *Slot) startBashLocal(u *Unit, ctxt *ishell.Context) error {
	cmd := s.Provider.Cmd
	if len(cmd) == 0 {
		return errors.New("no `cmd` provided")
	}

	// we don't care if it's set or not
	workingDir := s.Provider.WorkingDir

	pieces := strings.Split(cmd, " ")
	c := exec.Cmd{}
	c.Dir = workingDir
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
		CurrentSlot:   s,

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

	// TODO(ttacon): move to refactored function
	if u.Status.CurrentSlot.Provider.Type == "bash/local" {

		if err := u.Status.Cmd.Process.Kill(); err != nil {
			return err
		}

		if err := u.Status.OutFile.Close(); err != nil {
			return err
		}

		u.Status.Cmd.Stdout = nil
		u.Status.Cmd.Stderr = nil
	} else if u.Status.CurrentSlot.Provider.Type == "docker/local" {
		ctx := context.Background()
		cli, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return err
		}

		if err := cli.ContainerStop(ctx, u.Name, nil); err != nil {
			return err
		}

		if err := cli.ContainerRemove(
			ctx,
			u.Name,
			types.ContainerRemoveOptions{},
		); err != nil {
			return err
		}

	} else {
		errors.New("unknown provider for unit, cannot stop, also probably wasn't started")
	}

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
}

type Slot struct {
	Provider *Provider         `hcl:"provider"`
	Resolver map[string]string `hcl:"resolver"`
}

type Provider struct {
	Type string `hcl:"type"`

	WorkingDir string `hcl:"workingDir"`
	Cmd        string `hcl:"dir"`

	Image string   `hcl:"image"`
	Ports []string `hcl:"ports"`
}

func (s Slot) IsDefault() bool {
	typ, ok := s.Resolver["type"]

	return ok && typ == "default"
}

func (s Slot) Resolve(u *Unit) (bool, error) {

	return false, nil
}
