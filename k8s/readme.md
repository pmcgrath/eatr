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

```
# Prepare k8s cluster - Namespace, service account, cluster role and cluster role binding
make k8s-prepare

# Deploy eatr deployment
make k8s-deployment-up

# Remove eatr deployment
make k8s-deployment-down
```



# Configure
```
# Add a secret with AWS credentials (ecr-puller IAM user) for specific ECR registry - can do this multiple times
make k8s-create-aws-creds-secret AWS_ACCOUNT_ID=Replace-me AWS_REGION=Replace-me AWS_ACCESS_KEY_ID=Replace-me AWS_SECRET_ACCESS_KEY=Replace-me

# Label a namespace that needs an ECR auth token to pull images - can do this for multiple namespaces and apply multiple times to a namespace
make k8s-label-namespace K8S_NAMESPACE=Replace-me AWS_ACCOUNT_ID=Replace-me AWS_REGION=Replace-me
```



# Clean up
```
# Delabel a namespace that no longer needs an ECR auth token to pull images
# Can use this to get label info: kubectl get namespaces -o json | jq -r '.items[].metadata | {name: .name, labels: .labels }'
make k8s-delabel-namespace K8S_NAMESPACE=Replace-me AWS_ACCOUNT_ID=Replace-me AWS_REGION=Replace-me

# Remove a namespace ECR secret - if not it will expire and image pulls will fail
make k8s-delete-ecr-token-secret K8S_NAMESPACE=Replace-me AWS_ACCOUNT_ID=Replace-me AWS_REGION=Replace-me

# Removed k8s cluster content - Namespace, service account, cluster role and cluster role binding. Namespace removal will remove the pod
make k8s-cleanup
```
