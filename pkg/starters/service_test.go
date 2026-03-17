package starters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lambda/lambdaiface"
	"github.com/stretchr/testify/assert"
)

type mockLambdaClient struct {
	lambdaiface.LambdaAPI
	invokeFunc func(ctx context.Context, input *lambda.InvokeInput) (*lambda.InvokeOutput, error)
}

func (m *mockLambdaClient) InvokeWithContext(ctx context.Context, input *lambda.InvokeInput, _ ...request.Option) (*lambda.InvokeOutput, error) {
	return m.invokeFunc(ctx, input)
}

func TestGetStarters_HappyPath(t *testing.T) {
	questions := []string{"Q1?", "Q2?", "Q3?"}
	respPayload, _ := json.Marshal(StartersOutput{Questions: questions})

	client := &mockLambdaClient{
		invokeFunc: func(_ context.Context, input *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
			assert.Equal(t, "arn:test", aws.StringValue(input.FunctionName))

			var received StartersInput
			err := json.Unmarshal(input.Payload, &received)
			assert.NoError(t, err)
			assert.Equal(t, "test-account", received.Account)
			assert.Equal(t, "test-slug", received.LinkText)

			return &lambda.InvokeOutput{
				StatusCode: aws.Int64(200),
				Payload:    respPayload,
			}, nil
		},
	}

	svc := NewLambdaStartersService(client, "arn:test")
	out, err := svc.GetStarters(context.Background(), StartersInput{
		Account:  "test-account",
		LinkText: "test-slug",
	})
	assert.NoError(t, err)
	assert.Equal(t, questions, out.Questions)
}

func TestGetStarters_FunctionError(t *testing.T) {
	client := &mockLambdaClient{
		invokeFunc: func(_ context.Context, _ *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				StatusCode:    aws.Int64(200),
				FunctionError: aws.String("Unhandled"),
				Payload:       []byte(`{"errorMessage":"something broke"}`),
			}, nil
		},
	}

	svc := NewLambdaStartersService(client, "arn:test")
	out, err := svc.GetStarters(context.Background(), StartersInput{Account: "a", LinkText: "b"})
	assert.Nil(t, out)
	assert.ErrorContains(t, err, "lambda function error")
}

func TestGetStarters_InvalidJSON(t *testing.T) {
	client := &mockLambdaClient{
		invokeFunc: func(_ context.Context, _ *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				StatusCode: aws.Int64(200),
				Payload:    []byte(`not json`),
			}, nil
		},
	}

	svc := NewLambdaStartersService(client, "arn:test")
	out, err := svc.GetStarters(context.Background(), StartersInput{Account: "a", LinkText: "b"})
	assert.Nil(t, out)
	assert.ErrorContains(t, err, "failed to unmarshal")
}

func TestGetStarters_EmptyQuestions(t *testing.T) {
	respPayload, _ := json.Marshal(StartersOutput{Questions: []string{}})
	client := &mockLambdaClient{
		invokeFunc: func(_ context.Context, _ *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				StatusCode: aws.Int64(200),
				Payload:    respPayload,
			}, nil
		},
	}

	svc := NewLambdaStartersService(client, "arn:test")
	out, err := svc.GetStarters(context.Background(), StartersInput{Account: "a", LinkText: "b"})
	assert.Nil(t, out)
	assert.ErrorContains(t, err, "no questions")
}

func TestGetStarters_MissingQuestionsField(t *testing.T) {
	client := &mockLambdaClient{
		invokeFunc: func(_ context.Context, _ *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				StatusCode: aws.Int64(200),
				Payload:    []byte(`{"other": "field"}`),
			}, nil
		},
	}

	svc := NewLambdaStartersService(client, "arn:test")
	out, err := svc.GetStarters(context.Background(), StartersInput{Account: "a", LinkText: "b"})
	assert.Nil(t, out)
	assert.ErrorContains(t, err, "no questions")
}

func TestGetStarters_InvocationError(t *testing.T) {
	client := &mockLambdaClient{
		invokeFunc: func(_ context.Context, _ *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
			return nil, assert.AnError
		},
	}

	svc := NewLambdaStartersService(client, "arn:test")
	out, err := svc.GetStarters(context.Background(), StartersInput{Account: "a", LinkText: "b"})
	assert.Nil(t, out)
	assert.ErrorContains(t, err, "lambda invocation failed")
}
