#!/bin/bash
# Assumes
#	AWS credentials configured allowing us make the "aws iam create-access-key" call and the region are configured, and we have privileges to make this call
#	Kubeconfig exists and has privileges to write the secret into the ci-cd namespace
# Will need awscli and kubectl
# Will overwrite existinig secret if it exists !!!!!!! You may run into the AWS max limit of 2 access keys !!!!!
#
set -euf -o pipefail


# Parameters
: ${1?"AWS region"}
: ${2?"AWS user name"}
: ${3?"K8S Namespace to create secret in"}
: ${4?"K8S AWS credentials secret name"}
aws_region=$1
aws_user_name=$2
namespace=$3
secret_name=$4


# See https://kubernetes.io/docs/concepts/configuration/secret/ for -n and -w0 explanations,
# also need AWK's printf to avoid the new line issue, see https://github.com/kubernetes/kubernetes/issues/23404
echo "Creating new AWS access key for $aws_user_name - Will fail if you already have 2 of these"
aws_access_key_data=$(aws iam create-access-key --user-name $aws_user_name --output text)
secret_aws_access_key_id=$(echo -n "$aws_access_key_data" | awk '{printf $2}' | base64 -w0)
secret_aws_secret_access_key=$(echo -n $aws_access_key_data | awk '{printf $4}' | base64 -w0)
secret_aws_region=$(echo -n $aws_region | base64 -w0)


# Going with a secret for now, should probably be an encrypted secret which is still in beta at this time, see https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/
echo "Creating or updating eatr aws credentials k8s secret [${secret_name}] in the [${namespace}] namespace - This will be mounted into the eatr pod and allow it to make the AWS ECR get authentication token call"
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ${secret_name}
  namespace: ${namespace}
data:
  aws_access_key_id: ${secret_aws_access_key_id}
  aws_region: ${secret_aws_region}
  aws_secret_access_key: ${secret_aws_secret_access_key}
EOF
