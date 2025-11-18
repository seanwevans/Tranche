package storm

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"tranche/internal/db"
)

func TestEvaluatePolicyStartsStorm(t *testing.T) {
	store := newFakeStormStore()
	mv := &fakeMetricsView{avail: 0.4}
	eng := NewEngine(store, mv, fakeLogger{})
	now := time.Unix(1700000000, 0).UTC()
	eng.now = func() time.Time { return now }

	policy := db.StormPolicy{Kind: "failover", ThresholdAvail: 0.9, WindowSeconds: 60, CooldownSeconds: 300}
	if err := eng.evaluatePolicy(context.Background(), 1, policy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.inserts) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(store.inserts))
	}
}

func TestEvaluatePolicyHonorsCooldown(t *testing.T) {
	store := newFakeStormStore()
	mv := &fakeMetricsView{avail: 0.1}
	eng := NewEngine(store, mv, fakeLogger{})
	now := time.Unix(1700000000, 0).UTC()
	eng.now = func() time.Time { return now }

	key := store.key(1, "failover")
	store.last[key] = db.StormEvent{
		ID:        2,
		ServiceID: 1,
		Kind:      "failover",
		StartedAt: now.Add(-30 * time.Second),
		EndedAt:   sql.NullTime{Valid: true, Time: now.Add(-10 * time.Second)},
	}

	policy := db.StormPolicy{Kind: "failover", ThresholdAvail: 0.9, WindowSeconds: 60, CooldownSeconds: 60}
	if err := eng.evaluatePolicy(context.Background(), 1, policy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.inserts) != 0 {
		t.Fatalf("expected no inserts due to cooldown, got %d", len(store.inserts))
	}
}

func TestEvaluatePolicyResolvesStorm(t *testing.T) {
	store := newFakeStormStore()
	mv := &fakeMetricsView{avail: 0.99}
	eng := NewEngine(store, mv, fakeLogger{})
	now := time.Unix(1700000000, 0).UTC()
	eng.now = func() time.Time { return now }

	active := db.StormEvent{ID: 42, ServiceID: 1, Kind: "failover", StartedAt: now.Add(-5 * time.Minute)}
	store.active[store.key(1, "failover")] = active
	store.last[store.key(1, "failover")] = active

	policy := db.StormPolicy{Kind: "failover", ThresholdAvail: 0.9, WindowSeconds: 60, CooldownSeconds: 60}
	if err := eng.evaluatePolicy(context.Background(), 1, policy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.resolves) != 1 {
		t.Fatalf("expected 1 resolve, got %d", len(store.resolves))
	}
	if store.resolves[0].ID != active.ID {
		t.Fatalf("resolved wrong storm id: got %d want %d", store.resolves[0].ID, active.ID)
	}
}

type fakeMetricsView struct {
	avail float64
	err   error
}

func (f *fakeMetricsView) Availability(serviceID int64, window time.Duration) (float64, error) {
	return f.avail, f.err
}

type fakeLogger struct{}

func (fakeLogger) Printf(string, ...any) {}

type fakeStormStore struct {
	active   map[string]db.StormEvent
	last     map[string]db.StormEvent
	inserts  []db.InsertStormEventParams
	resolves []db.MarkStormEventResolvedParams
}

func newFakeStormStore() *fakeStormStore {
	return &fakeStormStore{
		active: make(map[string]db.StormEvent),
		last:   make(map[string]db.StormEvent),
	}
}

func (f *fakeStormStore) key(serviceID int64, kind string) string {
	return fmt.Sprintf("%d-%s", serviceID, kind)
}

func (f *fakeStormStore) GetActiveServices(ctx context.Context) ([]db.Service, error) {
	return nil, nil
}

func (f *fakeStormStore) GetStormPoliciesForService(ctx context.Context, serviceID int64) ([]db.StormPolicy, error) {
	return nil, nil
}

func (f *fakeStormStore) GetActiveStormForPolicy(ctx context.Context, arg db.GetActiveStormForPolicyParams) (db.StormEvent, error) {
	if storm, ok := f.active[f.key(arg.ServiceID, arg.Kind)]; ok {
		return storm, nil
	}
	return db.StormEvent{}, sql.ErrNoRows
}

func (f *fakeStormStore) GetLastStormEvent(ctx context.Context, arg db.GetLastStormEventParams) (db.StormEvent, error) {
	if storm, ok := f.last[f.key(arg.ServiceID, arg.Kind)]; ok {
		return storm, nil
	}
	return db.StormEvent{}, sql.ErrNoRows
}

func (f *fakeStormStore) InsertStormEvent(ctx context.Context, arg db.InsertStormEventParams) (db.StormEvent, error) {
	f.inserts = append(f.inserts, arg)
	storm := db.StormEvent{ID: int64(len(f.inserts)), ServiceID: arg.ServiceID, Kind: arg.Kind}
	f.active[f.key(arg.ServiceID, arg.Kind)] = storm
	f.last[f.key(arg.ServiceID, arg.Kind)] = storm
	return storm, nil
}

func (f *fakeStormStore) MarkStormEventResolved(ctx context.Context, arg db.MarkStormEventResolvedParams) (db.StormEvent, error) {
	f.resolves = append(f.resolves, arg)
	for key, storm := range f.active {
		if storm.ID == arg.ID {
			storm.EndedAt = sql.NullTime{Valid: true, Time: arg.EndedAt}
			f.active[key] = storm
			break
		}
	}
	return db.StormEvent{}, nil
}
