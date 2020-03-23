
CONFIG ?= glorious.glorious

build:
	go build -o glorious .

run: build
	./glorious -config $(CONFIG)

agent_bin:
	go build -o agent.exe ./agent