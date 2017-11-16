# Switch to bash
# SHELL=/bin/bash


# Defaults
AWS_ACCESS_KEY_ID ?=
AWS_ACCOUNT_ID ?=
AWS_REGION ?= eu-west-1
AWS_SECRET_ACCESS_KEY ?=
AWS_USER_NAME ?= ecr-puller
EATR_IMAGE ?= pmcgrath/eatr:latest
K8S_AWS_CREDS_SECRET_NAME ?= eatr-aws-credentials
K8S_ECR_SECRET_NAME_PATTERN ?= '*.dkr.ecr.*.amazonaws.com'
K8S_NAMESPACE ?= ci-cd


# Derived
ifeq "${AWS_ACCOUNT_ID}" ""
	AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --output text --query 'Account')
endif
AWS_REGISTRY_DOMAIN = ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com


# Targets
k8s-prepare:
	@# Will create all k8s content with the exception of the deployment for managing the ECR authentication token renewals
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" namespace-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" service-account-template.yaml | kubectl apply -f -
	sed -e "s/{{ECR_IMAGE_PULL_SECRET_NAME}}/${AWS_REGISTRY_DOMAIN}/" cluster-role-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" cluster-role-binding-template.yaml | kubectl apply -f -


k8s-create-aws-creds-decret:
	./create-eatr-aws-credentials-k8s-secret.sh ${AWS_REGION} ${AWS_USER_NAME} ${K8S_NAMESPACE} ${K8S_AWS_CREDS_SECRET_NAME} ${AWS_ACCESS_KEY_ID} ${AWS_SECRET_ACCESS_KEY}


k8s-deployment-up:
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | sed -e "s/{{AWS_CREDS_SECRET_NAME}}/${K8S_AWS_CREDS_SECRET_NAME}/g" | kubectl apply -f -


k8s-deployment-down:
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${K8S_NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | sed -e "s/{{AWS_CREDS_SECRET_NAME}}/${K8S_AWS_CREDS_SECRET_NAME}/g" | kubectl delete -f -
