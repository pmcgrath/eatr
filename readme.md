# Purpose
This contains a simple kubernetes controller that renews AWS ECR authorization token [image pull secrets](https://kubernetes.io/docs/concepts/containers/images/) periodically, it also needs to cater for newly created namespaces

This is specifically for a kubernetes cluster not running on EC2 instances, but where we need to pull images from AWS's ECR

Current implementation creates image pull secrets in each namespace that is not blacklisted, may change this to be based on a namespace having a specific label or annotation



# Hot it works
## Preparation
- Create an AWS user that has permission to read from an AWS account registry (ecr-puller in our case)
- Create a k8s namespace (ci-cd in our case)
- Create a secret in the ci-cd namespace which will be used to renew the ECR authorization token (This is for the AWS IAM user that has permissions to pull all images from our AWS account registry)
- Create a k8s service account, cluster role and cluster role binding for our deployment
- Build a docker image and push to docker hub (Nothing sensitive in the image)

## Run ECR authorization token renewer instance
- Run a deployment with this app as a single instance pod

## How it works
- It initially reads the AWS credentials secret
- It initially reads all the namespaces and for each that is not blacklisted it creates a new image pull secret in the namespace based on a new ECR authorization token
- It periodically renews the image pull secrets in all non blacklisted namespaces based on a new ECR authorization token
- It reacts to any newly added namespaces which are not blacklisted creating a new image pull secret based on a new ECR authorization token



# Metrics
- The instance also surfaces the following prometheus metrics (counters)

| Counter name           | Description                              |
| -----------------------| -----------------------------------------|
| secrets_created_total  | Number of secrets that have been created |
| secret_renewals_total  | Number of secret renewals made           |



# How to build
- Should use golang v1.9

## Dependencies
- Decided not to commit the vendor directory at this time
- Explicitly added as a .gitignore so repo is small
- Dependency management tool

```
# See https://github.com/golang/dep
go get -u github.com/golang/dep/cmd/dep
```
- I have commited the Gopkg.toml and Gopkg.lock dep files, so you should be able to restore the vendor directory with

```
dep ensure -v
```
- client-go package dependencies can be tricky so I have used these based on https://github.com/heptio/ark/blob/master/Godeps/Godeps.json commit b7265a59f2b912d733c991bd993ce75d66053d6a

```
dep ensure -v k8s.io/client-go@v4.0.0-beta.0
dep ensure -v k8s.io/apimachinery@abe34e4f5b4413c282a83011892cbeea5b32223b
```

- What are the k8s.io rep deps for client-go
```
# Find all go files with a '"k8s.io' content in a line, using the " sperator get import package name, using the / separator get the second field - this is the k8s.io repo name
grep -r '"k8s.io' --include '*.go' -w ~/go/src/k8s.io/client-go | awk 'BEGIN{ FS="\"" }; { print $2 }' | awk 'BEGIN{ FS="/" }; {print $2}' | sort | uniq
```

## Build locally
- PENDING

## Run from outside the cluster
- PENDING

## Build docker image
- PENDING

## Push docker image
- PENDING



# Getting up and running on the k8s cluster
- PENDING
