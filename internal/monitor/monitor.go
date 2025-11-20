package monitor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"tranche/internal/db"
)

type Logger interface {
	Printf(string, ...any)
}

type MetricsRecorder interface {
	RecordProbe(serviceID int64, target string, ok bool, latency time.Duration)
}

type AvailabilityProvider interface {
	Availability(serviceID int64, window time.Duration) float64
}

type MetricsView struct {
	rec AvailabilityProvider
}

func NewMetricsView(rec AvailabilityProvider) *MetricsView {
	return &MetricsView{rec: rec}
}

func (mv *MetricsView) Availability(serviceID int64, window time.Duration) (float64, error) {
	return mv.rec.Availability(serviceID, window), nil
}

type ProbeConfig struct {
	Path    string
	Timeout time.Duration
}

type Scheduler struct {
	db    *db.Queries
	m     MetricsRecorder
	log   Logger
	cfg   ProbeConfig
	mu    sync.Mutex
	loops map[string]context.CancelFunc
}

func NewScheduler(dbx *db.Queries, mr MetricsRecorder, log Logger, cfg ProbeConfig) *Scheduler {
	return &Scheduler{db: dbx, m: mr, log: log, cfg: cfg, loops: make(map[string]context.CancelFunc)}
}

func (s *Scheduler) Run(ctx context.Context) {
	client := &http.Client{Timeout: s.probeTimeout()}
	defer s.cancelAllLoops()

	for {
		services, err := s.db.GetActiveServices(ctx)
		if err != nil {
			s.log.Printf("GetActiveServices: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}

		active := make(map[string]struct{})
		for _, svc := range services {
			targets, err := s.targetsForService(ctx, svc)
			if err != nil {
				s.log.Printf("GetServiceDomains(service=%d): %v", svc.ID, err)
				s.preserveExistingLoops(active, svc.ID)
				continue
			}
			for _, target := range targets {
				active[target.key()] = struct{}{}
				s.ensureProbeLoop(ctx, client, target)
			}
		}
		s.stopMissingLoops(active)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

func (s *Scheduler) ensureProbeLoop(ctx context.Context, client *http.Client, target probeTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := target.key()
	if _, ok := s.loops[key]; ok {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	s.loops[key] = cancel
	go s.probeLoop(loopCtx, client, target)
}

func (s *Scheduler) stopMissingLoops(active map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.loops {
		if _, ok := active[id]; ok {
			continue
		}
		cancel()
		delete(s.loops, id)
	}
}

func (s *Scheduler) preserveExistingLoops(active map[string]struct{}, serviceID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := fmt.Sprintf("%d:", serviceID)
	for id := range s.loops {
		if strings.HasPrefix(id, prefix) {
			active[id] = struct{}{}
		}
	}
}

func (s *Scheduler) cancelAllLoops() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.loops {
		cancel()
		delete(s.loops, id)
	}
}

func (s *Scheduler) probeLoop(ctx context.Context, client *http.Client, target probeTarget) {
	for {
		start := time.Now()
		ok := false
		if resp, err := s.doProbe(ctx, client, target); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				ok = true
			}
		} else {
			s.log.Printf("probe target=%s: %v", target.metricsKey, err)
		}
		lat := time.Since(start)
		s.m.RecordProbe(target.serviceID, target.metricsKey, ok, lat)

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (s *Scheduler) doProbe(ctx context.Context, client *http.Client, target probeTarget) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.url, nil)
	if err != nil {
		return nil, err
	}
	if target.hostHeader != "" {
		req.Host = target.hostHeader
	}
	return client.Do(req)
}

func (s *Scheduler) targetsForService(ctx context.Context, svc db.Service) ([]probeTarget, error) {
	domains, err := s.db.GetServiceDomains(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	var targets []probeTarget
	for _, domain := range domains {
		// direct domain probe
		if t, ok := s.buildTarget(svc.ID, domain.ID, domain.Name, domain.Name, ""); ok {
			targets = append(targets, t)
		}
		if svc.PrimaryCdn != "" {
			label := fmt.Sprintf("primary:%s", svc.PrimaryCdn)
			if t, ok := s.buildTarget(svc.ID, domain.ID, domain.Name, svc.PrimaryCdn, label); ok {
				targets = append(targets, t)
			}
		}
		if svc.BackupCdn != "" {
			label := fmt.Sprintf("backup:%s", svc.BackupCdn)
			if t, ok := s.buildTarget(svc.ID, domain.ID, domain.Name, svc.BackupCdn, label); ok {
				targets = append(targets, t)
			}
		}
	}
	return targets, nil
}

func (s *Scheduler) buildTarget(serviceID, domainID int64, domainName, host, label string) (probeTarget, bool) {
	urlStr := buildProbeURL(host, s.probePath())
	if urlStr == "" {
		return probeTarget{}, false
	}
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return probeTarget{}, false
	}
	metricsLabel := domainName
	if label != "" {
		metricsLabel = fmt.Sprintf("%s@%s", domainName, label)
	}
	hostHeader := ""
	if !strings.EqualFold(parsed.Hostname(), domainName) {
		hostHeader = domainName
	}
	return probeTarget{
		serviceID:  serviceID,
		domainID:   domainID,
		domainName: domainName,
		url:        urlStr,
		hostHeader: hostHeader,
		metricsKey: metricsLabel,
	}, true
}

func (s *Scheduler) probeTimeout() time.Duration {
	if s.cfg.Timeout <= 0 {
		return 5 * time.Second
	}
	return s.cfg.Timeout
}

func (s *Scheduler) probePath() string {
	path := s.cfg.Path
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func buildProbeURL(host, probePath string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	parsed, err := url.Parse(host)
	if err != nil {
		return ""
	}
	parsed.Path = probePath
	return parsed.String()
}

type probeTarget struct {
	serviceID  int64
	domainID   int64
	domainName string
	url        string
	hostHeader string
	metricsKey string
}

func (t probeTarget) key() string {
	return fmt.Sprintf("%d:%d:%s", t.serviceID, t.domainID, t.metricsKey)
}
