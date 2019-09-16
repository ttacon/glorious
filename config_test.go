package main

import (
	"testing"
)

func TestGloriousConfigValidate(t *testing.T) {
	var tests = []struct {
		raw          string
		expectedErrs []error
	}{
		{
			raw:          basicConfig,
			expectedErrs: nil,
		},
		{
			raw:          remoteBashErrConfig,
			expectedErrs: []error{ErrBashRemoteMissingRemote},
		},
	}

	for i, test := range tests {
		config, err := ParseConfig(test.raw)
		if err != nil {
			t.Error("failed to parse config unexpectedly: ", err)
		}

		errs := config.Validate()
		if len(errs) != len(test.expectedErrs) {
			t.Errorf("[test %d] expected validation errors did not match returned: %v vs %v\n", i, errs, test.expectedErrs)
		}
	}
}

const (
	basicConfig = `
unit "yolo" {
  name = "yolo"
  description = "yolo"

  slot "dev" {
    provider {
      type = "bash/local"
      cmd = "npm run start"
    }
  }
}
`

	remoteBashErrConfig = `
unit "yolo" {
  name = "yolo"
  description = "yolo"

  slot "dev" {
    provider {
      type = "bash/remote"
      cmd = "npm run start"
    }
  }
}
`
)
