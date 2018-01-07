# Purpose
This contains a simple kubernetes controller that renews AWS ECR authorization token [image pull secrets](https://kubernetes.io/docs/concepts/containers/images/) periodically, it also needs to cater for newly created or updated namespaces

This is specifically for a kubernetes cluster not running on EC2 host instances, but where we need to pull images from multiple AWS ECRs

Current implementation creates image pull secrets in each namespace that are labelled with a key that matches a ECR DNS, this allows for using multiple ECR registries

Will need to create an AWS credential secret in the ci-cd namespace for each ECR registry that we need to pull images from

Expects to run on a 1.9+ cluster



# Hot it works
## Preparation
- See k8s/readme.md
- Create an AWS IAM user that has permission to read from an AWS account registry (ecr-puller in our case)
- Create a k8s namespace (ci-cd in our case)
- Create AWS credential secrets in the ci-cd namespace which will be used to renew the ECR authorization tokens (This is for the AWS IAM user that has permissions to pull all images from our AWS account registies)
- Create a k8s service account, cluster role and cluster role binding for our deployment
- Build a docker image and push to docker hub (Nothing sensitive in the image)


## Run ECR authorization token renewer instance
- Run a deployment with this app as a single instance pod, see k8s/readme.md


## How the controller works
- It uses a namespace informer to react to new or updated cluster namespaces
- Initially the informer raises an add event for each of the existing cluster namespaces
- It will try to create image pull secrets for namespaces that have labels that match a ECR DNS, if an equivalent AWS ECR credential secret exists in the host namespace (ci-cd)
- It periodically renews the image pull secrets for all the cluster namespaces, this addresses the 12 hour ECR expiry
- It reacts to any newly added or updated cluster namespaces creating new image pull secrets if appropriate labels are found
	- Currently re-creates all the cluster namespace image pull secrets as we do not expect namespaces to be modified very often, so lets keep it simple



# Metrics
- The instance surfaces the following prometheus metrics (counters)

| Counter name           | Description                                                                                |
| -----------------------| -------------------------------------------------------------------------------------------|
| secrets_created_total  | Number of secrets that have been created (new or updated), uses a namespace and name label |
| secret_renewals_total  | Number of secret renewals made                                                             |



# How to build
- Should use golang v1.9+


## Dependencies
- Deps can be tricky with client-golang
	- [Compatability matrix](https://github.com/kubernetes/client-go)
- Decided not to commit the vendor directory at this time
- Explicitly added as a .gitignore so repo is small
- Dependency management tool, see https://github.com/golang/dep
```
go get -u github.com/golang/dep/cmd/dep
```
- Hurray for dep it just worked
- I have up to date copies of the following repos, with all their deps
	- github.com/aws/aws-sdk-go"
	- github.com/golang/glog"
	- github.com/pkg/errors"
	- github.com/prometheus/client_golang"
	- github.com/stretchr/testify"
	- k8s.io/api
	- k8s.io/apimachinery
	- k8s.io/client-go
```
# Initialise the vendor dir
dep init -v
```
- I have commited the Gopkg.toml and Gopkg.lock dep files, so you should be able to restore the vendor directory


## Build locally
```
# Ensure you have dep tool and we have vendor content - Only need this once
make ensure-deps

# Build
make build
```


## Run from outside the cluster
```
# Get options
./eatr --help

# Assumes your kube config file is at ~/.kube/config or you have set the KUBECONFIG env var, also assumes the user has AWS privileges
# Run with fast renew all loop (20 seconds), informers resync interval (5 seconds) and verbose logging (6 to see more glog logs from the client-go componemts can use 9)
./eatr \
  -auth-token-renewal-interval 20s \
  -informers-resync-interval 5s \
  -logging-verbosity-level 6

# Can see metrics with
curl localhost:5000/metrics
```


## Build docker image
- Will build a statically linked binary via a multi-stage docker file, needs a recent docker CE and will be slow......

```
# Using VERSION file - prefered method
make image

# Passing explicit version via make arg
make image VERSION=30
```


## Push docker image
- Did not bother adding a make target
- Will just use automated builds in dockerhub, hence the hooks directory, see https://github.com/pmcgrath/dhab



# Getting up and running on the k8s cluster
## Create AWS account(s) to pull images - should do this for each AWS account we will need to pull images from - here I assume it is okay to pull from all repositories in the registry
- ecr-puller is the default user name
- You need to create this before you need to pull ECR images, have not included in the Makefile as there are any number of ways to manage this
- Included a terraform k8s/aws-ecr-users.tf file as a reference
- Could create with the following aws cli commands
```
aws iam create-user --user-name ecr-puller
aws iam attach-user-policy --policy-arn arn:aws:iam::aws:policy/AdministratorAccess --user-name ecr-puller
```

## Deploy to k8s cluster
- Assumes your kube config file is at ~/.kube/config or you have set the KUBECONFIG env var
- Deploy
```
kubectl apply -f k8s/eatr.yaml
```

## Create AWS ECR user credentials secrets
- Do this for each ECR puller AWS account and region that we need to pull images from
- First generate an AWS access key from the AWS console for the ecr-puller IAM user
```
aws_account_id=Replace-me
aws_region=Replace-me
aws_user_name=ecr-puller
k8s_namespace=ci-cd

# Don't want these in our history, access key is for the ecr-puller AWS IAM user
 aws_access_key_id=Replace-me
 aws_secret_access_key=Replace-me
 k8s/create-eatr-aws-credentials-k8s-secret.sh ${aws_account_id}" "${aws_region}" "${aws_user_name}" "${k8s_namespace}" "${aws_access_key_id}" "${aws_secret_access_key}"
```

## Label namespaces
- Label each namespace that needs to be able to pull ECR images
- A namespace may need to pull from multiple ECR registries, so apply multiple labels if needed
```
aws_account_id=Replace-me
k8s_namespace=fill-me-in
aws_region=Replace-me

kubectl label namespace ${k8s_namespace} ${aws_account_id}.dkr.ecr.${aws_region}.amazonaws.com="true"
```



# Clean up - removing content from the cluster
## Remove content
- Removes k8s cluster content - Namespace, service account, cluster role, cluster role binding and deployment
- Will not remove the namespace lables or namespace secrets
	- Can complete with the sections after this
	- Only issue is the auth token secrets will exist until the 12 hour expiry is completed after which they will be redundant
```
kubectl delete -f k8s/eatr.yaml
```

- Can do partial cleanups with the following

## De-label a namespace that no longer needs an ECR auth token to pull images
```
# Can use this to identify all labelled namespaces
kubectl get namespaces -o json | jq -r '.items[].metadata | select(has("labels") and (.labels | with_entries(select(.key | match(".*ecr.*.amazonaws.com$"))) | length) > 0) | {name: .name, labels: .labels}'

namespace=Replace-me
label_name=Replace-me

# Note trailing '-' which indicates removal
kubectl label namespace ${namespace} ${label_name}-
```

## Remove no longer needed ECR auth token secret from a namespace
```
# Can use this to identify candidate secrets
kubectl get secrets --all-namespaces --field-selector=type=kubernetes.io/dockerconfigjson

namespace=Replace-me
secret_name=Replace-me

kubectl delete secret ${secret_name} --namespace ${namespace} ${secret_name}
```

### Remove ECR IAM user credential secret
```
# Can use this to identify candidate secrets
kubectl get secrets --namespace ci-cd -o json | jq -r '.items[].metadata | select(.name | match("eatr-aws-credentials.*")) | .name'

secret_name=Replace-me

kubectl delete secret ${secret_name} --namespace ci-cd
```
