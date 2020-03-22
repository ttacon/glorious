package config

import (
	"testing"

	"github.com/ttacon/glorious/errors"
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
			expectedErrs: []error{errors.ErrBashRemoteMissingRemote},
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

func TestGloriousConfig_Dependencies(t *testing.T) {
	config, err := ParseConfig(appWithDependencies)
	if err != nil {
		t.Error("failed to parse config, err: ", err)
		t.Fail()
	}

	app, exists := config.GetUnit("app")
	if !exists {
		t.Error("app should exist")
	}

	if len(app.DependsOn) != 2 {
		t.Error("app should have two dependencies, found: ", app.DependsOnRaw)
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

	appWithDependencies = `
unit "db" {
  name = "db"
  description = "Mongo DB"

  slot "dev" {
    provider {
      type = "docker/local"
      image = "mongo:4.2.3-bionic"
    }
  }
}

unit "cache" {
  name = "cache"
  description = "Cache for app"

  slot "dev" {
    provider {
      type = "docker/local"
      image = "redis:5"
    }
  }
}

unit "app" {
  name = "app"
  description = "application"

  depends_on = [ "db", "cache" ]

  slot "dev" {
    provider {
      type = "bash/local"
      cmd = "npm start"
    }
  }
}
`
)
