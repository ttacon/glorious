package main

import (
	"errors"
	"fmt"
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
				ErrBashRemoteMissingRemote,
			)
		}
		fallthrough
	case "bash/local":
		if len(p.Cmd) == 0 {
			errs = append(
				errs,
				ErrBashMissingCommand,
			)
		}
		if len(p.Image) > 0 ||
			len(p.Ports) > 0 ||
			len(p.Volumes) > 0 ||
			len(p.Environment) > 0 {
			errs = append(errs, ErrBashExtraneousFields)
		}
	case "docker/remote":
		if len(p.Remote.Host) == 0 {
			errs = append(errs, ErrDockerRemoteMissingRemote)
		}
		fallthrough
	case "docker/local":
		if len(p.Image) == 0 {
			errs = append(errs, ErrDockerMissingImage)
		}
		if len(p.Cmd) > 0 || len(p.WorkingDir) > 0 {
			errs = append(errs, ErrDockerExtraneousFields)
		}
	default:
		return []error{ErrUnknownProvider}
	}
	return errs
}

type ProviderErr struct {
	ProviderType string
	Err          error
}

func (p ProviderErr) Error() string {
	return fmt.Sprintf("[%s] %s", p.ProviderType, p.Err)
}

var (
	ErrUnknownProvider         = errors.New("unknown provider")
	ErrBashRemoteMissingRemote = ProviderErr{
		"bash/remote",
		errors.New("must provide, at least, both host and user"),
	}
	ErrBashMissingCommand = ProviderErr{
		"bash/*",
		errors.New("must provide command"),
	}
	ErrBashExtraneousFields = ProviderErr{
		"bash/*",
		errors.New("provider does not support fields beyond cmd, workingDir, remote and resolver"),
	}
	ErrDockerRemoteMissingRemote = ProviderErr{
		"docker/remote",
		errors.New("must provide remote docker host"),
	}
	ErrDockerMissingImage = ProviderErr{
		"docker/*",
		errors.New("must provider docker image"),
	}
	ErrDockerExtraneousFields = ProviderErr{
		"docker/*",
		errors.New("provider does not support cmd or workingDir"),
	}
)
