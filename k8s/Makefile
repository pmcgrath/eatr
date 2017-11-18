# Switch to bash
# SHELL=/bin/bash


# Defaults
AWS_ACCESS_KEY_ID ?=
AWS_ACCOUNT_ID ?=
AWS_REGION ?= eu-west-1
AWS_SECRET_ACCESS_KEY ?=
AWS_USER_NAME ?= ecr-puller
EATR_IMAGE ?= pmcgrath/eatr:latest
K8S_NAMESPACE ?= ci-cd


# Targets
default: k8s-deployment-up


k8s-prepare:
	@# Some YAML files are templates rather than manifests - Will want to use http://ksonnet.heptio.com/ and http://jsonnet.org/ in the future
	@# Prepare the k8s cluster for running the controller which manages the ECR authentication token renewals
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" namespace-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" service-account-template.yaml | kubectl apply -f -
	kubectl apply -f cluster-role.yaml
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" cluster-role-binding-template.yaml | kubectl apply -f -


k8s-create-aws-creds-secret:
	@# Create an AWS credential secret for the ECR account and region
	./create-eatr-aws-credentials-k8s-secret.sh "${AWS_ACCOUNT_ID}" "${AWS_REGION}" "${AWS_USER_NAME}" "${K8S_NAMESPACE}" "${AWS_ACCESS_KEY_ID}" "${AWS_SECRET_ACCESS_KEY}"


k8s-label-namespace:
	@# Label a namespace indicating it needs an ECR auth token secret to pull images from an ECR registry
	kubectl label namespace ${K8S_NAMESPACE} ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com="true"


k8s-deployment-up:
	@# Start the controller
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | kubectl apply -f -


k8s-deployment-down:
	@# Terminate the controller
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | kubectl delete -f -


k8s-delabel-namespace:
	@# Remove a namespace label indicating it no longer needs an ECR auth token secret to pull images from an ECR registry
	kubectl label namespace ${K8S_NAMESPACE} ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com-


k8s-delete-ecr-token-secret:
	@# Remove a namespace label indicating it no longer needs an ECR auth token secret to pull images from an ECR registry
	kubectl delete secret ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com --namespace ${K8S_NAMESPACE}


k8s-cleanup:
	@# Removed namespace
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" namespace-template.yaml | kubectl delete -f -
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" cluster-role-binding-template.yaml | kubectl delete -f -
	kubectl delete -f cluster-role.yaml
