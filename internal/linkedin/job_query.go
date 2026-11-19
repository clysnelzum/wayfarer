package linkedin

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	searchPageSize     = 25 // Offset increment observed on LinkedIn's search UI.
	maxSuggestedFacets = 12
)

var (
	jobIDPattern      = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	locationIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
	companyIDPattern  = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	suggestedFacetID  = regexp.MustCompile(`^(228001|230001):([1-9][0-9]{0,11})$`)
	dateFacets        = map[string]string{"day": "r86400", "week": "r604800", "month": "r2592000"}
	workplaceFacets   = map[string]string{"remote": "272001", "hybrid": "1621001", "onsite": "296001"}
	experienceFacets  = map[string]string{"entry": "276001", "senior": "277001", "manager": "278001", "director": "272003", "executive": "279001"}
	employmentFacets  = map[string]string{"full_time": "272015", "part_time": "273001", "contract": "274001", "internship": "275001", "volunteer": "272002"}
)

// ValidJobID reports whether id is a valid positive numeric LinkedIn job identifier.
func ValidJobID(id string) bool {
	return jobIDPattern.MatchString(id)
}

func (f JobFilters) empty() bool {
	return f.LocationID == "" && len(f.LocationIDs) == 0 && f.CompanyID == "" && len(f.CompanyIDs) == 0 && f.PostedWithin == "" &&
		len(f.Workplace) == 0 && len(f.Experience) == 0 && len(f.Employment) == 0 && len(f.SuggestedFacetIDs) == 0 &&
		!f.EasyApply && !f.InNetwork && !f.Verified && !f.Under10
}

// Validate checks that q obeys LinkedIn job query parameter rules and combinations.
func (q JobQuery) Validate() error {
	_, err := q.searchPath()
	return err
}

func (q JobQuery) searchPath() (string, error) {
	f := q.Filters
	if q.SourceContext != "" {
		if _, err := decodeSourceContext(q.SourceContext, q.Source); err != nil {
			return "", fmt.Errorf("%w: invalid recommendation source context", ErrInvalidInput)
		}
	}
	if q.Source != "" {
		if _, ok := recommendationOrigins[q.Source]; !ok {
			return "", fmt.Errorf("%w: source must be preferences or profile_match", ErrInvalidInput)
		}
		if q.Keywords != "" || q.RecommendationLimit != 0 || !f.empty() {
			return "", fmt.Errorf("%w: source can only be combined with page", ErrInvalidInput)
		}
		if q.Page < 1 || q.Page > MaxSearchPage {
			return "", fmt.Errorf("%w: page must be an integer from 1 to %d", ErrInvalidInput, MaxSearchPage)
		}
		return "/jobs/", nil // Resolve the personalized destination before searching.
	}
	if q.RecommendationLimit != 0 {
		if q.Keywords != "" {
			return "", fmt.Errorf("%w: limit is only supported for recommendations without q", ErrInvalidInput)
		}
		if q.RecommendationLimit < 1 || q.RecommendationLimit > MaxJobRecommendationLimit {
			return "", fmt.Errorf("%w: limit must be an integer from 1 to %d", ErrInvalidInput, MaxJobRecommendationLimit)
		}
	}
	if q.Keywords == "" {
		if q.Page != 0 || !f.empty() {
			return "", fmt.Errorf("%w: page and filters require a keyword search (q)", ErrInvalidInput)
		}
		return "/jobs/", nil
	}
	if !utf8.ValidString(q.Keywords) || len(q.Keywords) > 240 || strings.TrimSpace(q.Keywords) == "" {
		return "", fmt.Errorf("%w: q must contain 1 to 240 UTF-8 bytes", ErrInvalidInput)
	}
	if q.Page < 1 || q.Page > MaxSearchPage {
		return "", fmt.Errorf("%w: page must be an integer from 1 to %d", ErrInvalidInput, MaxSearchPage)
	}

	args := url.Values{"keywords": {q.Keywords}}
	if q.Page > 1 {
		args.Set("start", strconv.Itoa((q.Page-1)*searchPageSize))
	}

	locations := f.LocationIDs
	if f.LocationID != "" {
		if len(locations) != 0 {
			return "", fmt.Errorf("%w: use either geo_id or geo_ids, not both", ErrInvalidInput)
		}
		locations = []string{f.LocationID}
	}
	if len(locations) > 0 {
		if !validIDList(locations, locationIDPattern, 20) {
			return "", fmt.Errorf("%w: geo_ids accepts up to 20 distinct numeric LinkedIn geography IDs", ErrInvalidInput)
		}
		args.Set("geoId", strings.Join(locations, ","))
	}

	companies := f.CompanyIDs
	if f.CompanyID != "" {
		if len(companies) != 0 {
			return "", fmt.Errorf("%w: use either company_id or company_ids, not both", ErrInvalidInput)
		}
		companies = []string{f.CompanyID}
	}
	if len(companies) > 0 {
		if !validIDList(companies, companyIDPattern, 20) {
			return "", fmt.Errorf("%w: company_ids accepts up to 20 distinct numeric LinkedIn organization IDs", ErrInvalidInput)
		}
		args.Set("f_C", strings.Join(companies, ","))
	}

	if f.PostedWithin != "" {
		date, ok := dateFacets[f.PostedWithin]
		if !ok {
			return "", fmt.Errorf("%w: posted_within must be day, week, or month", ErrInvalidInput)
		}
		args.Set("f_TPR", date)
	}

	facets := []struct {
		param, group string
		options      []string
		ids          map[string]string
	}{
		{"workplace", "225001", f.Workplace, workplaceFacets},
		{"experience", "227001", f.Experience, experienceFacets},
		{"employment_type", "226001", f.Employment, employmentFacets},
	}
	var encoded []string
	for _, facet := range facets {
		if len(facet.options) == 0 {
			continue
		}
		seen := make(map[string]bool, len(facet.options))
		ids := make([]string, 0, len(facet.options))
		for _, opt := range facet.options {
			val, ok := facet.ids[opt]
			if !ok || seen[opt] {
				return "", fmt.Errorf("%w: %s contains an unsupported or duplicate value", ErrInvalidInput, facet.param)
			}
			seen[opt] = true
			ids = append(ids, val)
		}
		encoded = append(encoded, "f_SA_id_"+facet.group+":"+strings.Join(ids, ","))
	}

	if len(f.SuggestedFacetIDs) > maxSuggestedFacets {
		return "", fmt.Errorf("%w: suggested_facet_ids accepts at most %d IDs", ErrInvalidInput, maxSuggestedFacets)
	}
	seenSuggestions := make(map[string]bool, len(f.SuggestedFacetIDs))
	suggestedGroups := make(map[string][]string)
	for _, id := range f.SuggestedFacetIDs {
		match := suggestedFacetID.FindStringSubmatch(id)
		if match == nil || seenSuggestions[id] {
			return "", fmt.Errorf("%w: suggested_facet_ids must contain distinct IDs returned by the job search", ErrInvalidInput)
		}
		seenSuggestions[id] = true
		suggestedGroups[match[1]] = append(suggestedGroups[match[1]], match[2])
	}
	for _, group := range []string{"230001", "228001"} {
		if ids := suggestedGroups[group]; len(ids) > 0 {
			encoded = append(encoded, "f_SA_id_"+group+":"+strings.Join(ids, ","))
		}
	}
	if len(encoded) > 0 {
		args.Set("f_SAL", strings.Join(encoded, "$"))
	}
	if f.EasyApply {
		args.Set("f_AL", "true")
	}
	if f.InNetwork {
		args.Set("f_JIYN", "true")
	}
	if f.Verified {
		args.Set("f_VJ", "true")
	}
	if f.Under10 {
		args.Set("f_EA", "true")
	}
	return "/jobs/search-results/?" + args.Encode(), nil
}

