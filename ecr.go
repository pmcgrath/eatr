package main

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/pkg/errors"
)

type ecrClient int

func newECRClient() ecrClient {
	return ecrClient(0)
}

func (e ecrClient) GetAuthToken(ctx context.Context) (*ecr.AuthorizationData, error) {
	svc := ecr.New(session.New())

	inp := &ecr.GetAuthorizationTokenInput{}
	out, err := svc.GetAuthorizationTokenWithContext(ctx, inp)
	if err != nil {
		return nil, errors.Wrap(err, "get ECR authorization token failed")
	}

	return out.AuthorizationData[0], nil
}
