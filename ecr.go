package main

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/pkg/errors"
)

// Subset so we can test, we can fake a subset of ECR
type ecrClient int

func newECRClient() ecrClient {
	return ecrClient(0)
}

// Need to support multiple ECR repos so we cannot relay on normal env vars or config file, hence the region id and secret args
func (e ecrClient) GetAuthToken(ctx context.Context, region, id, secret string) (*ecr.AuthorizationData, error) {
	creds := credentials.NewStaticCredentials(id, secret, "")
	config := aws.NewConfig().WithCredentials(creds).WithRegion(region)
	sess, _ := session.NewSession(config)
	svc := ecr.New(sess)

	inp := &ecr.GetAuthorizationTokenInput{}
	out, err := svc.GetAuthorizationTokenWithContext(ctx, inp)
	if err != nil {
		return nil, errors.Wrap(err, "get ECR authorization token failed")
	}

	return out.AuthorizationData[0], nil
}
