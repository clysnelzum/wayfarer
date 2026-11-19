package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

type Source interface {
	Jobs(context.Context, linkedin.JobQuery) (linkedin.JobResults, error)
	Job(context.Context, string) (linkedin.Job, error)
	JobLocations(context.Context, string) (linkedin.JobLocations, error)
	JobCompanies(context.Context, string) (linkedin.JobCompanies, error)
	Feed(context.Context, linkedin.FeedQuery) (linkedin.FeedResults, error)
	Profile(context.Context, string) (linkedin.Profile, error)
	Company(context.Context, string) (linkedin.Company, error)
}

var jobID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func New(token string, source Source) http.Handler {
	want := sha256.Sum256([]byte(token))
	refs := newSourceRefs()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		hash := sha256.Sum256([]byte(got))
		if token == "" || !ok || subtle.ConstantTimeCompare(want[:], hash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fail(w, http.StatusUnauthorized, "unauthorized", "A valid API token is required.")
			return
		}
		p := r.URL.Path
		job, isJob := strings.CutPrefix(p, "/v1/jobs/")
		profile, isProfile := strings.CutPrefix(p, "/v1/profiles/")
		company, isCompany := strings.CutPrefix(p, "/v1/companies/")
		if p != "/healthz" && p != "/v1/feed" && p != "/v1/jobs" && !isJob && !isProfile && !isCompany {
			fail(w, http.StatusNotFound, "not_found", "Unknown API route.")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported.")
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || (len(query) > 0 && p != "/v1/jobs" && p != "/v1/jobs/locations" && p != "/v1/jobs/companies" && p != "/v1/feed") {
			fail(w, http.StatusBadRequest, "invalid_query", "This route does not accept these query parameters.")
			return
		}
		ctx := r.Context()
		switch {
		case p == "/healthz":
			write(w, http.StatusOK, map[string]string{"status": "ok"})
		case p == "/v1/jobs":
			q, err := parseJobQuery(query)
			if err != nil {
				fail(w, http.StatusBadRequest, "invalid_query", err.Error())
				return
			}
			ref := query.Get("source_ref")
			if ref != "" {
				q.SourceContext = refs.open(ref, q.Source, time.Now())
				if q.SourceContext == nil {
					fail(w, http.StatusGone, "recommendation_source_ref_unavailable", "Refresh recommendations to get a new source_ref.")
					return
				}
			}
			v, err := source.Jobs(ctx, q)
			if err == nil {
				now := time.Now()
				for i := range v.Sections {
					v.Sections[i].SourceRef = refs.seal(v.Sections[i].SourceContext, now)
				}
				if v.Search != nil && v.Search.SourceContext != nil {
					v.Search.SourceRef = ref
					if ref == "" {
						v.Search.SourceRef = refs.seal(v.Search.SourceContext, now)
					}
				}
			}
			result(w, v, err)
		case p == "/v1/feed":
			q, err := parseFeedQuery(query)
			if err != nil {
				fail(w, http.StatusBadRequest, "invalid_query", err.Error())
				return
			}
			v, err := source.Feed(ctx, q)
			result(w, v, err)
		case p == "/v1/jobs/locations":
			q := strings.TrimSpace(query.Get("q"))
			if len(query) != 1 || len(query["q"]) != 1 || !linkedin.ValidLocationQuery(q) {
				fail(w, http.StatusBadRequest, "invalid_query", "q must contain at least 2 characters and at most 120 UTF-8 bytes, without control characters.")
				return
			}
			v, err := source.JobLocations(ctx, q)
			result(w, v, err)
		case p == "/v1/jobs/companies":
			q := strings.TrimSpace(query.Get("q"))
			if len(query) != 1 || len(query["q"]) != 1 || !linkedin.ValidJobCompanyQuery(q) {
				fail(w, http.StatusBadRequest, "invalid_query", "q must contain at least 2 characters and at most 120 UTF-8 bytes, without control characters.")
				return
			}
			v, err := source.JobCompanies(ctx, q)
			result(w, v, err)
		case isJob:
			if !jobID.MatchString(job) {
				fail(w, http.StatusBadRequest, "invalid_job_id", "Expected a numeric job ID.")
				return
			}
			v, err := source.Job(ctx, job)
			result(w, v, err)
		case isProfile:
			if !linkedin.ValidSlug(profile) {
				fail(w, http.StatusBadRequest, "invalid_slug", "Expected a LinkedIn profile slug.")
				return
			}
			v, err := source.Profile(ctx, profile)
			result(w, v, err)
		case isCompany:
			if !linkedin.ValidSlug(company) {
				fail(w, http.StatusBadRequest, "invalid_slug", "Expected a LinkedIn company slug.")
				return
			}
			v, err := source.Company(ctx, company)
			result(w, v, err)
		}
	})
}

func parseFeedQuery(values url.Values) (linkedin.FeedQuery, error) {
	if len(values) == 0 {
		return linkedin.FeedQuery{}, nil
	}
	if len(values) != 1 || len(values["limit"]) != 1 {
		return linkedin.FeedQuery{}, errors.New("Only one limit parameter is supported.")
	}
	limit, err := strconv.Atoi(values.Get("limit"))
	if err != nil || limit < 1 || limit > linkedin.MaxFeedLimit {
		return linkedin.FeedQuery{}, errors.New("limit must be an integer from 1 to " + strconv.Itoa(linkedin.MaxFeedLimit) + ".")
	}
	return linkedin.FeedQuery{Limit: limit}, nil
}

