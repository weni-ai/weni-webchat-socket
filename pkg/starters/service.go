package starters

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lambda/lambdaiface"
)

// StartersInput contains the product data forwarded to the Lambda function.
type StartersInput struct {
	Account     string            `json:"account"`
	LinkText    string            `json:"linkText"`
	ProductName string            `json:"productName,omitempty"`
	Description string            `json:"description,omitempty"`
	Brand       string            `json:"brand,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// StartersOutput contains the conversation starter questions returned by the Lambda.
type StartersOutput struct {
	Questions []string `json:"questions"`
}

// StartersService abstracts the generation of conversation starter questions.
type StartersService interface {
	GetStarters(ctx context.Context, input StartersInput) (*StartersOutput, error)
}

// LambdaStartersService invokes an AWS Lambda function to generate starters.
type LambdaStartersService struct {
	client      lambdaiface.LambdaAPI
	functionARN string
}

// NewLambdaStartersService creates a service that invokes the given Lambda ARN.
func NewLambdaStartersService(client lambdaiface.LambdaAPI, functionARN string) *LambdaStartersService {
	return &LambdaStartersService{
		client:      client,
		functionARN: functionARN,
	}
}

// GetStarters invokes the Lambda synchronously (RequestResponse) and returns
// the generated questions. The provided context controls the invocation timeout.
func (s *LambdaStartersService) GetStarters(ctx context.Context, input StartersInput) (*StartersOutput, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("starters: failed to marshal input: %w", err)
	}

	invokeInput := &lambda.InvokeInput{
		FunctionName:   aws.String(s.functionARN),
		Payload:        payload,
		InvocationType: aws.String("RequestResponse"),
	}

	result, err := s.client.InvokeWithContext(ctx, invokeInput)
	if err != nil {
		return nil, fmt.Errorf("starters: lambda invocation failed: %w", err)
	}

	if result.FunctionError != nil {
		return nil, fmt.Errorf("starters: lambda function error: %s", aws.StringValue(result.FunctionError))
	}

	var output StartersOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		return nil, fmt.Errorf("starters: failed to unmarshal lambda response: %w", err)
	}

	if len(output.Questions) == 0 {
		return nil, fmt.Errorf("starters: lambda returned no questions")
	}

	return &output, nil
}
