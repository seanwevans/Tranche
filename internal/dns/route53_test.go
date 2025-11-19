package dns

import (
	"context"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

type mockRoute53Client struct {
	listZonesFn    func(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	listRecordsFn  func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	changeRecordFn func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

func (m *mockRoute53Client) ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	return m.listZonesFn(ctx, params, optFns...)
}

func (m *mockRoute53Client) ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	return m.listRecordsFn(ctx, params, optFns...)
}

func (m *mockRoute53Client) ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	return m.changeRecordFn(ctx, params, optFns...)
}

func discardLogger() Logger {
	return log.New(testWriter{}, "", 0)
}

type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestRoute53ProviderSetWeights(t *testing.T) {
	mock := &mockRoute53Client{}
	mock.listZonesFn = func(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
		return &route53.ListHostedZonesByNameOutput{
			HostedZones: []route53types.HostedZone{
				{Name: aws.String("example.com."), Id: aws.String("/hostedzone/Z123")},
			},
		}, nil
	}

	rrPrimary := route53types.ResourceRecordSet{
		Name:          aws.String("app.example.com."),
		Type:          route53types.RRTypeCname,
		SetIdentifier: aws.String("primary"),
		Weight:        aws.Int64(10),
		TTL:           aws.Int64(60),
		ResourceRecords: []route53types.ResourceRecord{
			{Value: aws.String("primary.example.net.")},
		},
	}
	rrBackup := route53types.ResourceRecordSet{
		Name:          aws.String("app.example.com."),
		Type:          route53types.RRTypeCname,
		SetIdentifier: aws.String("backup"),
		Weight:        aws.Int64(5),
		TTL:           aws.Int64(60),
		ResourceRecords: []route53types.ResourceRecord{
			{Value: aws.String("backup.example.net.")},
		},
	}

	mock.listRecordsFn = func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
		return &route53.ListResourceRecordSetsOutput{
			ResourceRecordSets: []route53types.ResourceRecordSet{rrPrimary, rrBackup},
		}, nil
	}

	var captured *route53.ChangeResourceRecordSetsInput
	mock.changeRecordFn = func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
		captured = params
		return &route53.ChangeResourceRecordSetsOutput{}, nil
	}

	provider := newRoute53Provider(discardLogger(), mock, Route53ProviderConfig{MaxAttempts: 1})

	if err := provider.SetWeights("app.example.com", 50, 10); err != nil {
		t.Fatalf("SetWeights returned error: %v", err)
	}

	if captured == nil {
		t.Fatalf("expected change request to be sent")
	}

	if got := aws.ToInt64(captured.ChangeBatch.Changes[0].ResourceRecordSet.Weight); got != 50 {
		t.Fatalf("expected primary weight 50, got %d", got)
	}
	if got := aws.ToInt64(captured.ChangeBatch.Changes[1].ResourceRecordSet.Weight); got != 10 {
		t.Fatalf("expected backup weight 10, got %d", got)
	}
}

func TestRoute53ProviderRetriesFailures(t *testing.T) {
	mock := &mockRoute53Client{}
	mock.listZonesFn = func(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
		return &route53.ListHostedZonesByNameOutput{
			HostedZones: []route53types.HostedZone{{Name: aws.String("example.com."), Id: aws.String("/hostedzone/Z123")}},
		}, nil
	}

	rrPrimary := route53types.ResourceRecordSet{Name: aws.String("app.example.com."), Type: route53types.RRTypeCname, SetIdentifier: aws.String("primary"), Weight: aws.Int64(1), TTL: aws.Int64(60)}
	rrBackup := route53types.ResourceRecordSet{Name: aws.String("app.example.com."), Type: route53types.RRTypeCname, SetIdentifier: aws.String("backup"), Weight: aws.Int64(1), TTL: aws.Int64(60)}

	mock.listRecordsFn = func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
		return &route53.ListResourceRecordSetsOutput{ResourceRecordSets: []route53types.ResourceRecordSet{rrPrimary, rrBackup}}, nil
	}

	attempts := 0
	mock.changeRecordFn = func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary error")
		}
		return &route53.ChangeResourceRecordSetsOutput{}, nil
	}

	provider := newRoute53Provider(discardLogger(), mock, Route53ProviderConfig{MaxAttempts: 2})
	provider.sleepFn = func(d time.Duration) {}

	if err := provider.SetWeights("app.example.com", 10, 5); err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}

	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}