func parseJobQuery(values url.Values) (linkedin.JobQuery, error) {
	for key, entries := range values {
		switch key {
		case "q", "source", "source_ref", "page", "limit", "geo_id", "geo_ids", "company_id", "company_ids", "posted_within", "workplace", "experience", "employment_type", "suggested_facet_ids", "easy_apply", "in_network", "verified", "under_10_applicants":
		default:
			return linkedin.JobQuery{}, errors.New("Unsupported job search parameter.")
		}
		if len(entries) != 1 || strings.TrimSpace(entries[0]) == "" {
			return linkedin.JobQuery{}, errors.New("Job search parameters must be supplied once and cannot be empty.")
		}
	}
	q := linkedin.JobQuery{Source: values.Get("source")}
	if values.Has("source_ref") && q.Source == "" {
		return q, errors.New("source_ref requires source.")
	}
	if q.Source != "" {
		for key := range values {
			if key != "source" && key != "page" && key != "source_ref" {
				return q, errors.New("source can only be combined with page and source_ref.")
			}
		}
		q.Page = 1
	}
	if _, ok := values["q"]; ok {
		q.Keywords = strings.TrimSpace(values.Get("q"))
		if q.Keywords == "" || len(q.Keywords) > 240 || !utf8.ValidString(q.Keywords) {
			return q, errors.New("q must contain 1 to 240 UTF-8 bytes.")
		}
		q.Page = 1
	}
	if _, ok := values["page"]; ok {
		if q.Keywords == "" && q.Source == "" {
			return q, errors.New("page requires q or source.")
		}
		page, err := strconv.Atoi(values.Get("page"))
		if err != nil || page < 1 || page > linkedin.MaxSearchPage {
			return q, errors.New("page must be an integer from 1 to " + strconv.Itoa(linkedin.MaxSearchPage) + ".")
		}
		q.Page = page
	}
	if q.Source != "" {
		return q, q.Validate()
	}
	if _, ok := values["limit"]; ok {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit < 1 || limit > linkedin.MaxJobRecommendationLimit {
			return q, errors.New("limit must be an integer from 1 to " + strconv.Itoa(linkedin.MaxJobRecommendationLimit) + ".")
		}
		q.RecommendationLimit = limit
	}
	if q.Keywords == "" && len(values) > 0 && !(q.RecommendationLimit > 0 && len(values) == 1) {
		return q, errors.New("Filters require a keyword search (q).")
	}
	for _, key := range []string{"easy_apply", "in_network", "verified", "under_10_applicants"} {
		if v := values.Get(key); v != "" && v != "true" && v != "false" {
			return q, errors.New(key + " must be true or false.")
		}
	}
	options := func(key string) []string {
		if values.Get(key) == "" {
			return nil
		}
		return strings.Split(values.Get(key), ",")
	}
	q.Filters = linkedin.JobFilters{
		LocationID: values.Get("geo_id"), LocationIDs: options("geo_ids"), CompanyID: values.Get("company_id"), CompanyIDs: options("company_ids"), PostedWithin: values.Get("posted_within"),
		Workplace: options("workplace"), Experience: options("experience"), Employment: options("employment_type"),
		SuggestedFacetIDs: options("suggested_facet_ids"),
		EasyApply:         values.Get("easy_apply") == "true", InNetwork: values.Get("in_network") == "true",
		Verified: values.Get("verified") == "true", Under10: values.Get("under_10_applicants") == "true",
	}
	return q, q.Validate()
}

func result(w http.ResponseWriter, value any, err error) {
	switch {
	case err == nil:
		write(w, http.StatusOK, value)
	case errors.Is(err, linkedin.ErrSearchMismatch):
		fail(w, http.StatusBadGateway, "upstream_search_changed", "LinkedIn did not retain the requested job search, page, or filters.")
	case errors.Is(err, linkedin.ErrJobSourceUnavailable):
		fail(w, http.StatusNotFound, "recommendation_source_unavailable", "LinkedIn did not show this recommendation source in the current session.")
	case errors.Is(err, linkedin.ErrNotFound):
		fail(w, http.StatusNotFound, "job_not_found", "LinkedIn reports that the job is unavailable.")
	case errors.Is(err, linkedin.ErrProfileNotFound):
		fail(w, http.StatusNotFound, "profile_not_found", "LinkedIn reports that the personal profile is unavailable.")
	case errors.Is(err, linkedin.ErrCompanyNotFound):
		fail(w, http.StatusNotFound, "company_not_found", "LinkedIn reports that the company profile is unavailable.")
	case errors.Is(err, chrome.ErrDebuggerUnavailable):
		fail(w, http.StatusServiceUnavailable, "upstream_debugger_unavailable", "Check the Chrome debugger endpoint and approve the connection in Chrome if prompted.")
	case errors.Is(err, chrome.ErrAuth):
		fail(w, http.StatusServiceUnavailable, "upstream_auth_required", "Sign in or complete the verification in Chrome.")
	case errors.Is(err, chrome.ErrBusy):
		w.Header().Set("Retry-After", "2")
		fail(w, http.StatusServiceUnavailable, "upstream_busy", "Chrome has too many active queries.")
	case errors.Is(err, context.DeadlineExceeded):
		fail(w, http.StatusGatewayTimeout, "upstream_timeout", "Chrome did not provide the expected page data in time.")
	case errors.Is(err, context.Canceled):
		return
	default:
		fail(w, http.StatusBadGateway, "upstream_error", "Could not read the LinkedIn page via Chrome.")
	}
}

func fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
