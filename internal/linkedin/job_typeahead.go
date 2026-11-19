package linkedin

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

// The fixed landing query opens LinkedIn's job search shell without applying
// typeahead text as job search terms.
const jobTypeaheadPath = "/jobs/search-results/?keywords=jobs"

// ValidLocationQuery reports whether query meets LinkedIn location typeahead constraints.
func ValidLocationQuery(query string) bool {
	return validTypeaheadQuery(query)
}

// ValidJobCompanyQuery reports whether query meets LinkedIn company typeahead constraints.
func ValidJobCompanyQuery(query string) bool {
	return validTypeaheadQuery(query)
}

func validTypeaheadQuery(query string) bool {
	return utf8.ValidString(query) && len(query) <= 120 && utf8.RuneCountInString(query) >= 2 &&
		strings.TrimSpace(query) == query && strings.IndexFunc(query, unicode.IsControl) == -1
}

// JobLocations looks up LinkedIn geography IDs matching query.
func (c *Client) JobLocations(ctx context.Context, query string) (JobLocations, error) {
	if !ValidLocationQuery(query) {
		return JobLocations{}, fmt.Errorf("%w: invalid location query", ErrInvalidInput)
	}
	if err := c.ensureReady(); err != nil {
		return JobLocations{}, err
	}
	inv, err := c.scripts.JobLocations(query)
	if err != nil {
		return JobLocations{}, err
	}
	return readTypeahead(ctx, c, "job_locations", inv, locationIDPattern,
		func(item wireJobLocation) (string, string) { return item.ID, item.Name },
		func(status string, items []wireJobLocation) JobLocations {
			out := make([]JobLocation, len(items))
			for i, item := range items {
				out[i] = JobLocation{ID: item.ID, Name: item.Name}
			}
			return JobLocations{Items: out, Status: status}
		},
	)
}

// JobCompanies looks up LinkedIn organization IDs matching query for the company filter.
func (c *Client) JobCompanies(ctx context.Context, query string) (JobCompanies, error) {
	if !ValidJobCompanyQuery(query) {
		return JobCompanies{}, fmt.Errorf("%w: invalid company query", ErrInvalidInput)
	}
	if err := c.ensureReady(); err != nil {
		return JobCompanies{}, err
	}
	inv, err := c.scripts.JobCompanies(query)
	if err != nil {
		return JobCompanies{}, err
	}
	return readTypeahead(ctx, c, "job_companies", inv, companyIDPattern,
		func(item wireJobCompany) (string, string) { return item.ID, item.Name },
		func(status string, items []wireJobCompany) JobCompanies {
			out := make([]JobCompany, len(items))
			for i, item := range items {
				out[i] = JobCompany{ID: item.ID, Name: item.Name, Industry: item.Industry}
			}
			return JobCompanies{Items: out, Status: status}
		},
	)
}

func readTypeahead[Item any, Out any](
	ctx context.Context,
	c *Client,
	op string,
	inv scripts.Invocation,
	idPattern *regexp.Regexp,
	fields func(Item) (id, name string),
	build func(status string, items []Item) Out,
) (Out, error) {
	var zero Out
	wire, err := readWire[wireTypeahead[Item]](ctx, c, false, op, jobTypeaheadPath, inv.Expression)
	if err != nil {
		return zero, err
	}
	if wire.Items == nil || len(*wire.Items) > maxTypeaheadSuggestions {
		return zero, contractError(op, "missing or oversized typeahead items", nil)
	}
	items := *wire.Items
	if (len(items) == 0 && wire.Status != "no_suggestions_observed") ||
		(len(items) > 0 && wire.Status != "suggestions") {
		return zero, contractError(op, "inconsistent typeahead status", nil)
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id, name := fields(item)
		if !idPattern.MatchString(id) || strings.TrimSpace(name) == "" {
			return zero, contractError(op, "invalid typeahead item", nil)
		}
		if _, duplicate := seen[id]; duplicate {
			return zero, contractError(op, "duplicate typeahead item id", nil)
		}
		seen[id] = struct{}{}
	}
	return build(wire.Status, items), nil
}
