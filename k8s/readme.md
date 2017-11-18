# Create AWS account to pull images - should do this for each AWS account we will need to pull images from - here I assume it is okay to pull from all repositories in the registry
- ecr-puller is the default user
- You need to create this before you need to pull ECR images, have not included in the Makefile as there are any number of ways to manage this
	- Included a terraform aws-ecr-users.tf file as a reference
	- Could create with the followign aws cli commands

	```
		aws iam create-user --user-name ecr-puller
		aws iam attach-user-policy --policy-arn arn:aws:iam::aws:policy/AdministratorAccess --user-name ecr-puller
	```



# Setup
- YAML files are templates rather than manifests - Will want to use http://ksonnet.heptio.com/ and http://jsonnet.org/ in the future

```
# Prepare k8s cluster - Namespace, service account, cluster role and cluster role binding
make k8s-prepare

# Add a secret for specific ECR registry - can do this multiple times
make k8s-create-aws-creds-secret AWS_ACCOUNT_ID=Replace-me AWS_REGION=Replace-me AWS_ACCESS_KEY_ID=Replace-me AWS_SECRET_ACCESS_KEY=Replace-me

# Deploy eatr deployment
make k8s-deployment-up

# Remove eatr deployment
make k8s-deployment-down
```