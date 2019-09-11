
CONFIG ?= glorious.glorious

build:
	go build -o glorious .

run: build
	./glorious -config $(CONFIG)