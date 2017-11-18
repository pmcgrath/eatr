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
k8s-prepare:
	@# Will create all k8s content with the exception of the deployment for managing the ECR authentication token renewals
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" namespace-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" service-account-template.yaml | kubectl apply -f -
	echo cluster-role-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" cluster-role-binding-template.yaml | kubectl apply -f -


k8s-create-aws-creds-secret:
	./create-eatr-aws-credentials-k8s-secret.sh "${AWS_ACCOUNT_ID}" "${AWS_REGION}" "${AWS_USER_NAME}" "${K8S_NAMESPACE}" "${AWS_ACCESS_KEY_ID}" "${AWS_SECRET_ACCESS_KEY}"
	# Possibly add a cluster role rule for the secret ?


k8s-deployment-up:
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | kubectl apply -f -


k8s-deployment-down:
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | kubectl delete -f -