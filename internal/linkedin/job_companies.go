package linkedin

import (
	"context"
	"errors"
	"strings"
)

type JobCompanies struct {
	Items  []JobCompany `json:"items"`
	Status string       `json:"status"`
}

type JobCompany struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Industry string `json:"industry,omitempty"`
}

func ValidJobCompanyQuery(q string) bool {
	return validTypeaheadQuery(q)
}

func (c Client) JobCompanies(ctx context.Context, q string) (JobCompanies, error) {
	if !ValidJobCompanyQuery(q) {
		return JobCompanies{}, errors.New("invalid company query")
	}
	inv, err := c.scripts().JobCompanies(q)
	if err != nil {
		return JobCompanies{}, err
	}
	v, err := read[JobCompanies](ctx, c.Reader, jobTypeaheadPath, inv.Expression)
	if err != nil {
		return JobCompanies{}, err
	}
	if v.Items == nil || len(v.Items) > 15 || (len(v.Items) == 0 && v.Status != "no_suggestions_observed") ||
		(len(v.Items) > 0 && v.Status != "suggestions") {
		return JobCompanies{}, errors.New("invalid company suggestions from Chrome")
	}
	seen := make(map[string]bool, len(v.Items))
	for _, item := range v.Items {
		if !companyIDPattern.MatchString(item.ID) || strings.TrimSpace(item.Name) == "" || seen[item.ID] {
			return JobCompanies{}, errors.New("invalid company suggestion from Chrome")
		}
		seen[item.ID] = true
	}
	return v, nil
}
