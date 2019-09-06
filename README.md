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


### Running list of todos

 - [ ] documentation on configuring `dockerd` for remote API access
 - [ ] `.glorious` file verification checks at boot up
 - [ ] daemon for process status monitoring (?)
 - [ ] ability to identify if remote processes and docker containers are
       still running
 - [ ] remote command execution (for "bash/remote" mode)
 - [ ] file watching and remote synchronization
 - [ ] remote dev mode constraints (initial setup, command hooks - i.e. npm
       install after `package[-lock].json` changes)
 - [ ] debug log mode
 - [ ] support volume mounting and envvar files for "docker/*" modes


