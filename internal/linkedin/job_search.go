package linkedin

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxSearchPage = 100
const searchPageSize = 25 // Offset increment observed on LinkedIn's search UI.
const maxSuggestedFacets = 12

type JobQuery struct {
	Keywords string
	Page     int // 0 for recommendations, otherwise one-based.
	Filters  JobFilters
}

type JobFilters struct {
	LocationID        string
	CompanyID         string
	CompanyIDs        []string
	PostedWithin      string
	Workplace         []string
	Experience        []string
	Employment        []string
	SuggestedFacetIDs []string
	EasyApply         bool
	InNetwork         bool
	Verified          bool
	Under10           bool
}

var (
	ErrSearchMismatch = errors.New("LinkedIn did not retain the requested job search")
	locationIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
	companyIDPattern  = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	suggestedFacetID  = regexp.MustCompile(`^(228001|230001):([1-9][0-9]{0,11})$`)
	dateFacets        = map[string]string{"day": "r86400", "week": "r604800", "month": "r2592000"}
	workplaceFacets   = map[string]string{"remote": "272001", "hybrid": "1621001", "onsite": "296001"}
	experienceFacets  = map[string]string{"entry": "276001", "senior": "277001", "manager": "278001", "director": "272003", "executive": "279001"}
	employmentFacets  = map[string]string{"full_time": "272015", "part_time": "273001", "contract": "274001", "internship": "275001", "volunteer": "272002"}
)

func (q JobQuery) searchPath() (string, error) {
	f := q.Filters
	if q.Keywords == "" {
		if q.Page != 0 || f.LocationID != "" || f.CompanyID != "" || len(f.CompanyIDs) != 0 || f.PostedWithin != "" ||
			len(f.Workplace) != 0 || len(f.Experience) != 0 || len(f.Employment) != 0 || len(f.SuggestedFacetIDs) != 0 ||
			f.EasyApply || f.InNetwork || f.Verified || f.Under10 {
			return "", errors.New("page and filters require a keyword search (q)")
		}
		return "/jobs/", nil
	}
	if !utf8.ValidString(q.Keywords) || len(q.Keywords) > 240 || strings.TrimSpace(q.Keywords) == "" {
		return "", errors.New("q must contain 1 to 240 UTF-8 bytes")
	}
	if q.Page < 1 || q.Page > MaxSearchPage {
		return "", fmt.Errorf("page must be an integer from 1 to %d", MaxSearchPage)
	}
	args := url.Values{"keywords": {q.Keywords}}
	if q.Page > 1 {
		args.Set("start", strconv.Itoa((q.Page-1)*searchPageSize))
	}
	if f.LocationID != "" {
		if !locationIDPattern.MatchString(f.LocationID) {
			return "", errors.New("expected a numeric geo_id")
		}
		args.Set("geoId", f.LocationID)
	}
	companies := f.CompanyIDs
	if f.CompanyID != "" {
		if len(companies) != 0 {
			return "", errors.New("use either company_id or company_ids, not both")
		}
		companies = []string{f.CompanyID}
	}
	if len(companies) > 20 {
		return "", errors.New("company_ids accepts at most 20 organization IDs")
	}
	if len(companies) > 0 {
		seen := make(map[string]bool, len(companies))
		for _, id := range companies {
			if !companyIDPattern.MatchString(id) || seen[id] {
				return "", errors.New("company IDs must be distinct numeric LinkedIn organization IDs")
			}
			seen[id] = true
		}
		args.Set("f_C", strings.Join(companies, ","))
	}
	if f.PostedWithin != "" {
		date, ok := dateFacets[f.PostedWithin]
		if !ok {
			return "", errors.New("posted_within must be day, week, or month")
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
		seen := map[string]bool{}
		ids := make([]string, 0, len(facet.options))
		for _, opt := range facet.options {
			val, ok := facet.ids[opt]
			if !ok || seen[opt] {
				return "", fmt.Errorf("%s contains an unsupported or duplicate value", facet.param)
			}
			seen[opt] = true
			ids = append(ids, val)
		}
		encoded = append(encoded, "f_SA_id_"+facet.group+":"+strings.Join(ids, ","))
	}
	if len(f.SuggestedFacetIDs) > maxSuggestedFacets {
		return "", fmt.Errorf("suggested_facet_ids accepts at most %d IDs", maxSuggestedFacets)
	}
	seenSuggestions := make(map[string]bool, len(f.SuggestedFacetIDs))
	suggestedGroups := make(map[string][]string)
	for _, id := range f.SuggestedFacetIDs {
		match := suggestedFacetID.FindStringSubmatch(id)
		if match == nil || seenSuggestions[id] {
			return "", errors.New("suggested_facet_ids must contain distinct IDs returned by the job search")
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
	for _, key := range []string{"keywords", "start", "geoId", "f_C", "f_TPR", "f_SAL", "f_AL", "f_JIYN", "f_VJ", "f_EA"} {
		if len(actual[key]) > 1 {
			return fmt.Errorf("%w: duplicated %s", ErrSearchMismatch, key)
		}
		a, b := actual.Get(key), want.Get(key)
		switch key {
		case "geoId":
			if b == "" {
				continue
			}
		case "start":
			if a == "0" {
				a = ""
			}
		case "f_AL", "f_JIYN", "f_VJ", "f_EA":
			if a == "false" {
				a = ""
			}
		case "f_C":
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
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func normalizedFacets(value string) string {
	groups := strings.Split(value, "$")
	for i, group := range groups {
		if key, raw, ok := strings.Cut(group, ":"); ok {
			groups[i] = key + ":" + normalizedCSV(raw)
		}
	}
	sort.Strings(groups)
	return strings.Join(groups, "$")
}

func (q JobQuery) Validate() error {
	_, err := q.searchPath()
	return err
}
