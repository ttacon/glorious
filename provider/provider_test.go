package provider

import (
	"testing"

	"github.com/ttacon/glorious/errors"
)

func TestProviderValidate(t *testing.T) {

	var tests = []struct {
		Provider     Provider
		expectedErrs []error
	}{
		{
			Provider: Provider{
				Type: "foo",
			},
			expectedErrs: []error{
				errors.ErrUnknownProvider,
			},
		},
		// Bash specific
		{
			Provider: Provider{
				Type: "bash/local",
				Cmd:  "npm run start",
			},
			expectedErrs: nil,
		},
		{
			Provider: Provider{
				Type:       "bash/local",
				Cmd:        "npm run start",
				WorkingDir: "/home/yolo",
			},
			expectedErrs: nil,
		},
		{
			Provider: Provider{
				Type:       "bash/local",
				Cmd:        "npm run start",
				WorkingDir: "/home/yolo",
				Image:      "super/app",
			},
			expectedErrs: []error{errors.ErrBashExtraneousFields},
		},
		{
			Provider: Provider{
				Type:       "bash/remote",
				Cmd:        "npm run start",
				WorkingDir: "/home/yolo",
			},
			expectedErrs: []error{errors.ErrBashRemoteMissingRemote},
		},
		// Docker specific
		{
			Provider: Provider{
				Type:  "docker/local",
				Image: "super/app",
			},
			expectedErrs: nil,
		},
		{
			Provider: Provider{
				Type:  "docker/local",
				Cmd:   "npm run start",
				Image: "super/app",
			},
			expectedErrs: []error{errors.ErrDockerExtraneousFields},
		},
		{
			Provider: Provider{
				Type:  "docker/remote",
				Image: "super/app",
				Ports: []string{"0.0.0.0:80:80"},
			},
			expectedErrs: []error{errors.ErrDockerRemoteMissingRemote},
		},
	}

	for i, test := range tests {
		errs := test.Provider.Validate()
		if len(errs) != len(test.expectedErrs) {
			t.Errorf("[test %d] expected validation errors did not match returned: %v vs %v\n", i, errs, test.expectedErrs)
		}
	}
}
