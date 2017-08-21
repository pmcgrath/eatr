ARG        GO_VERSION=1.8.3

FROM       golang:${GO_VERSION} as builder

ARG        VERSION=1.0
ARG        REPO_BRANCH
ARG        REPO_VERSION

# Assumes vendor directory exists already so no need to run "go get" here
COPY       .  /go/src/app/
WORKDIR    /go/src/app
RUN        go get -u github.com/golang/dep/cmd/dep && \
           dep ensure && \
           CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o eatr -ldflags "-X main.version=${VERSION} -X main.repoBranch=${REPO_BRANCH} -X main.repoVersion=${REPO_VERSION}" .

FROM       scratch
# See for why we need to repeat the ARGs, see https://github.com/moby/moby/issues/34129
ARG        VERSION=1.0
ARG        REPO_VERSION
LABEL      version=${VERSION}
LABEL      repo.version=${REPO_VERSION}
COPY       --from=builder /go/src/app/eatr .
EXPOSE     5000
ENTRYPOINT ["/eatr"]
