// Package catalog is the adapter of I-02: chat reads a product from catalog-service when a
// conversation opens (E-30), with its own service token (ADR-020 point 5).
package catalog

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// Client calls I-02 as the Anexo D asks of an outgoing call: service token, X-Correlation-Id,
// a timeout per attempt and a bounded retry with full jitter only for transient failures.
type Client struct {
	BaseURL       string
	ServiceToken  string
	Timeout       time.Duration // per attempt
	Attempts      int
	BaseDelay     time.Duration // the wait before attempt n is random in [0, BaseDelay × 2ⁿ⁻¹]
	HTTP          *http.Client
	CorrelationID func(context.Context) string
}

type productView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProducerID   string `json:"producerId"`
	ProducerName string `json:"producerName"`
}

// Product: 200 → the snapshot; 404 → ErrProductNotFound; a transient failure (network, timeout,
// 429, 5xx) is retried; whatever is left, or any other answer, is ErrCatalogUnavailable (E-30 row 2).
func (c *Client) Product(ctx context.Context, id string) (model.ProductSnapshot, error) {
	for attempt := 1; attempt <= c.Attempts; attempt++ {
		if attempt > 1 {
			wait := time.Duration(rand.Int64N(int64(c.BaseDelay<<(attempt-2)) + 1))
			select {
			case <-ctx.Done():
				return model.ProductSnapshot{}, model.ErrCatalogUnavailable
			case <-time.After(wait):
			}
		}
		p, err, transient := c.once(ctx, id)
		if !transient {
			return p, err
		}
	}
	return model.ProductSnapshot{}, model.ErrCatalogUnavailable
}

func (c *Client) once(ctx context.Context, id string) (p model.ProductSnapshot, err error, transient bool) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/internal/v1/catalog/products/"+id, nil)
	if err != nil {
		return p, model.ErrCatalogUnavailable, false
	}
	req.Header.Set("Authorization", "Bearer "+c.ServiceToken)
	req.Header.Set("X-Correlation-Id", c.CorrelationID(ctx))
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return p, model.ErrCatalogUnavailable, true
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusOK:
		var v productView
		if json.NewDecoder(res.Body).Decode(&v) != nil || v.ID == "" || v.ProducerID == "" {
			return p, model.ErrCatalogUnavailable, false
		}
		return model.ProductSnapshot{ID: v.ID, Name: v.Name, ProducerID: v.ProducerID, ProducerName: v.ProducerName}, nil, false
	case res.StatusCode == http.StatusNotFound:
		return p, model.ErrProductNotFound, false
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return p, model.ErrCatalogUnavailable, true
	}
	return p, model.ErrCatalogUnavailable, false
}
