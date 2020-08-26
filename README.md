# glorious

The goal of this project is for people to stumble upon `glorious` and go:

![glorious](https://media.giphy.com/media/yidUzHnBk32Um9aMMw/giphy.gif).

## Goals of this project

### Multiple ways to run a project

You often want to run a project in different manners depending on certain
criteria. What's an example? Imagine you have 20 services that you need to
run in order to fully test a system, but you only need to actively work on
one. Normally, you have to run all 20 services - which isn't fun.

What if you had container images for each of these? It sure would be nice to
run those instead of every project in dev mode. This is where `glorious` comes
in. With `glorious` you can have multiple [slots](./notes.md), where `glorious`
will determine which `slot` to run depending on the provided criteria. This
leads to less load locally, saving your computer's resources.

#### units

Units are the top level components in `glorious`. A `unit` defines the *what*
of what we're trying to run (is it a server, redis node, etc). At the top level,
a `unit` will usually contain a `name`, a `description` and one or more `slot`s.


#### slots

Slots are how we run a `unit`. Each slot will have a `provider`, how we'll run
it, and a `resolver` which defines when we should run it. We'll check this out
in an example a little bit later.


### Running code remotely
With glorious, you can run code remotely via tunneling to a remote server or
via the dockerd API.


### Providers

`glorious` currently supports four provider types (plugins coming!):

 - `bash/local`: For running code locally.
 - `bash/remote`: For running code remotely.
 - `docker/local`: For running docker images locally.
 - `docker/remote`: For running docker code remotely.

### Auto-detecting new versions of code
TB filled out (new images, code, etc)


### Example config

```hcl
// Remote app
unit "remote-app" {
  name = "remote-app"
  description = "example remote app"
  
  slot "image-slot" {
    provider {
      type = "docker/remote"
      image = "my-docker-hub/example-app:latest"
      remote {
        host = "tcp://dev.remote.box:2376"
      }
    }

    resolver {
      type = "keyword/value"
      keyword = "services/app/remote/dev-mode"
      value = "true"
    }
  }
  
  slot "dev-slot" {
    type = "bash/remote"
    workingDir = "/home/user/code/app"
    cmd = "npm run start"
    remote {
      workingDir = "/home/user/code/app"
      host = "dev.remote.box"
      user = "user"
      identityFile = "~/.ssh/user-key.pem"
    }

    handler "rsync" {
      type = "rsync/remote"
      exclude = "node_modules"
    }

    handler "npm-install" {
      type = "execute/remote"
      match = ".*package(-lock)?.json$"
      cmd = "npm install"
    }
  }
}

// Local app
unit "local-app" {
  name = "local-app"
  description = "example local app"

  slot "dev-slot" {
    provider {
      type = "bash/local"
      workingDir = "/home/user/code/app"
      cmd = "npm run start"
    }

    resolver {
      type = "default"
    }
  }

  slot "image-slot" {
    provider {
      type = "docker/local"
      image = "my-docker-hub/example-app:latest"
    }

    resolver {
      type = "keyword/value"
      keyword = "services/app/local/dev-mode"
      value = "false"
    }
  }
}
```

### Misc

#### dockerd remote API setup
`dockerd` supports exposing the API remotely if the daemon is configured to do
so. If on a `systemd` based OS, you can do this by updating your
`/etc/sysconfig/dockerd` file by adding the following flags to the `OPTIONS`
var:

```sh
-H=unix://  -H=tcp://0.0.0.0:2376
```

i.e.
```sh
OPTIONS="--default-ulimit nofile=1024:4096  -H=unix://  -H=tcp://0.0.0.0:2376"
```

To enforce some degree of security, you should enforce firewall rules to only
allow traffic to port 2376 from your IP address.

Once you have this configured, reload the config file and restart dockerd:

```sh
sudo systemctl daemon-reload
sudo systemctl restart docker.service
```

For more details, see Docker's notes here: https://success.docker.com/article/how-do-i-enable-the-remote-api-for-dockerd.

### Running list of todos

 - [x] documentation on configuring `dockerd` for remote API access
 - [x] `.glorious` file verification checks at boot up
 - [x] ~daemon for process status monitoring (?)~
   - instead, each provider should be responsible for its own state tracking.
 - [x] ability to identify if remote processes and docker containers are
       still running
 - [x] support PID file based (bash) process state identification
 - [x] remote command execution (for "bash/remote" mode)
 - [x] file watching and remote synchronization
 - [x] remote dev mode constraints (initial setup, command hooks - i.e. npm
       install after `package[-lock].json` changes)
 - [x] debug log mode
 - [x] support volume mounting and envvar files for "docker/*" modes


