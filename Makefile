
CONFIG ?= glorious.glorious

build: build-macos build-linux

run: build
	./glo -config $(CONFIG)

build-macos:
	GOOS=darwin go build -o glo .

build-linux:
	GOOS=linux go build -o glo.linux .

send:
	scp -i ~/.ssh/id_rsa glo.linux root@134.209.164.241:~/glo

send-config:
	scp -i ~/.ssh/id_rsa examples/simple.hcl root@134.209.164.241:~/simple.hcl
