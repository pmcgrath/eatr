# Assumption for some targets is that the golang is already installed


# Switch default shell to be bash
SHELL=/bin/bash


# Parameters - defaulted
DOCKERHUB_REPO_NAME ?= ${USER}
IMAGE_NAME ?= eatr
VERSION ?= $(shell cat VERSION)


# Derived or calculated
BUILD_DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
FULL_IMAGE_NAME = ${DOCKERHUB_REPO_NAME}/${IMAGE_NAME}
FULL_IMAGE_TAG_NAME = ${FULL_IMAGE_NAME}:${VERSION}
REPO_BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
REPO_VERSION = $(shell git rev-parse HEAD)


default: build


ensure-deps:
	[[ -z "$(which dep)" ]] && go get -u github.com/golang/dep/cmd/dep
	dep ensure -v


build-with-deps: ensure-deps build


build:
	@# Fast build - so we can run without having to wait for full static build
	go build -ldflags "-X main.version=${VERSION} -X main.repoBranch=${REPO_BRANCH} -X main.repoVersion=${REPO_VERSION}" .


test:
	go test -v ./...


build-static:
	CGO_ENABLED=0 GOOS=linux go build \
		-a \
		-installsuffix cgo \
		-o eatr \
		-ldflags "-X main.version=${VERSION} -X main.repoBranch=${REPO_BRANCH} -X main.repoVersion=${REPO_VERSION}" \
		.


image:
	docker image build \
		--build-arg BUILD_DATE=${BUILD_DATE} \
		--build-arg REPO_BRANCH=${REPO_BRANCH} \
		--build-arg REPO_VERSION=${REPO_VERSION} \
		--build-arg VERSION=${VERSION} \
		--tag ${FULL_IMAGE_TAG_NAME} \
		.
