# Purpose
This contains a simple kubernetes controller that renews AWS ECR authorization token [image pull secrets](https://kubernetes.io/docs/concepts/containers/images/) periodically, it also needs to cater for newly created namespaces or updated namespaces

This is specifically for a kubernetes cluster not running on EC2 instances, but where we need to pull images from multiple AWS ECRs

Current implementation creates image pull secrets in each namespace that are labeled with a key that matches a ECR DNS, this allows for using multiple ECR registries

Will need to create an AWS credential secret in the ci-cd namespace for each ECR registry that we need to pull images from



# Hot it works
## Preparation
- See k8s/readme.md
- Create an AWS user that has permission to read from an AWS account registry (ecr-puller in our case)
- Create a k8s namespace (ci-cd in our case)
- Create secrets in the ci-cd namespace which will be used to renew the ECR authorization tokens (This is for the AWS IAM user that has permissions to pull all images from our AWS account registies)
- Create a k8s service account, cluster role and cluster role binding for our deployment
- Build a docker image and push to docker hub (Nothing sensitive in the image)


## Run ECR authorization token renewer instance
- Run a deployment with this app as a single instance pod, see k8s/readme.md


## How the controller works
- It uses a namespace informer to react to new or updated cluster namespaces
- Initially the informer raises an add event for each of the existing cluster namespaces
- It will try to create image pull secrets for namespaces that have labels that match a ECR DNS, if an equivalent AWS ECR credential secret exists in the host namesapce (ci-cd)
- It periodically renews the image pull secrets for all the cluster namespaces, this addresses the 12 hour ECR expiry
- It reacts to any newly added or updated cluster namespaces creating new image pull secrets if appropriate labels are found
	- Currently recreates all the cluster namespace image pull secrets as we do not expect namespaces to be modified very often, so lets keep it simple



# Metrics
- The instance also surfaces the following prometheus metrics (counters)

| Counter name           | Description                              |
| -----------------------| -----------------------------------------|
| secrets_created_total  | Number of secrets that have been created |
| secret_renewals_total  | Number of secret renewals made           |



# How to build
- Should use golang v1.9

## Dependencies
- Deps can be tricky with client-golang
	- [Compatability matrix](https://github.com/kubernetes/client-go)
	- [Example with dep overrides](https://www.nirmata.com/2017/08/28/building-the-kubernetes-go-client-using-dep/)
	- [Heptio guidance from a while back](https://blog.heptio.com/straighten-out-your-kubernetes-client-go-dependencies-heptioprotip-8baeed46fe7d)
- Decided not to commit the vendor directory at this time
- Explicitly added as a .gitignore so repo is small
- Dependency management tool, see https://github.com/golang/dep
```
go get -u github.com/golang/dep/cmd/dep
```

- This has been my k8s.io dance, having already cloned client-go etc, I went with client-go tag v5.0.1
	- I have been happy to go with master for all the other dependencies, have not had issues
	- I expect all the k8s.io repos will migrate from using godep to the new dep tool, so some of this will get easier and be out of date soon hopefully
- I created local branches for the following k8s.io repos, while developing so I have consistent dependencies, see below for the specific commits

| Repo                | Commit\Tag                               | Branch creation                                                              | Why                                    |
| --------------------| -----------------------------------------| -----------------------------------------------------------------------------| ---------------------------------------|
| k8s.io/api          | 6c6dac0277229b9e9578c5ca3f74a4345d35cdc2 | git checkout -b pmcg-client-go-v5.0.1 6c6dac0277229b9e9578c5ca3f74a4345d35cdc2 | Matches client-go, see below         |
| k8s.io/apimachinery | 019ae5ada31de202164b118aee88ee2d14075c31 | git checkout -b pmcg-client-go-v5.0.1 019ae5ada31de202164b118aee88ee2d14075c31 | Matches client-go, see below         |
| k8s.io/client-go    | v5.0.1                                   | git checkout -b pmcg-client-go-v5.0.1 v5.0.1                                   | Matches clinet-go matrix for k8s 1.8 |


```
# Initialise the vendor dir
dep init -v

# Now check client-go deps for a specific tag
pushd .
cd ~/go/src/k9

## List tags
git tag -l
the_tag=v5.0.1

# Check that still using godep tool to manage deps
git ls-tree -r $the_tag | grep Godeps/Godeps.json

# Get list of client-go k8s.io deps
echo $(git show $the_tag:Godeps/Godeps.json) | jq -r '.Deps[] | select(.ImportPath | startswith("k8s.io/")) | .Rev + " " + (.ImportPath | split("/")[1])' | sort | uniq
	6c6dac0277229b9e9578c5ca3f74a4345d35cdc2 k8s.io/api
	019ae5ada31de202164b118aee88ee2d14075c31 k8s.io/apimachinery
	868f2f29720b192240e18284659231b440f9cda5 k8s.io/kube-openapi

# Now lets update the dep file Gopkg.toml as follows
	[[constraint]]
	  name = "k8s.io/client-go"
	  version = "5.0.1"

	[[override]]
	  name = "k8s.io/api"
	  revision = "6c6dac0277229b9e9578c5ca3f74a4345d35cdc2"

	[[override]]
	  name = "k8s.io/apimachinery"
	  revision = "019ae5ada31de202164b118aee88ee2d14075c31"

# Now lets re-populate the vendor directory
popd
dep ensure -v


# Can I use these ?
dep ensure -v k8s.io/client-go@v5.0.1
dep ensure -v k8s.io/apimachinery@abe34e4f5b4413c282a83011892cbeea5b32223b
```

- What are the k8s.io rep deps for client-go
```
# Find all go files with a '"k8s.io' content in a line, using the " sperator get import package name, using the / separator get the second field - this is the k8s.io repo name
#grep -r '"k8s.io' --include '*.go' -w ~/go/src/k8s.io/client-go | awk 'BEGIN{ FS="\"" }; { print $2 }' | awk 'BEGIN{ FS="/" }; {print $2}' | sort | uniq
```

- I have commited the Gopkg.toml and Gopkg.lock dep files, so you should be able to restore the vendor directory with


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

# Assumes your kube config file is at ~/kube/config or you have set KUBECONFIG env var, also assumes the user has privileges
# Run with fast renew all loop (20 seconds), informers resync interval (5 seconds) and verbose logging (6, to see more glog logs from the client-go componemts can use 9)
./eatr \
  -auth-token-renewal-interval 20s \
  -informers-resync-interval 5s \ 
  -logging-verbosity-level 6 

# Can see metrics with 
curl localhost:5000/metrics
```


## Build docker image - Will build a statically linked binary via a multistage docker file, needs a recent docker CE and will be slow......
```
# Passing version via make arg
make image VERSION=30 

# Using VERSION file - prefered method
# edit VERSION
make image
```


## Push docker image
- Did not bother adding a make target
- Will just use automated builds in dockerhub, hence the hooks directory, see https://github.com/pmcgrath/dhab



# Getting up and running on the k8s cluster
- See k8s/readme.md