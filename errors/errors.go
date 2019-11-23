package errors

import (
	"errors"
	"fmt"
	"strings"
)

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

	ErrStopStopped = errors.New("cannot stop stopped unit")
)

type ErrWithPath struct {
	Path []string
	Err  error
}

func (s ErrWithPath) Error() string {
	return fmt.Sprintf("[%s] %s", strings.Join(s.Path, "."), s.Err)
}
