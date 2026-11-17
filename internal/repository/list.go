package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrInvalidListQuery reports a filter, sort or value a listing does not accept. The
// HTTP layer answers it with 400, so a typo is an error rather than an ignored filter.
var ErrInvalidListQuery = errors.New("invalid list query")

// ListQuery narrows, searches and orders one page of a listing. Field names are the
// API's, not columns: each repository maps the ones it allows through a listSpec.
type ListQuery struct {
	Page, Size int
	// Filters holds exact-match values by field; a comma-separated value matches any.
	Filters map[string]string
	// Search matches case-insensitively anywhere in the listing's searchable columns.
	Search string
	Sort   string
	Desc   bool
	// From and To bound created_at, inclusive of From and exclusive of To.
	From, To *time.Time
}

// filterKind says how a filter's raw value becomes a column value.
type filterKind int

const (
	textFilter filterKind = iota
	boolFilter
)

type filterSpec struct {
	column string
	kind   filterKind
}

// listSpec is what one listing allows: which fields filter and sort which columns, and
// which columns a search looks in. Anything not declared here is rejected.
type listSpec struct {
	filters map[string]filterSpec
	search  []string
	sorts   map[string]string
	// order applies when the caller names no sort, and breaks ties when it does.
	order string
}

// likeEscape is '!' rather than a backslash, which MySQL string literals treat as an
// escape of their own; every supported database accepts it in an ESCAPE clause.
const likeEscape = "!"

var likeEscaper = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")

// apply narrows session to q under spec. It returns ErrInvalidListQuery, wrapped with
// the offending field, for anything the spec does not declare.
func (s listSpec) apply(session *gorm.DB, q ListQuery) (*gorm.DB, error) {
	for field, raw := range q.Filters {
		filter, ok := s.filters[field]
		if !ok {
			return nil, fmt.Errorf("%w: unknown filter %q", ErrInvalidListQuery, field)
		}
		values, err := filterValues(filter.kind, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: filter %q: %v", ErrInvalidListQuery, field, err)
		}
		session = session.Where(filter.column+" IN ?", values)
	}
	if term := strings.TrimSpace(q.Search); term != "" {
		if len(s.search) == 0 {
			return nil, fmt.Errorf("%w: this listing is not searchable", ErrInvalidListQuery)
		}
		pattern := "%" + likeEscaper.Replace(strings.ToLower(term)) + "%"
		clauses := make([]string, len(s.search))
		args := make([]any, len(s.search))
		for index, column := range s.search {
			clauses[index] = "LOWER(" + column + ") LIKE ? ESCAPE '" + likeEscape + "'"
			args[index] = pattern
		}
		session = session.Where("("+strings.Join(clauses, " OR ")+")", args...)
	}
	if q.From != nil {
		session = session.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		session = session.Where("created_at < ?", *q.To)
	}
	return session, nil
}

// orderBy resolves the caller's sort against spec, falling back to the default order.
func (s listSpec) orderBy(q ListQuery) (string, error) {
	if q.Sort == "" {
		return s.order, nil
	}
	column, ok := s.sorts[q.Sort]
	if !ok {
		return "", fmt.Errorf("%w: cannot sort by %q", ErrInvalidListQuery, q.Sort)
	}
	direction := " asc"
	if q.Desc {
		direction = " desc"
	}
	// The default order breaks ties, so rows with equal sort values keep a stable page.
	return column + direction + ", " + s.order, nil
}

func filterValues(kind filterKind, raw string) ([]any, error) {
	parts := strings.Split(raw, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if kind == boolFilter {
			value, err := strconv.ParseBool(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not true or false", part)
			}
			values = append(values, value)
			continue
		}
		values = append(values, part)
	}
	if len(values) == 0 {
		return nil, errors.New("no value")
	}
	return values, nil
}

// list returns one filtered, ordered page and the total of the same filter, so the
// page reports how much of the filtered listing it is.
func (b base[T]) list(ctx context.Context, spec listSpec, q ListQuery, scope ...func(*gorm.DB) *gorm.DB) (Page[T], error) {
	order, err := spec.orderBy(q)
	if err != nil {
		return Page[T]{}, err
	}
	filtered, err := spec.apply(b.model(ctx).Scopes(scope...), q)
	if err != nil {
		return Page[T]{}, err
	}
	var total int64
	// Session forks the statement, so the count and the page share filters, not state.
	if err := filtered.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return Page[T]{}, err
	}
	items := []T{}
	err = filtered.Session(&gorm.Session{}).Order(order).
		Limit(q.Size).Offset((q.Page - 1) * q.Size).Find(&items).Error
	return Page[T]{Items: items, Total: total}, err
}
