# Switch to bash
# SHELL=/bin/bash


# Defaults
AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --output text --query 'Account')
K8S_AWS_CREDS_SECRET_NAME ?= eatr-aws-credentials
AWS_PROFILE ?= tppoc-pmcgrath-admin
AWS_REGION ?= eu-west-1
AWS_USER_NAME ?= ecr-puller
EATR_IMAGE ?= pmcgrath/eatr:1.0
NAMESPACE ?= ci-cd


# Derived
AWS_REGISTRY_DOMAIN = ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com


# Targets
aws-create-user-plan:
	terraform init
	terraform plan


aws-create-user-apply:
	@# Will create ECR AWS user
	terraform apply


k8s-prepare:
	@# Will create all k8s content with the exception of the deployment for managing the ECR authentication token renewals
	sed -e "s/{{NAMESPACE}}/${NAMESPACE}/" namespace-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${NAMESPACE}/" service-account-template.yaml | kubectl apply -f -
	sed -e "s/{{ECR_IMAGE_PULL_SECRET_NAME}}/${AWS_REGISTRY_DOMAIN}/" cluster-role-template.yaml | kubectl apply -f -
	sed -e "s/{{NAMESPACE}}/${NAMESPACE}/" cluster-role-binding-template.yaml | kubectl apply -f -
	./create-eatr-aws-credentials-k8s-secret.sh ${AWS_REGION} ${AWS_USER_NAME} ${NAMESPACE} ${K8S_AWS_CREDS_SECRET_NAME}


k8s-deployment-up:
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | sed -e "s/{{AWS_CREDS_SECRET_NAME}}/${K8S_AWS_CREDS_SECRET_NAME}/g" | kubectl apply -f -


k8s-deployment-down:
	cat deployment-template.yaml | sed -e "s/{{NAMESPACE}}/${NAMESPACE}/" | sed -e "s|{{EATR_IMAGE}}|${EATR_IMAGE}|" | sed -e "s/{{AWS_CREDS_SECRET_NAME}}/${K8S_AWS_CREDS_SECRET_NAME}/g" | kubectl delete -f -
