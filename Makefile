
include makefile-extensions/*.mk

CONFIG ?= glorious.glorious

build: build-macos build-linux

run: build
	./glo -config $(CONFIG)

build-macos:
	GOOS=darwin go build -o glo .

build-linux:
	GOOS=linux go build -o glo.linux .

