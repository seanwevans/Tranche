package dns

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Route53ProviderConfig captures the configuration necessary to talk to Route53.
type Route53ProviderConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	MaxAttempts     int
}

// route53API captures the subset of the AWS SDK we use so it can be mocked in tests.
type route53API interface {
	ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

// Route53Provider implements Provider backed by AWS Route53.
type Route53Provider struct {
	log         Logger
	client      route53API
	zoneCache   map[string]string
	cacheMu     sync.RWMutex
	maxAttempts int
	sleepFn     func(time.Duration)
}

// NewRoute53Provider builds a Route53-backed provider from AWS configuration.
func NewRoute53Provider(ctx context.Context, log Logger, cfg Route53ProviderConfig) (*Route53Provider, error) {
	if cfg.Region == "" {
		return nil, errors.New("route53 region is required")
	}

	loadOpts := []func(*awscfg.LoadOptions) error{awscfg.WithRegion(cfg.Region)}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		loadOpts = append(loadOpts, awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)))
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := route53.NewFromConfig(awsCfg)
	return newRoute53Provider(log, client, cfg), nil
}

func newRoute53Provider(log Logger, client route53API, cfg Route53ProviderConfig) *Route53Provider {
	attempts := cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	return &Route53Provider{
		log:         log,
		client:      client,
		zoneCache:   make(map[string]string),
		maxAttempts: attempts,
		sleepFn:     time.Sleep,
	}
}

// SetWeights updates the weighted DNS entries for a domain.
func (p *Route53Provider) SetWeights(domain string, primaryWeight, backupWeight int) error {
	if strings.TrimSpace(domain) == "" {
		return errors.New("domain is required")
	}

	normalizedDomain := strings.TrimSuffix(domain, ".")
	var lastErr error
	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		ctx := context.Background()
		if err := p.setWeightsOnce(ctx, normalizedDomain, primaryWeight, backupWeight); err != nil {
			lastErr = err
			p.log.Printf("route53 SetWeights attempt %d/%d for %s failed: %v", attempt, p.maxAttempts, normalizedDomain, err)
			if attempt < p.maxAttempts {
				backoff := time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond
				p.sleepFn(backoff)
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("route53 SetWeights(%s) failed: %w", normalizedDomain, lastErr)
}

func (p *Route53Provider) setWeightsOnce(ctx context.Context, domain string, primaryWeight, backupWeight int) error {
	zoneID, err := p.lookupHostedZone(ctx, domain)
	if err != nil {
		return err
	}

	primary, backup, err := p.fetchWeightedRecords(ctx, zoneID, domain)
	if err != nil {
		return err
	}

	primaryUpdate := cloneRecordSet(primary)
	backupUpdate := cloneRecordSet(backup)
	primaryUpdate.Weight = aws.Int64(int64(primaryWeight))
	backupUpdate.Weight = aws.Int64(int64(backupWeight))

	_, err = p.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &route53types.ChangeBatch{
			Comment: aws.String(fmt.Sprintf("tranche weight update %s", time.Now().UTC().Format(time.RFC3339))),
			Changes: []route53types.Change{
				{Action: route53types.ChangeActionUpsert, ResourceRecordSet: primaryUpdate},
				{Action: route53types.ChangeActionUpsert, ResourceRecordSet: backupUpdate},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("change record sets: %w", err)
	}

	return nil
}

func (p *Route53Provider) lookupHostedZone(ctx context.Context, domain string) (string, error) {
	p.cacheMu.RLock()
	if id, ok := p.zoneCache[domain]; ok {
		p.cacheMu.RUnlock()
		return id, nil
	}
	p.cacheMu.RUnlock()

	resp, err := p.client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{DNSName: aws.String(domain)})
	if err != nil {
		return "", fmt.Errorf("list hosted zones: %w", err)
	}

	var (
		bestID   string
		bestName string
	)
	for _, zone := range resp.HostedZones {
		zoneName := strings.TrimSuffix(aws.ToString(zone.Name), ".")
		if zoneName == "" {
			continue
		}
		if !strings.HasSuffix(domain, zoneName) {
			continue
		}
		if len(zoneName) > len(bestName) {
			bestName = zoneName
			bestID = strings.TrimPrefix(aws.ToString(zone.Id), "/hostedzone/")
		}
	}

	if bestID == "" {
		return "", fmt.Errorf("no hosted zone for %s", domain)
	}

	p.cacheMu.Lock()
	p.zoneCache[domain] = bestID
	p.cacheMu.Unlock()

	return bestID, nil
}

func (p *Route53Provider) fetchWeightedRecords(ctx context.Context, zoneID, domain string) (*route53types.ResourceRecordSet, *route53types.ResourceRecordSet, error) {
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordName: aws.String(domain),
	}

	var primary, backup *route53types.ResourceRecordSet
	for {
		resp, err := p.client.ListResourceRecordSets(ctx, input)
		if err != nil {
			return nil, nil, fmt.Errorf("list record sets: %w", err)
		}

		for i := range resp.ResourceRecordSets {
			rr := resp.ResourceRecordSets[i]
			name := strings.TrimSuffix(aws.ToString(rr.Name), ".")
			if name != domain {
				continue
			}
			if rr.SetIdentifier == nil || rr.Weight == nil {
				continue
			}
			switch strings.ToLower(aws.ToString(rr.SetIdentifier)) {
			case "primary":
				copy := rr
				primary = &copy
			case "backup":
				copy := rr
				backup = &copy
			}
		}

		if primary != nil && backup != nil {
			break
		}

		if !resp.IsTruncated {
			break
		}

		input.StartRecordName = resp.NextRecordName
		input.StartRecordType = resp.NextRecordType
		input.StartRecordIdentifier = resp.NextRecordIdentifier
	}

	if primary == nil || backup == nil {
		return nil, nil, fmt.Errorf("weighted records for %s not found", domain)
	}

	return primary, backup, nil
}

func cloneRecordSet(in *route53types.ResourceRecordSet) *route53types.ResourceRecordSet {
	if in == nil {
		return nil
	}
	copy := *in
	if in.ResourceRecords != nil {
		copy.ResourceRecords = append([]route53types.ResourceRecord{}, in.ResourceRecords...)
	}
	return &copy
}
