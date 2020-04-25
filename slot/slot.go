package slot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rjeczalik/notify"
	gcontext "github.com/ttacon/glorious/context"
	gerrors "github.com/ttacon/glorious/errors"
	"github.com/ttacon/glorious/provider"
	"github.com/ttacon/glorious/status"
	"github.com/ttacon/glorious/store"
)

type Slot struct {
	Name     string             `hcl:"name"`
	Provider *provider.Provider `hcl:"provider"`
	Events   chan notify.EventInfo
	Resolver map[string]string `hcl:"resolver"`
}

type UnitInterface interface {
	GetName() string
	SetRunningStatus(*status.Status, status.StatusCallback)
	GetStatus() *status.Status
	OutputFile() (*os.File, error)
	UnsetCurrentSlot()
	SetCurrentSlot(*Slot)
	SavePIDFile(c *exec.Cmd) error
	InternalStore() *store.Store
	GetContext() gcontext.Context
}

func (s *Slot) Start(u UnitInterface) error {
	providerType := s.Provider.Type
	if len(providerType) == 0 {
		return errors.New("no provider given")
	}

	switch providerType {
	case "bash/local":
		return s.startBashLocal(u)
	case "bash/remote":
		return s.startBashRemote(u)
	case "docker/local":
		return s.startDockerLocal(u)
	case "docker/remote":
		return s.startDockerRemote(u)
	default:
		return errors.New("unknown provider")
	}
}

func (s *Slot) Stop(u UnitInterface) error {
	providerType := s.Provider.Type
	if len(providerType) == 0 {
		return errors.New("no provider given")
	}

	switch providerType {
	case "bash/local":
		return s.stopBash(u, false)
	case "bash/remote":
		return s.stopBash(u, true)
	case "docker/local":
		return s.stopDocker(u, false)
	case "docker/remote":
		return s.stopDocker(u, true)
	default:
		return errors.New("unknown provider for unit, cannot stop, also probably wasn't started")
	}
}

func (s Slot) IsDefault() bool {
	typ, ok := s.Resolver["type"]

	return ok && typ == "default"
}

func (s Slot) Resolve(u UnitInterface) (bool, error) {
	keyword := s.Resolver["keyword"]
	triggerValue := s.Resolver["value"]

	existingVal, err := u.InternalStore().GetInternalStoreVal(keyword)
	if err != nil {
		return false, err
	}

	return existingVal == triggerValue, nil
}

func (s *Slot) startDockerLocal(u UnitInterface) error {
	return s.startDockerInternal(u, false)
}

func (s *Slot) startDockerRemote(u UnitInterface) error {
	return s.startDockerInternal(u, true)
}

func (s *Slot) startDockerInternal(u UnitInterface, remote bool) error {

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

	lgr := u.GetContext().Logger()

	// first see if the image exists
	_, _, err = cli.ImageInspectWithRaw(ctx, image)
	if err != nil {
		if client.IsErrNotFound(err) {
			lgr.Info("\nimage not found locally, trying to pull...")
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

	if len(s.Provider.Volumes) > 0 {
		mounts := make([]mount.Mount, len(s.Provider.Volumes))
		for i, volume := range s.Provider.Volumes {
			dirs := strings.Split(volume, ":")
			mounts[i] = mount.Mount{
				Type:   mount.TypeBind,
				Source: dirs[0],
				Target: dirs[1],
			}
		}
		hostConfig.Mounts = mounts
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Env:   s.Provider.Environment,
	}, hostConfig, nil, u.GetName())
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

	u.SetCurrentSlot(s)
	u.SetRunningStatus(status.NewRunningStatus(nil, nil), nil)

	lgr.Info("begun as container ", resp.ID)

	return nil
}

func (s *Slot) startBashLocal(u UnitInterface) error {
	return s.startBashInternal(u, false)
}

func (s *Slot) startBashRemote(u UnitInterface) error {
	err := s.startBashInternal(u, true)
	if err != nil {
		return err
	}

	// Start a buffered channel
	s.Events = make(chan notify.EventInfo, 1)
	err = notify.Watch(fmt.Sprintf("%s/...", s.Provider.WorkingDir), s.Events, notify.All)
	if err != nil {
		return errors.New("cannot watch files for the provider")
	}

	lgr := u.GetContext().Logger()

	lgr.Info("started watcher...")
	go func() {
		for {
			select {
			case e := <-s.Events:
				err := s.ExecuteHandlers(e, u)
				if err != nil {
					lgr.Error(err)
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

func (s *Slot) startBashInternal(u UnitInterface, remote bool) error {
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

	// Purge the PID file to disk
	if err := u.SavePIDFile(&c); err != nil {
		// TODO(ttacon): we'll need to cleanup here
		return err
	}

	u.SetCurrentSlot(s)
	u.SetRunningStatus(status.NewRunningStatus(
		&c,
		outputFile,
	), func(stat *status.Status) {
		go func(stat *status.Status) {
			stat.WaitForCommandEnd()
		}(stat)
	})

	u.GetContext().Logger().Infof("begun as pid %d...\n", c.Process.Pid)

	return nil
}

func (s *Slot) ExecuteHandlers(e notify.EventInfo, u UnitInterface) error {
	for _, handler := range s.Provider.Handlers {
		var match bool
		var err error
		if handler.Match != "" {
			match, err = regexp.MatchString(handler.Match, e.Path())
		} else if handler.Exclude != "" {
			match, err = regexp.MatchString(handler.Exclude, e.Path())
			// Negate the result since we're excluding files matching this pattern
			match = !match
		}

		if err != nil {
			return err
		}
		if match == false {
			continue
		}

		switch handler.Type {
		case "rsync/remote":
			return s.RSync(e.Path(), u)
		case "execute/remote":
			c, err := s.BashCmd(handler.Cmd, true)
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
		default:
			return errors.New("unknown handler")
		}
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
	c := exec.Command("ssh", remoteHost, remoteCmd)

	return *c, nil
}

func (s *Slot) RSync(local string, u UnitInterface) error {
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

func (s *Slot) stopDocker(u UnitInterface, remote bool) error {
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

	if err := cli.ContainerStop(ctx, u.GetName(), nil); err != nil {
		return err
	}

	if err := cli.ContainerRemove(
		ctx,
		u.GetName(),
		types.ContainerRemoveOptions{},
	); err != nil {
		return err
	}

	stat := u.GetStatus()

	stat.Stop()
	u.UnsetCurrentSlot()

	return nil
}

func (s *Slot) stopBash(u UnitInterface, remote bool) error {
	stat := u.GetStatus()

	// It's possible to be beaten here by the goroutine that is
	// waiting on the process to exit, so safety belts!
	if stat.Cmd == nil {
		return nil
	}

	if err := stat.Cmd.Process.Kill(); err != nil {
		return err
	}

	if err := stat.OutFile.Close(); err != nil {
		return err
	}

	stat.Cmd.Stdout = nil
	stat.Cmd.Stderr = nil

	// Kill the remote watcher if this is a remote bash script
	if remote {
		notify.Stop(s.Events)
	}
	return nil
}

func (s *Slot) Validate() []*gerrors.ErrWithPath {
	var rawErrs = s.Provider.Validate()
	var errs = make([]*gerrors.ErrWithPath, len(rawErrs))
	for i, err := range rawErrs {
		errs[i] = &gerrors.ErrWithPath{
			Path: []string{
				"slot",
				s.Name,
				"provider",
			},
			Err: err,
		}
	}
	return errs
}
