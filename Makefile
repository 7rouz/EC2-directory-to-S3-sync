NAME=ec2-to-s3-sync
VERSION=$(shell git describe --tags --always)

build:
	mkdir -p bin
	go build -v -i --ldflags '-s -extldflags "-static" -X main.version=${VERSION}' -o bin/${NAME} .

build-image:
	docker build -f docker/Dockerfile --build-arg binaryName=${NAME} --no-cache -t 7rouz/${NAME}:${VERSION} .
