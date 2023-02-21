package handler

import (
	"context"

	"github.com/aws/aws-node-termination-handler/pkg/ec2metadata"
)

func NewAWSInterruptChecker() MetadataChecker {
	return &awsInterruptChecker{
		imds: ec2metadata.New("http://169.254.169.254", 3),
	}
}

type awsInterruptChecker struct {
	imds *ec2metadata.Service
}

func (c *awsInterruptChecker) CheckRebalanceRecommendation(_ context.Context) (bool, error) {
	rebalanceRecommendation, err := c.imds.GetRebalanceRecommendationEvent()
	if err != nil {
		return false, err
	}
	if rebalanceRecommendation == nil {
		return false, nil
	}
	return true, nil
}

func (c *awsInterruptChecker) CheckInterrupt(_ context.Context) (bool, error) {
	instanceAction, err := c.imds.GetSpotITNEvent()
	if instanceAction == nil && err == nil {
		// if there are no spot itns and no errors
		return false, nil
	}
	return true, err
}
