package cdn

import (
	"context"
	"testing"
	"time"

	"tranche/internal/db"
)

type fakeProvider struct{ name string }

func (f fakeProvider) Name() string { return f.name }

func (fakeProvider) FetchUsage(ctx context.Context, svc db.Service, since, until time.Time) (int64, int64, error) {
	return 0, 0, nil
}

func TestSelectorPrecedence(t *testing.T) {
	selector, err := NewSelector(SelectorConfig{
		DefaultProvider:   "default",
		CustomerOverrides: map[int64]string{1: "customer"},
		ServiceOverrides:  map[int64]string{2: "service"},
		Providers: []UsageProvider{
			fakeProvider{name: "default"},
			fakeProvider{name: "customer"},
			fakeProvider{name: "service"},
			fakeProvider{name: "primary"},
		},
	})
	if err != nil {
		t.Fatalf("selector init: %v", err)
	}

	cases := []struct {
		name     string
		svc      db.Service
		expected string
	}{
		{name: "default", svc: db.Service{ID: 3}, expected: "default"},
		{name: "primary", svc: db.Service{ID: 3, PrimaryCdn: "primary"}, expected: "primary"},
		{name: "customer override", svc: db.Service{ID: 3, CustomerID: 1, PrimaryCdn: "primary"}, expected: "customer"},
		{name: "service override", svc: db.Service{ID: 2, CustomerID: 1, PrimaryCdn: "primary"}, expected: "service"},
	}

	for _, tc := range cases {
		prov, err := selector.ProviderForService(tc.svc)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if prov.Name() != tc.expected {
			t.Fatalf("%s: expected %s got %s", tc.name, tc.expected, prov.Name())
		}
	}
}
