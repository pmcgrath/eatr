# Switch to bash
SHELL=/bin/bash


# Parameters - defaulted
DOCKERHUB_REPO_NAME ?= ${USER}
IMAGE_NAME ?= eatr
VERSION ?= 0.1


# Derived or calculated
FULL_IMAGE_NAME = ${DOCKERHUB_REPO_NAME}/${IMAGE_NAME}
FULL_IMAGE_NAME_AND_TAG = ${FULL_IMAGE_NAME}:${VERSION}
REPO_BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
REPO_VERSION = $(shell git rev-parse HEAD)


build:
	@#Fast build - so we can run without having to wait for full static build
	go build -ldflags "-X main.version=${VERSION} -X main.repoBranch=${REPO_BRANCH} -X main.repoVersion=${REPO_VERSION}" .


test:
	go test -v ./...


build-static:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o eatr -ldflags "-X main.version=${VERSION} -X main.repoBranch=${REPO_BRANCH} -X main.repoVersion=${REPO_VERSION}" .


docker-build:
	docker image build --build-arg REPO_BRANCH=${REPO_BRANCH} --build-arg REPO_VERSION=${REPO_VERSION} --build-arg VERSION=${VERSION} --tag ${FULL_IMAGE_NAME_AND_TAG} .
