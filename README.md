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

### Running code remotely
TB filled out

### Auto-detecting new versions of code
TB filled out (new images, code, etc)


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
 - [ ] `.glorious` file verification checks at boot up
 - [x] ~daemon for process status monitoring (?)~
   - instead, each provider should be responsible for its own state tracking.
 - [x] ability to identify if remote processes and docker containers are
       still running
 - [ ] support PID file based (bash) process state identification
 - [x] remote command execution (for "bash/remote" mode)
 - [ ] file watching and remote synchronization
 - [ ] remote dev mode constraints (initial setup, command hooks - i.e. npm
       install after `package[-lock].json` changes)
 - [ ] debug log mode
 - [ ] support volume mounting and envvar files for "docker/*" modes


