package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/cache"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/repository"
)

// maxAnalyticsBuckets bounds a query by refusing a range that would produce more
// points than a chart can render, rather than silently truncating it.
const maxAnalyticsBuckets = 400

// AnalyticsFilter narrows a metrics query. Every field is optional.
type AnalyticsFilter struct {
	// From is the inclusive start of the range.
	From time.Time
	// To is the exclusive end of the range.
	To time.Time
	// Interval buckets the range: "day" or "hour".
	Interval string
	// AppID restricts the query to one application.
	AppID string
	// ProviderAccountID restricts the query to one provider account.
	ProviderAccountID string
}

// ServiceCounts holds one bucket's transaction counts by service type.
type ServiceCounts struct {
	// Collection is the number of collections in the bucket.
	Collection int64 `json:"collection"`
	// Disbursement is the number of disbursements in the bucket.
	Disbursement int64 `json:"disbursement"`
}

// AnalyticsBucket is one point on a time series.
type AnalyticsBucket struct {
	// Period is the bucket's start, formatted to the interval's precision.
	Period string `json:"period"`
	// Total is every transaction in the bucket, whatever its service or status.
	Total int64 `json:"total"`
	// ByService splits Total into collections and disbursements.
	ByService ServiceCounts `json:"by_service"`
	// Succeeded counts transactions that reached a successful terminal state.
	Succeeded int64 `json:"succeeded"`
	// Failed counts transactions that failed, expired, or were cancelled.
	Failed int64 `json:"failed"`
}

// CurrencyVolume is the transaction volume for one currency.
type CurrencyVolume struct {
	// Currency is the three-letter code.
	Currency string `json:"currency"`
	// Count is the number of transactions in that currency.
	Count int64 `json:"count"`
	// Amount is the summed amount in the currency's minor unit.
	Amount int64 `json:"amount"`
}

// TransactionAnalytics is a bucketed transaction series with its totals.
type TransactionAnalytics struct {
	// From is the inclusive start of the range covered.
	From time.Time `json:"from"`
	// To is the exclusive end of the range covered.
	To time.Time `json:"to"`
	// Interval is the bucket width, "day" or "hour".
	Interval string `json:"interval"`
	// Buckets is the series, ascending, with empty periods present and zeroed so a
	// chart shows a gap in traffic rather than joining across it.
	Buckets []AnalyticsBucket `json:"buckets"`
	// Total is every transaction in the range.
	Total int64 `json:"total"`
	// ByService splits Total across the two identity.
	ByService ServiceCounts `json:"by_service"`
	// Volume is the amount moved, per currency.
	//
	// Deliberately never summed into one figure: amounts are in each currency's minor
	// unit, so adding UGX to USD would produce a number that means nothing. A caller
	// that wants one line picks a currency.
	Volume []CurrencyVolume `json:"volume"`
}

// AnalyticsService answers aggregate questions about transactions.
type AnalyticsService struct {
	repos *repository.UnitOfWork
	cache *cache.RedisStore
}

// NewAnalyticsService creates a transaction analytics service.
func NewAnalyticsService(repos *repository.UnitOfWork, store *cache.RedisStore) *AnalyticsService {
	return &AnalyticsService{repos: repos, cache: store}
}

// Transactions returns a bucketed transaction series for the filtered range.
//
// Bucketing happens in SQL, so the response size depends on the range rather than on
// how many transactions fall inside it. The date expression is the one thing that
// cannot be written portably — every driver spells truncation differently — so it is
// selected per dialect and nothing else in the query varies.
func (s *AnalyticsService) Transactions(ctx context.Context, filter AnalyticsFilter) (*TransactionAnalytics, error) {
	filter, err := filter.normalize()
	if err != nil {
		return nil, err
	}
	key := analyticsCacheKey(filter)
	if value := cache.Get[TransactionAnalytics](ctx, s.cache, key); value != nil {
		return value, nil
	}
	expression, err := bucketExpression(s.repos.Dialect(), filter.Interval)
	if err != nil {
		return nil, err
	}

	rows, err := s.repos.Transactions.Aggregate(ctx, repository.AggregateFilter{
		Bucket:            expression,
		From:              filter.From,
		To:                filter.To,
		AppID:             filter.AppID,
		ProviderAccountID: filter.ProviderAccountID,
	})
	if err != nil {
		return nil, err
	}

	out := &TransactionAnalytics{From: filter.From, To: filter.To, Interval: filter.Interval}
	// Pre-seeding every period keeps a quiet day as a zero rather than a missing point,
	// which is the difference between a chart showing no traffic and one implying the
	// line simply jumped.
	index := map[string]int{}
	for _, period := range periods(filter) {
		index[period] = len(out.Buckets)
		out.Buckets = append(out.Buckets, AnalyticsBucket{Period: period})
	}
	volume := map[string]*CurrencyVolume{}
	for _, r := range rows {
		position, ok := index[r.Period]
		if !ok {
			// A row outside the seeded periods means the driver formatted the bucket
			// differently than expected; skipping it silently would understate the
			// chart, so it is a bug worth surfacing rather than swallowing.
			return nil, fmt.Errorf("analytics: bucket %q is outside the requested range", r.Period)
		}
		bucket := &out.Buckets[position]
		bucket.Total += r.Count
		switch r.ServiceType {
		case domain.ServiceCollection:
			bucket.ByService.Collection += r.Count
			out.ByService.Collection += r.Count
		case domain.ServiceDisbursement:
			bucket.ByService.Disbursement += r.Count
			out.ByService.Disbursement += r.Count
		}
		switch r.Status {
		case domain.TxSucceeded:
			bucket.Succeeded += r.Count
		case domain.TxFailed, domain.TxExpired, domain.TxCancelled:
			bucket.Failed += r.Count
		}
		out.Total += r.Count
		if _, ok := volume[r.Currency]; !ok {
			volume[r.Currency] = &CurrencyVolume{Currency: r.Currency}
		}
		volume[r.Currency].Count += r.Count
		volume[r.Currency].Amount += r.Amount
	}
	for _, currency := range sortedKeys(volume) {
		out.Volume = append(out.Volume, *volume[currency])
	}
	cache.Set(ctx, s.cache, key, out)
	return out, nil
}

