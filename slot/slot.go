package slot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
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
			image = s.getImageString(u, image)
			lgr.Infof("image %q not found locally, trying to pull...", image)

			d, pullErr := cli.ImagePull(
				context.Background(),
				image,
				s.dockerImagePullOptions(u),
			)
			if pullErr != nil {
				isUnauthorized := errdefs.IsUnauthorized(pullErr) ||
					strings.Contains(pullErr.Error(), "no basic auth credentials")
				lgr.Debug("pull failure was due to lack of authorization: ", isUnauthorized)
				lgr.Debug("failed to pull image: ", image)
				return pullErr
			}

			if resp, err := cli.ImageLoad(
				context.Background(),
				d,
				false,
			); err != nil {
				lgr.Debug("failed to load retrieved image, err: ", err)
				return err
			} else if err := resp.Body.Close(); err != nil {
				lgr.Debug("failed to closed response body, err: ", err)
				return err
			}

			if err := d.Close(); err != nil {
				lgr.Debug("failed to close pull request, err: ", err)
				return err
			}
		} else {
			lgr.Debug("failed to check for image: ", err)
			return err
		}
	}

	lgr.Debug("parsing host ports")
	hostConfig := &container.HostConfig{}
	if len(s.Provider.Ports) > 0 {
		bindings := nat.PortMap{}
		for _, port := range s.Provider.Ports {
			vals, err := nat.ParsePortSpec(port)
			if err != nil {
				lgr.Debug("failed to parse port spec: ", err)
				return err
			}
			for _, val := range vals {
				bindings[val.Port] = []nat.PortBinding{val.Binding}
			}
		}
		hostConfig.PortBindings = bindings
	}

	lgr.Debug("parsing host volumes")
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

	lgr.Debug("creating container for image: ", image)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Env:   s.Provider.Environment,
	}, hostConfig, nil, u.GetName())
	if err != nil {
		lgr.Debugf("failed to create container for image %q, err %s\n", image, err)
		return err
	}

	lgr.Debug("starting container for image: ", image)
	if err := cli.ContainerStart(
		ctx,
		resp.ID,
		types.ContainerStartOptions{},
	); err != nil {
		lgr.Debugf("failed to start container for image %q, err %s\n", image, err)
		return err
	}

	u.SetCurrentSlot(s)
	u.SetRunningStatus(status.NewRunningStatus(nil, nil), nil)

	lgr.Info("begun as container ", resp.ID)

	return nil
}

var (
	ecrImageRegex = regexp.MustCompile("^([a-zA-Z0-9][a-zA-Z0-9-_]*).dkr.ecr.([a-zA-Z0-9][a-zA-Z0-9-_]*).amazonaws.com(.cn)?\\/.*")
)

func (s *Slot) getImageString(u UnitInterface, image string) string {
	lgr := u.GetContext().Logger()

	lgr.Debug("identifying image string")
	if strings.Index(image, ":") > 0 {
		lgr.Debug("image string contains tag, no more work to do")
		return image
	}

	pieces := strings.SplitN(image, "/", 2)
	if len(pieces) != 2 {
		lgr.Debug("image in unexpected format: ", image)
		return image
	}

	registry := pieces[0]
	registryID := strings.SplitN(registry, ".", 2)[0]
	repository := pieces[1]

	// We only support tag identification functionality for AWS ECR,
	// currently.
	if !ecrImageRegex.MatchString(image) {
		lgr.Debug("image is not in an AWS ECR registry, no more work to do")
		return image
	}

	lgr.Debug("generating AWS ECR session")
	sesh, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		lgr.Debug("failed to generate AWS session: ", err)
		return image
	}
	svc := ecr.New(sesh)

	// Use DescribeImages
	//
	// https://docs.aws.amazon.com/sdk-for-go/api/service/ecr/#DescribeImagesInput
	//
	// need the repository name and (optional, registry ID (i.e. AWS account ID))
	var mostRecentTag string
	if err := svc.DescribeImagesPages(&ecr.DescribeImagesInput{
		RegistryId:     aws.String(registryID),
		RepositoryName: aws.String(repository),
	}, func(page *ecr.DescribeImagesOutput, lastPage bool) bool {
		images := page.ImageDetails

		// TODO(ttacon): identify a better way to pull tags here
		//
		// One concern is if this is a named tag, we should look for an
		// image's unique (hash) tag.
		mostRecentTag = *(images[len(images)-1].ImageTags[0])
		return !lastPage
	}); err != nil {
		lgr.Debug("failed to search all image tags, err: ", err)
		return image
	}

	if len(mostRecentTag) == 0 {
		lgr.Debug("failed to identify tag, exiting")
		return image
	}

	lgr.Debug("identified tag as most recent: ", mostRecentTag)

	// these our returned in order from oldest to newest, so we'll have to page through
	// all of them...
	//
	// if the library supported better querying, we could do:
	// https://stackoverflow.com/a/49413539/11254876

	return image + ":" + mostRecentTag
}

