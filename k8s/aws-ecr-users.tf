# Have left terraform content which indicates how to create the user's with respective policies
# Backed out of using terrafrom here as our use case is too simple

# User whose regenerated ECR authentication token will be used by k8s to pull all ECR images for this accounts registry
resource "aws_iam_user" "ecr-puller" {
  name = "ecr-puller"
  path = "/"
}

# User used by CI system to create image repositories and push images
resource "aws_iam_user" "ecr-pusher" {
  name = "ecr-pusher"
  path = "/"
}

# Attach puller policy allowing reading from the registry
resource "aws_iam_user_policy_attachment" "ecr-puller" {
  user       = "${aws_iam_user.ecr-puller.name}"
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# Attach pusher policy allowing creating and pushing to the registry
resource "aws_iam_user_policy_attachment" "ecr-pusher" {
  user       = "${aws_iam_user.ecr-pusher.name}"
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser"
}