func analyticsCacheKey(filter AnalyticsFilter) string {
	identity := strings.Join([]string{
		filter.From.Format(time.RFC3339Nano),
		filter.To.Format(time.RFC3339Nano),
		filter.Interval,
		filter.AppID,
		filter.ProviderAccountID,
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return "analytics:v1:" + hex.EncodeToString(digest[:])
}

// normalize applies defaults and rejects a range a chart could not render.
func (f AnalyticsFilter) normalize() (AnalyticsFilter, error) {
	f.Interval = strings.ToLower(strings.TrimSpace(f.Interval))
	if f.Interval == "" {
		f.Interval = "day"
	}
	if f.Interval != "day" && f.Interval != "hour" {
		return f, errors.New("interval must be day or hour")
	}
	now := time.Now().UTC()
	if f.To.IsZero() {
		f.To = now.Add(time.Hour).Truncate(time.Hour)
	}
	if f.From.IsZero() {
		f.From = f.To.AddDate(0, 0, -30)
	}
	f.From, f.To = f.From.UTC().Truncate(time.Hour), f.To.UTC().Truncate(time.Hour)
	if f.Interval == "day" {
		f.From = f.From.Truncate(24 * time.Hour)
	}
	if !f.To.After(f.From) {
		return f, errors.New("to must be after from")
	}
	if len(periods(f)) > maxAnalyticsBuckets {
		return f, fmt.Errorf("range covers more than %d %s buckets; narrow it or use a wider interval", maxAnalyticsBuckets, f.Interval)
	}
	return f, nil
}

// periods lists every bucket label in the range, ascending.
func periods(f AnalyticsFilter) []string {
	step := 24 * time.Hour
	if f.Interval == "hour" {
		step = time.Hour
	}
	var out []string
	for at := f.From; at.Before(f.To); at = at.Add(step) {
		out = append(out, formatPeriod(at, f.Interval))
		if len(out) > maxAnalyticsBuckets {
			return out
		}
	}
	return out
}

// formatPeriod renders a bucket label identically to the SQL expressions below.
func formatPeriod(at time.Time, interval string) string {
	if interval == "hour" {
		return at.UTC().Format("2006-01-02 15:00")
	}
	return at.UTC().Format("2006-01-02")
}

// bucketExpression returns the driver's date-truncation expression.
//
// This is the only per-dialect SQL in the codebase. Bucketing in Go instead would be
// portable but would have to load every matching row, so a busy range would either be
// slow or silently capped; a wrong chart is worse than a dialect switch.
func bucketExpression(dialect, interval string) (string, error) {
	day := interval == "day"
	switch dialect {
	case "sqlite":
		if day {
			return "strftime('%Y-%m-%d', created_at)", nil
		}
		return "strftime('%Y-%m-%d %H:00', created_at)", nil
	case "postgres":
		if day {
			return "to_char(created_at, 'YYYY-MM-DD')", nil
		}
		return "to_char(created_at, 'YYYY-MM-DD HH24:00')", nil
	case "mysql":
		if day {
			return "DATE_FORMAT(created_at, '%Y-%m-%d')", nil
		}
		return "DATE_FORMAT(created_at, '%Y-%m-%d %H:00')", nil
	default:
		return "", fmt.Errorf("analytics is not supported on the %q driver", dialect)
	}
}

// sortedKeys returns a map's keys in ascending order, so a response is stable.
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
