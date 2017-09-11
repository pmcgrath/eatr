# User whose regenerated ECR authentication token will be used by k8s to pull all ECR images for this accounts registry
resource "aws_iam_user" "ecr-puller" {
  name = "ecr-puller"
  path = "/"
}

# Attach policy allowing reading from the registry - Allows pulls
resource "aws_iam_user_policy_attachment" "ecr-puller" {
  user       = "${aws_iam_user.ecr-puller.name}"
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}
