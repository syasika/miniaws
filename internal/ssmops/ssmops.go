package ssmops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type Parameter struct {
	Name           string
	Type           string
	Value          string
	LastModified   time.Time
	Version        int64
}

type Page struct {
	Parameters []Parameter
	NextToken  *string
}

func IsConnectionErr(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "dial tcp")
}

func useFriendlyErr(err error) error {
	if err == nil {
		return nil
	}
	if IsConnectionErr(err) {
		return fmt.Errorf("cannot reach ministack — is the container running?")
	}
	return friendlyErr(err)
}

func friendlyErr(err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "api error ") {
		parts := strings.SplitN(errStr, "api error ", 2)
		if len(parts) == 2 {
			return fmt.Errorf("SSM API error: %s", strings.ToLower(strings.TrimSpace(parts[1])))
		}
	}
	return err
}

func ListAllParameters(ctx context.Context, client *ssm.Client) ([]Parameter, error) {
	var params []Parameter
	paginator := ssm.NewDescribeParametersPaginator(client, &ssm.DescribeParametersInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, useFriendlyErr(err)
		}
		for _, p := range page.Parameters {
			params = append(params, toParam(p))
		}
	}
	return params, nil
}

func ListParameters(ctx context.Context, client *ssm.Client, nextToken *string, maxResults int32) (*Page, error) {
	input := &ssm.DescribeParametersInput{
		MaxResults: aws.Int32(maxResults),
		NextToken:  nextToken,
	}
	resp, err := client.DescribeParameters(ctx, input)
	if err != nil {
		return nil, useFriendlyErr(err)
	}
	params := make([]Parameter, len(resp.Parameters))
	for i, p := range resp.Parameters {
		params[i] = toParam(p)
	}
	return &Page{Parameters: params, NextToken: resp.NextToken}, nil
}

func toParam(p types.ParameterMetadata) Parameter {
	return Parameter{
		Name:         *p.Name,
		Type:         string(p.Type),
		LastModified: *p.LastModifiedDate,
		Version:     p.Version,
	}
}

func GetParameter(ctx context.Context, client *ssm.Client, name string) (*Parameter, error) {
	resp, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(false),
	})
	if err != nil {
		return nil, useFriendlyErr(err)
	}
	p := resp.Parameter
	return &Parameter{
		Name:         *p.Name,
		Type:         string(p.Type),
		Value:        *p.Value,
		LastModified: *p.LastModifiedDate,
		Version:     p.Version,
	}, nil
}

func PutParameter(ctx context.Context, client *ssm.Client, name, value, paramType string) error {
	_, err := client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String(name),
		Value: aws.String(value),
		Type:  types.ParameterType(paramType),
	})
	return useFriendlyErr(err)
}

func DeleteParameter(ctx context.Context, client *ssm.Client, name string) error {
	_, err := client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(name),
	})
	return useFriendlyErr(err)
}
