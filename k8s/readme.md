# Create AWS account to pull images
- ecr-puller is the default user
- You need to create this before completing the setup, have not included in the Makefile as there are any number of ways to manage this
	- Included a terraform aws-ecr-users.tf file as a reference
	- Could create with the followign aws commands

	```
		aws iam create-user --user-name ecr-puller
		aws iam attach-user-policy --policy-arn arn:aws:iam::aws:policy/AdministratorAccess --user-name ecr-puller
	```



# Setup
- PENDING - this needs fixing
- YAML files are templates rather than manifests - Can we use for CD - See http://ksonnet.heptio.com/ and http://jsonnet.org/

```
# Review the Makefile vars

# Prepare k8s cluster - Namespace, ClusterRoles, ClusterRoleBindings, etc.
# Where you already have the ecr-puller access key
make k8s-prepare AWS_ACCOUNT_ID=Replace-me AWS_ACCESS_KEY_ID=Replace-me AWS_SECRET_ACCESS_KEY=Replace-me

# Deply eatr pod
make k8s-deployment-up AWS_ACCOUNT_ID=Replace-me 

# Remove eatr pod
make k8s-deployment-down AWS_ACCOUNT_ID=Replace-me 
```
