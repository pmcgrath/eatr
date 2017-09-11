# Create AWS account to pull images
- ecr-puller - one for the registry
- Used terraform to create the user with an existing AWS policy to allow pulls
	- Needed to run terrafrom init
	- PENDING - Can we control the location of the plugins content - do not want to commit this - too big, is this new ?
- Move most of the templates to the eatr repo
- YAML files are templates rather than manifests - Can we use for CD - See http://ksonnet.heptio.com/ and http://jsonnet.org/
- Thought about running as cronjob, but would not deal with new namespaces - Will need something like this for the jetNEXUS possibly ?
- Started with a script but again, how to we deal with failures and new namespaces



# Setup
```
# Review the Makefile vars in particular the AWS profile

# Create AWS user(s) and permissions
make aws-create-user-plan
make aws-create-user-apply

# Prepare k8s cluster - Namespace, ClusterRoles, ClusterRoleBindings, etc.
make k8s-prepare

# Deply eatr pod
make k8s-deployment-up
```