// validateObservedSearch only compares controls in Wayfarer's contract. Ignore
// unrelated navigation/tracking keys and a location default chosen by Chrome.
func validateObservedSearch(path string, actual url.Values) error {
	u, err := url.Parse(path)
	if err != nil {
		return err
	}
	want := u.Query()
	keys := []string{"keywords", "start", "geoId", "f_C", "f_TPR", "f_SAL", "f_AL", "f_JIYN", "f_VJ", "f_EA"}
	if want.Has("origin") {
		keys = append(keys, "origin", "originToLandingJobPostings")
	}
	for _, key := range keys {
		if len(actual[key]) > 1 {
			return fmt.Errorf("%w: duplicated %s", ErrSearchMismatch, key)
		}
		a, b := actual.Get(key), want.Get(key)
		switch key {
		case "geoId":
			if b == "" {
				continue
			}
			a, b = normalizedCSV(a), normalizedCSV(b)
		case "origin":
			a, b = strings.TrimPrefix(a, "JobSearchOrigin_"), strings.TrimPrefix(b, "JobSearchOrigin_")
		case "start":
			if a == "0" {
				a = ""
			}
		case "f_AL", "f_JIYN", "f_VJ", "f_EA":
			if a == "false" {
				a = ""
			}
		case "f_C", "originToLandingJobPostings":
			a, b = normalizedCSV(a), normalizedCSV(b)
		case "f_SAL":
			a, b = normalizedFacets(a), normalizedFacets(b)
		}
		if a != b {
			return fmt.Errorf("%w: %s", ErrSearchMismatch, key)
		}
	}
	return nil
}

// LinkedIn may reorder facet groups or alternatives when serializing the URL.
// Sorting preserves cardinality and does not ignore extra groups or values.
func normalizedCSV(value string) string {
	ids := strings.Split(value, ",")
	slices.Sort(ids)
	return strings.Join(ids, ",")
}

func normalizedFacets(value string) string {
	groups := strings.Split(value, "$")
	for i, group := range groups {
		if key, raw, ok := strings.Cut(group, ":"); ok {
			groups[i] = key + ":" + normalizedCSV(raw)
		}
	}
	slices.Sort(groups)
	return strings.Join(groups, "$")
}

func validIDList(ids []string, pattern *regexp.Regexp, maxLen int) bool {
	if len(ids) < 1 || len(ids) > maxLen {
		return false
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !pattern.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
