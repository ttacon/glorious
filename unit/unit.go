package unit

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/docker/docker/client"
	"github.com/hpcloud/tail"
	gcontext "github.com/ttacon/glorious/context"
	gerrors "github.com/ttacon/glorious/errors"
	"github.com/ttacon/glorious/slot"
	"github.com/ttacon/glorious/status"
	"github.com/ttacon/glorious/store"
)

var (
	NOT_STARTED = "not started"
)

type Unit struct {
	Name        string      `hcl:"name"`
	Description string      `hcl:"description"`
	Groups      []string    `hcl:"groups"`
	Slots       []slot.Slot `hcl:"slot"`

	Status      *status.Status
	CurrentSlot *slot.Slot
	Context     gcontext.Context

	DependsOnRaw []string `hcl:"depends_on"`
	DependsOn    []*Unit
}

func (u *Unit) SetContext(c gcontext.Context) {
	u.Context = c
}

func (u *Unit) Start(ctxt *ishell.Context) error {
	lgr := u.Context.Logger()

	// First, see if we have any dependencies that need to be running.
	lgr.Debugf("unit %q has %d dependencies\n", u.Name, len(u.DependsOn))
	if len(u.DependsOn) > 0 {
		for i, dependency := range u.DependsOn {
			preambled := func(s string) string {
				return "[unit:%d][dependency:%d]" + s
			}

			lgr.Debugf(
				preambled(" checking status of dependency: %d\n"),
				u.Name,
				i,
				dependency.Name,
			)

			if dependency.HasStatus(status.Running) {
				lgr.Debugf(
					preambled("dependency %q is running, no action"),
					u.Name,
					i,
					dependency.Name,
				)
				continue
			}

			lgr.Debugf(
				preambled("starting dependency %q"),
				u.Name,
				i,
				dependency.Name,
			)
			if err := dependency.Start(ctxt); err != nil {
				return err
			}
		}
	}

	// Now, for some tomfoolery
	lgr.Debugf("[unit:%q] identifying slot\n", u.Name)
	slot, err := u.IdentifySlot()
	if err != nil {
		return err
	}

	if u.HasStatus(status.Running) && slot == u.CurrentSlot {
		return fmt.Errorf("%s is already running", u.Name)
	}

	lgr.Debugf("[unit:%q] starting slot %q\n", u.Name, slot.Name)
	return slot.Start(u, ctxt)
}

func (u *Unit) Restart(ctxt *ishell.Context) error {
	if err := u.Stop(ctxt); err != nil {
		return err
	}
	return u.Start(ctxt)
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

func (u *Unit) SavePIDFile(c *exec.Cmd) error {
	pid := c.Process.Pid
	home := os.Getenv("HOME")
	if len(home) == 0 {
		return errors.New("cannot determine home directory")
	}

	outputDir := filepath.Join(home, ".glorious", "state", "pid-files")

	if err := os.MkdirAll(
		outputDir,
		0744,
	); err != nil {
		return err
	}

	return ioutil.WriteFile(
		filepath.Join(outputDir, u.Name),
		[]byte(strconv.Itoa(pid)),
		0644,
	)

}

func (u *Unit) ProcessStatus() string {
	slot, _ := u.IdentifySlot()
	u.populateDockerStatus(slot)

	if u.Status == nil {
		return NOT_STARTED
	}
	return u.Status.String()
}

func (u *Unit) HasStatus(status status.UnitStatus) bool {
	return u.Status != nil && u.Status.CurrentStatus == status
}

func (u *Unit) Stop(ctxt *ishell.Context) error {
	if u.Status == nil {
		return gerrors.ErrStopStopped
	}

	if u.HasStatus(status.Stopped) {
		return fmt.Errorf("%s is already stopped", u.Name)
	}

	u.Status.MarkShutdownRequested()

	u.Status.Lock()
	defer u.Status.Unlock()

	return u.CurrentSlot.Stop(u, ctxt)
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

func (u *Unit) Init() error {
	slot, err := u.IdentifySlot()
	if err != nil {
		return err
	}

	if slot == nil || slot.Provider == nil {
		return fmt.Errorf("invalid provider for unit %q", u.Name)
	}

	if strings.HasPrefix(slot.Provider.Type, "docker") {
		if err := u.populateDockerStatus(slot); err != nil {
			return err
		}
	}

	return nil
}

func (u *Unit) IdentifySlot() (*slot.Slot, error) {
	// Short circuit if only a single slot exists.
	if len(u.Slots) == 0 {
		return nil, errors.New("unit has no slots")
	} else if len(u.Slots) == 1 {
		return &(u.Slots[0]), nil
	}

	// Always first identify the default slot
	var defaultSlot *slot.Slot
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
	var resolvedSlot *slot.Slot
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

func (u *Unit) populateDockerStatus(slot *slot.Slot) error {
	isRemote := slot.Provider.Type == "docker/remote"

	options := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if isRemote {
		options = append(options, client.WithHost(slot.Provider.Remote.Host))
	}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(options...)
	if err != nil {
		return err
	}

	if _, err = cli.ContainerInspect(ctx, u.Name); err != nil {
		if client.IsErrNotFound(err) {
			if u.Status != nil {
				u.Status = nil
			}

			return nil
		}
		return err
	}

	u.CurrentSlot = slot

	u.SetRunningStatus(status.NewRunningStatus(nil, nil), nil)

	return nil
}

func (u *Unit) Validate() []*gerrors.ErrWithPath {
	var unitErrs []*gerrors.ErrWithPath
	for _, slot := range u.Slots {
		if errs := slot.Validate(); len(errs) > 0 {
			for _, err := range errs {
				err.Path = append([]string{"unit", u.Name}, err.Path...)
			}
			unitErrs = append(unitErrs, errs...)
		}
	}
	return unitErrs
}

func (u *Unit) GetName() string {
	return u.Name
}

func (u *Unit) SetRunningStatus(stat *status.Status, cb status.StatusCallback) {
	u.Status = stat
	u.Status.ClearShutdown()

	if cb != nil {
		u.Status.Lock()
		defer u.Status.Unlock()

		cb(stat)
	}
}

func (u *Unit) GetStatus() *status.Status {
	return u.Status
}

func (u *Unit) SetCurrentSlot(s *slot.Slot) {
	u.CurrentSlot = s
}

func (u *Unit) UnsetCurrentSlot() {
	u.CurrentSlot = nil
}

func (u *Unit) InternalStore() *store.Store {
	return u.Context.InternalStore()
}