func (s *Slot) dockerImagePullOptions(u UnitInterface) types.ImagePullOptions {
	var (
		authFunc func(gcontext.Logger, string) types.RequestPrivilegeFunc
		opts     types.ImagePullOptions

		lgr = u.GetContext().Logger()
	)
	lgr.Debug("generating docker ImagePullOptions")

	if s.Provider.Extra == nil {
		lgr.Debug("no extra config info found")
		return opts
	} else if funcType, ok := s.Provider.Extra["authProvider"]; !ok {
		lgr.Debug("no authProvider identifier")
		return opts
	} else if funcType == "aws/ecr" {
		lgr.Debug("identified authProvider of aws/ecr")
		authFunc = awsAuthFunc
	} else {
		lgr.Debugf("unknown auth function %q, exiting\n", funcType)
		return opts
	}

	image := s.Provider.Image
	lgr.Debugf("attempting to pull image %q with PrivilegeFunc\n", image)
	token, err := authFunc(lgr, image)()
	if err != nil {
		lgr.Debug("failed to generate auth information, err: ", err)
		return opts
	}

	return types.ImagePullOptions{
		RegistryAuth:  token,
		PrivilegeFunc: authFunc(lgr, image),
	}
}

func awsAuthFunc(lgr gcontext.Logger, image string) types.RequestPrivilegeFunc {
	// Separate out register, repository and tag info
	//
	// 351073081746.dkr.ecr.us-east-1.amazonaws.com/mixmax-apps/files:53226da5

	var registry string
	if pieces := strings.SplitN(image, "/", 2); len(pieces) == 2 {
		registry = pieces[0]
	}

	return func() (string, error) {
		lgr.Debug("generating AWS SDK client")
		sesh, err := session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
		})
		if err != nil {
			lgr.Debug("failed to generate AWS session: ", err)
			return "", err
		}
		svc := ecr.New(sesh)

		// TODO(ttacon): support specifying the registry.
		// https://docs.aws.amazon.com/sdk-for-go/api/service/ecr/#GetAuthorizationTokenInput
		// https://docs.aws.amazon.com/sdk-for-go/api/service/ecr/#ECR.GetAuthorizationToken
		lgr.Debug("requesting ECR authentication token")
		authTokenInput := ecr.GetAuthorizationTokenInput{}
		if len(registry) > 0 {
			authTokenInput.RegistryIds = []*string{
				aws.String(strings.Split(registry, ".")[0]),
			}
		}
		result, err := svc.GetAuthorizationToken(&authTokenInput)
		if err != nil {
			lgr.Debug("failed to retrieve authorization token: ", err)
			return "", err
		} else if len(result.AuthorizationData) == 0 {
			lgr.Debug("ECR authorization response had no authorization data")
			return "", errors.New("no valid authorization returned")
		}

		lgr.Debug("ECR returned authorization data (num): ", len(result.AuthorizationData))
		token := *result.AuthorizationData[0].AuthorizationToken
		decodedToken, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return "", err
		}

		parts := strings.Split(string(decodedToken), ":")

		rawJSON, _ := json.Marshal(map[string]string{
			"username": parts[0],
			"password": parts[1],
		})
		return base64.StdEncoding.EncodeToString(rawJSON), nil
	}
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

	// Test if this is path-like first, if not, try to resolve it.
	path, err := exec.LookPath(pieces[0])
	if err != nil {
		fmt.Printf("failed to lookup %q: %v\n", pieces[0], err)
	} else {
		fmt.Printf("resolved %q to %q\n", pieces[0], path)
		pieces[0] = path
	}

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
