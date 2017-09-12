ARG        GO_VERSION=1.9.0

FROM       golang:${GO_VERSION} as builder

ARG        VERSION=0.1
ARG        REPO_BRANCH
ARG        REPO_VERSION

COPY       .  /go/src/app/
WORKDIR    /go/src/app
RUN        go get -u github.com/golang/dep/cmd/dep && \
           dep ensure && \
           CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o eatr -ldflags "-X main.version=${VERSION} -X main.repoBranch=${REPO_BRANCH} -X main.repoVersion=${REPO_VERSION}" .

# For an explanation of why we need to repeat the ARGs, see https://github.com/moby/moby/issues/34129
# Also needed to copy the CA certifictes to scratch as it has no content and the AWS package calls will fail
FROM       scratch
ARG        VERSION=0.1
ARG        REPO_VERSION
LABEL      version=${VERSION}
LABEL      repo.version=${REPO_VERSION}
COPY       --from=builder /go/src/app/eatr .
COPY       --from=builder /etc/ssl/certs/ca-certificates.crt  /etc/ssl/certs/ca-certificates.crt
EXPOSE     5000
ENTRYPOINT ["/eatr"]
