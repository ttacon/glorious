package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rjeczalik/notify"
	"github.com/tevino/abool"
)

type Slot struct {
	Provider *Provider `hcl:"provider"`
	Events   chan notify.EventInfo
	Resolver map[string]string `hcl:"resolver"`
}

type Provider struct {
	Type string `hcl:"type"`

	WorkingDir string `hcl:"workingDir"`
	Cmd        string `hcl:"cmd"`

	Image string   `hcl:"image"`
	Ports []string `hcl:"ports"`

	Remote RemoteInfo `hcl:"remote"`
}

type RemoteInfo struct {
	Host         string `hcl:"host"`
	User         string `hcl:"user"`
	IdentityFile string `hcl:"identityFile"`
	WorkingDir   string `hcl:workingDir`
}

func (s *Slot) Start(u *Unit, ctxt *ishell.Context) error {
	providerType := s.Provider.Type
	if len(providerType) == 0 {
		return errors.New("no provider given")
	}

	switch providerType {
	case "bash/local":
		return s.startBashLocal(u, ctxt)
	case "bash/remote":
		return s.startBashRemote(u, ctxt)
	case "docker/local":
		return s.startDockerLocal(u, ctxt)
	case "docker/remote":
		return s.startDockerRemote(u, ctxt)
	default:
		return errors.New("unknown provider")
	}
}

func (s Slot) IsDefault() bool {
	typ, ok := s.Resolver["type"]

	return ok && typ == "default"
}

func (s Slot) Resolve(u *Unit) (bool, error) {

	return false, nil
}

func (s *Slot) startDockerLocal(u *Unit, ctxt *ishell.Context) error {
	return s.startDockerInternal(u, ctxt, false)
}

func (s *Slot) startDockerRemote(u *Unit, ctxt *ishell.Context) error {
	return s.startDockerInternal(u, ctxt, true)
}

func (s *Slot) startDockerInternal(u *Unit, ctxt *ishell.Context, remote bool) error {
	image := s.Provider.Image
	if len(image) == 0 {
		return errors.New("no image provided")
	}

	options := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if remote {
		options = append(options, client.WithHost(s.Provider.Remote.Host))
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(options...)
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
	return s.startBashInternal(u, ctxt, false)
}

func (s *Slot) startBashInternal(u *Unit, ctxt *ishell.Context, remote bool) error {
	cmd := s.Provider.Cmd
	if len(cmd) == 0 {
		return errors.New("no `cmd` provided")
	}

	c, err := s.BashCmd(cmd, remote)
	if err != nil {
		return err
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

	ctxt.Printf("begun as pid %d...\n", c.Process.Pid)

	return nil
}

func (s *Slot) startBashRemote(u *Unit, ctxt *ishell.Context) error {
	err := s.startBashInternal(u, ctxt, true)
	if err != nil {
		return err
	}

	// Start a buffered channel
	s.Events = make(chan notify.EventInfo, 1)
	err = notify.Watch(fmt.Sprintf("%s/...", s.Provider.WorkingDir), s.Events, notify.All)
	if err != nil {
		return errors.New("cannot watch files for the provider")
	}
	ctxt.Println("started watcher...")
	go func() {
		for {
			select {
			case e := <-s.Events:
				if strings.Contains(e.Path(), "node_modules") {
					continue
				}
				err := s.RSync(e.Path(), u)
				if err != nil {
					ctxt.Println(err)
				}
			}
		}
	}()

	err = s.RSync(s.Provider.WorkingDir, u)
	if err != nil {
		return err
	}

	return nil
}

func (s *Slot) BashCmd(cmd string, remote bool) (exec.Cmd, error) {
	pieces := strings.Split(cmd, " ")
	if remote == false {
		c := exec.Cmd{}
		c.Dir = s.Provider.WorkingDir
		c.Path = pieces[0]
		if len(pieces) > 1 {
			c.Args = pieces[1:]
		}

		return c, nil
	}

	remoteHost := fmt.Sprintf("%s@%s", s.Provider.Remote.User, s.Provider.Remote.Host)
	remoteCmd := fmt.Sprintf("cd %s; %s", s.Provider.Remote.WorkingDir, strings.Join(pieces, " "))
	fmt.Println(pieces)
	c := exec.Command("ssh", remoteHost, remoteCmd)

	return *c, nil
}

func (s *Slot) RSync(local string, u *Unit) error {
	remoteInfo := s.Provider.Remote
	remoteDir := remoteInfo.WorkingDir
	if local != s.Provider.WorkingDir {
		remoteDir = strings.Replace(local, s.Provider.WorkingDir, remoteDir, 1)
	}
	remote := fmt.Sprintf("%s@%s:%s", remoteInfo.User, remoteInfo.Host, remoteDir)
	rsync := exec.Command("rsync", "-avuzq", "--exclude", "**/node_modules/*", local, remote)

	outputFile, err := u.OutputFile()
	if err != nil {
		return err
	}

	rsync.Stdout = outputFile
	rsync.Stderr = outputFile

	err = rsync.Run()
	if err != nil {
		return err
	}
	return nil
}

var ErrStopStopped = errors.New("cannot stop stopped unit")

func (s *Slot) stopDocker(u *Unit, ctxt *ishell.Context, remote bool) error {
	options := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	if remote {
		options = append(options, client.WithHost(s.Provider.Remote.Host))
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(options...)
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
	return nil
}

func (s *Slot) stopBash(u *Unit, ctxt *ishell.Context, remote bool) error {

	if err := u.Status.Cmd.Process.Kill(); err != nil {
		return err
	}

	if err := u.Status.OutFile.Close(); err != nil {
		return err
	}

	u.Status.Cmd.Stdout = nil
	u.Status.Cmd.Stderr = nil

	// Kill the remote watcher if this is a remote bash script
	if remote {
		notify.Stop(s.Events)
	}
	return nil
}
