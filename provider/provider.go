package provider

import (
	"github.com/ttacon/glorious/errors"
)

type Provider struct {
	Type string `hcl:"type"`

	WorkingDir string `hcl:"workingDir"`
	Cmd        string `hcl:"cmd"`

	Image       string   `hcl:"image"`
	Ports       []string `hcl:"ports"`
	Volumes     []string `hcl:"volumes"`
	Environment []string `hcl:"environment"`

	Remote   RemoteInfo    `hcl:"remote"`
	Handlers []HandlerInfo `hcl:"handler"`
}

func (p *Provider) Validate() []error {
	var errs []error
	switch p.Type {
	case "bash/remote":
		if len(p.Remote.Host) == 0 || len(p.Remote.User) == 0 {
			errs = append(
				errs,
				errors.ErrBashRemoteMissingRemote,
			)
		}
		fallthrough
	case "bash/local":
		if len(p.Cmd) == 0 {
			errs = append(
				errs,
				errors.ErrBashMissingCommand,
			)
		}
		if len(p.Image) > 0 ||
			len(p.Ports) > 0 ||
			len(p.Volumes) > 0 ||
			len(p.Environment) > 0 {
			errs = append(errs, errors.ErrBashExtraneousFields)
		}
	case "docker/remote":
		if len(p.Remote.Host) == 0 {
			errs = append(errs, errors.ErrDockerRemoteMissingRemote)
		}
		fallthrough
	case "docker/local":
		if len(p.Image) == 0 {
			errs = append(errs, errors.ErrDockerMissingImage)
		}
		if len(p.Cmd) > 0 || len(p.WorkingDir) > 0 {
			errs = append(errs, errors.ErrDockerExtraneousFields)
		}
	default:
		return []error{errors.ErrUnknownProvider}
	}
	return errs
}

type RemoteInfo struct {
	Host         string `hcl:"host"`
	User         string `hcl:"user"`
	IdentityFile string `hcl:"identityFile"`
	WorkingDir   string `hcl:"workingDir"`
}

type HandlerInfo struct {
	Type    string `hcl:"type"`
	Match   string `hcl:"match"`
	Exclude string `hcl:"exclude"`
	Cmd     string `hcl:"cmd"`
}
