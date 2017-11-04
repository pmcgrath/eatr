#!/bin/bash
# Assumes
#	If AWS access key's not passed AWS credentials are configured allowing us make the "aws iam create-access-key" call and the region are configured
#	Kubeconfig exists and has privileges to write the secret into the ci-cd namespace
# Will need awscli and kubectl
# Will overwrite existing secret if it exists !!!!!!! You may run into the AWS max limit of 2 access keys !!!!!
#
set -euf -o pipefail


# Parameters - Some are optional
: ${1?"AWS region"}
: ${2?"AWS user name"}
: ${3?"K8S namespace to create secret in"}
: ${4?"K8S AWS credentials secret name"}
aws_region=$1
aws_user_name=$2
k8s_namespace=$3
k8s_secret_name=$4
[[ $# -gt 4 ]] && aws_access_key_id=$5 || aws_access_key_id=
[[ $# -gt 5 ]] && aws_secret_access_key=$6 || aws_secret_access_key=

# Cater for optional parameters
if [[ -z $aws_access_key_id ]] || [[ -z $aws_secret_access_key ]]; then 
	# Need to create the access key - assumes current user has privileges to do this
	# See https://kubernetes.io/docs/concepts/configuration/secret/ for -n and -w0 explanations
	# Also need AWK's printf to avoid the new line issue, see https://github.com/kubernetes/kubernetes/issues/23404
	echo "Creating new AWS access key for $aws_user_name - Will fail if you already have 2 of these"
	aws_access_key_data=$(aws iam create-access-key --user-name $aws_user_name --output text)
	secret_aws_access_key_id=$(echo -n $aws_access_key_data | awk '{printf $2}' | base64 -w0)
	secret_aws_secret_access_key=$(echo -n $aws_access_key_data | awk '{printf $4}' | base64 -w0)
else
	secret_aws_access_key_id=$(echo -n $aws_access_key_id | base64 -w0)
	secret_aws_secret_access_key=$(echo -n $aws_secret_access_key | base64 -w0)
fi

# AWS region
secret_aws_region=$(echo -n $aws_region | base64 -w0)

# Going with a secret for now, should probably be an encrypted secret which is still in beta at this time, see https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/
echo "Creating or updating eatr aws credentials k8s secret [${k8s_secret_name}] in the [${k8s_namespace}] namespace - This will be mounted into the eatr pod and allow it to make the AWS ECR get authentication token call"
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ${k8s_secret_name}
  namespace: ${k8s_namespace}
data:
  aws_access_key_id: ${secret_aws_access_key_id}
  aws_region: ${secret_aws_region}
  aws_secret_access_key: ${secret_aws_secret_access_key}
EOF
