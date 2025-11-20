package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"tranche/internal/cdn"
)

const graphqlEndpoint = "https://api.cloudflare.com/client/v4/graphql"

// Client queries Cloudflare's GraphQL analytics API for usage stats.
type Client struct {
	accountID string
	apiToken  string
	client    *http.Client
}

func NewClient(accountID, apiToken string) *Client {
	return &Client{
		accountID: accountID,
		apiToken:  apiToken,
		client:    http.DefaultClient,
	}
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type adaptiveResponse struct {
	Data struct {
		Viewer struct {
			Accounts []struct {
				HttpRequestsAdaptiveGroups []struct {
					Dimensions struct {
						DatetimeHour          time.Time `json:"datetimeHour"`
						ClientRequestHTTPHost string    `json:"clientRequestHTTPHost"`
					} `json:"dimensions"`
					Sum struct {
						Bytes int64 `json:"bytes"`
					} `json:"sum"`
				} `json:"httpRequestsAdaptiveGroups"`
			} `json:"accounts"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Usage fetches usage grouped by hour and hostname between [start, end).
// Cloudflare's adaptive dataset emits hourly windows, so window must be exactly 1h.
func (c *Client) Usage(ctx context.Context, start, end time.Time, window time.Duration, hosts []string) ([]cdn.WindowedUsage, error) {
	if window != time.Hour {
		return nil, fmt.Errorf("cloudflare only supports 1h windows; got %s", window)
	}

	query := `query usage($accountTag: String, $from: Time!, $to: Time!, $hosts: [String!]) {
  viewer {
    accounts(filter: {accountTag: $accountTag}) {
      httpRequestsAdaptiveGroups(
        filter: {datetime_geq: $from, datetime_lt: $to, clientRequestHTTPHost_in: $hosts},
        limit: 10000,
        orderBy: [datetimeHour_ASC]) {
        dimensions { datetimeHour clientRequestHTTPHost }
        sum { bytes }
      }
    }
  }
}`

	payload := gqlRequest{
		Query: query,
		Variables: map[string]any{
			"accountTag": c.accountID,
			"from":       start.Format(time.RFC3339),
			"to":         end.Format(time.RFC3339),
			"hosts":      hosts,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal graphql payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query cloudflare: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloudflare api status %d", resp.StatusCode)
	}

	var decoded adaptiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(decoded.Errors) > 0 {
		return nil, fmt.Errorf("cloudflare graphql error: %s", decoded.Errors[0].Message)
	}
	if len(decoded.Data.Viewer.Accounts) == 0 {
		return nil, fmt.Errorf("cloudflare account %s not found", c.accountID)
	}

	usages := make([]cdn.WindowedUsage, 0, len(decoded.Data.Viewer.Accounts[0].HttpRequestsAdaptiveGroups))
	for _, group := range decoded.Data.Viewer.Accounts[0].HttpRequestsAdaptiveGroups {
		start := group.Dimensions.DatetimeHour.Truncate(window)
		usages = append(usages, cdn.WindowedUsage{
			Host:        group.Dimensions.ClientRequestHTTPHost,
			WindowStart: start,
			WindowEnd:   start.Add(window),
			Bytes:       group.Sum.Bytes,
		})
	}

	return usages, nil
}
