package linkedin

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type JobLocations struct {
	Items  []JobLocation `json:"items"`
	Status string        `json:"status"`
}

type JobLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// The fixed landing query opens LinkedIn's job search shell without applying
// typeahead text as job search terms.
const jobTypeaheadPath = "/jobs/search-results/?keywords=jobs"

func ValidLocationQuery(q string) bool {
	return validTypeaheadQuery(q)
}

func validTypeaheadQuery(q string) bool {
	return utf8.ValidString(q) && len(q) <= 120 && utf8.RuneCountInString(q) >= 2 &&
		strings.TrimSpace(q) == q && strings.IndexFunc(q, unicode.IsControl) == -1
}

func (c Client) JobLocations(ctx context.Context, q string) (JobLocations, error) {
	if !ValidLocationQuery(q) {
		return JobLocations{}, errors.New("invalid location query")
	}
	inv, err := c.scripts().JobLocations(q)
	if err != nil {
		return JobLocations{}, err
	}
	v, err := read[JobLocations](ctx, c.Reader, jobTypeaheadPath, inv.Expression)
	if err != nil {
		return JobLocations{}, err
	}
	if v.Items == nil || len(v.Items) > 15 || (len(v.Items) == 0 && v.Status != "no_suggestions_observed") ||
		(len(v.Items) > 0 && v.Status != "suggestions") {
		return JobLocations{}, errors.New("invalid location suggestions from Chrome")
	}
	for _, item := range v.Items {
		if !locationIDPattern.MatchString(item.ID) || item.Name == "" {
			return JobLocations{}, errors.New("invalid location suggestion from Chrome")
		}
	}
	return v, nil
}
