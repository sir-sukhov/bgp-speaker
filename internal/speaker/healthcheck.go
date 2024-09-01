package speaker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/osrg/gobgp/v3/pkg/log"
)

const (
	healthyThreshold = 3
	interval         = 1
	timeoutSeconds   = 1
)

const (
	Unhealthy Status = iota
	Healthy
)

type Status int

func (s Status) String() string {
	if s == Healthy {
		return "Healthy"
	}
	return "Unhealthy"
}

// HealthCheck проверяет статус сервиса 1 раз в секунду.
type HealthCheck struct {
	status      Status
	u           *url.URL
	okCounter   int
	client      *http.Client
	cbHealthy   func(context.Context) error
	cbUnhealthy func(context.Context) error
}

// NewHealthCheck создает новый HealthCheck, который после запуска HealthCheck.Run:
//   - выполняет cbHealthy call back, eсли статус меняется на healthy
//   - выполняет cbUnhealthy call back, eсли статус меняется на unhealthy
//   - ничего не делает, если статус не меняется
func NewHealthCheck(cbHealthy, cbUnhealthy func(context.Context) error, rawURL string) (*HealthCheck, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("HealthCheck: parse url error: %w", err)
	}
	return &HealthCheck{
		status: Unhealthy,
		u:      u,
		client: &http.Client{
			Timeout: time.Second * timeoutSeconds,
		},
		cbHealthy:   cbHealthy,
		cbUnhealthy: cbUnhealthy,
	}, nil
}

func (hc *HealthCheck) Run(ctx context.Context, logger Logger) error {
	if hc.u.String() == "" {
		logger.Warn("HealthCheck URL is empty", nil)
		<-ctx.Done()
		return nil
	}
	ticker := time.NewTicker(time.Second * interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info(fmt.Sprintf("HealthCheck: exiting: %s", ctx.Err().Error()), nil)
			return nil
		case <-ticker.C:
			logger.Debug("HealthCheck", log.Fields{"status": hc.status, "okCount": hc.okCounter})
			err := hc.Do(ctx)
			if err != nil && hc.status == Healthy {
				if err := hc.cbUnhealthy(ctx); err != nil {
					logger.Error("HealthCheck callback error, status not changed", log.Fields{"error": err.Error()})
					continue
				}
				hc.status = Unhealthy
				hc.okCounter = 0
				logger.Warn("HealthCheck status changed", log.Fields{"status": hc.status, "okCount": hc.okCounter})
				continue
			}
			if err == nil && hc.status == Unhealthy {
				if hc.okCounter >= healthyThreshold {
					if err := hc.cbHealthy(ctx); err != nil {
						logger.Error("HealthCheck callback error, status not changed", log.Fields{"error": err.Error()})
						continue
					}
					hc.status = Healthy
					continue
				}
				hc.okCounter++
			}
		}
	}
}

func (hc *HealthCheck) Do(ctx context.Context) error {
	req := http.Request{Method: http.MethodGet, URL: hc.u}
	resp, err := hc.client.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("HealthCheck: http get failed: %w", err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("HealthCheck: read response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HealthCheck: unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
