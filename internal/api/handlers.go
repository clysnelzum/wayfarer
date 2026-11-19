package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

var jobIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

type handler struct {
	service Service
	refs    *sourceRefs
	logger  *slog.Logger
}

type healthResponse struct {
	Status string `json:"status"`
}

func (h *handler) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, h.logger, http.StatusNotFound, "not_found", "Unknown API route.")
}

func (h *handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if !h.rejectQueryParams(w, r) {
		return
	}
	writeJSON(w, h.logger, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *handler) handleFeed(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseRawQuery(w, r)
	if !ok {
		return
	}
	limit, err := parseFeedQuery(query)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	v, err := h.service.Feed(r.Context(), limit)
	writeResult(w, h.logger, v, err)
}

func (h *handler) handleJobs(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseRawQuery(w, r)
	if !ok {
		return
	}
	q, err := parseJobQuery(query)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_query", err.Error())
		return
	}
	ref := query.Get("source_ref")
	if ref != "" {
		sourceCtx, opened := h.refs.open(ref, q.Source)
		if !opened {
			writeError(w, h.logger, http.StatusGone, "recommendation_source_ref_unavailable", "Refresh recommendations to get a new source_ref.")
			return
		}
		q.SourceContext = sourceCtx
	}
	v, err := h.service.Jobs(r.Context(), q)
	if err != nil {
		writeResult(w, h.logger, nil, err)
		return
	}
	writeResult(w, h.logger, toJobResultsResponse(v, h.refs, ref), nil)
}

func (h *handler) handleJobLocations(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseRawQuery(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(query.Get("q"))
	if len(query) != 1 || len(query["q"]) != 1 || !linkedin.ValidLocationQuery(q) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_query", "q must contain at least 2 characters and at most 120 UTF-8 bytes, without control characters.")
		return
	}
	v, err := h.service.JobLocations(r.Context(), q)
	writeResult(w, h.logger, v, err)
}

func (h *handler) handleJobCompanies(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseRawQuery(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(query.Get("q"))
	if len(query) != 1 || len(query["q"]) != 1 || !linkedin.ValidJobCompanyQuery(q) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_query", "q must contain at least 2 characters and at most 120 UTF-8 bytes, without control characters.")
		return
	}
	v, err := h.service.JobCompanies(r.Context(), q)
	writeResult(w, h.logger, v, err)
}

func (h *handler) handleJob(w http.ResponseWriter, r *http.Request) {
	if !h.rejectQueryParams(w, r) {
		return
	}
	id := r.PathValue("id")
	if !jobIDPattern.MatchString(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_job_id", "Expected a numeric job ID.")
		return
	}
	v, err := h.service.Job(r.Context(), id)
	writeResult(w, h.logger, v, err)
}

func (h *handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	if !h.rejectQueryParams(w, r) {
		return
	}
	slug := r.PathValue("slug")
	if !linkedin.ValidSlug(slug) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_slug", "Expected a LinkedIn profile slug.")
		return
	}
	v, err := h.service.Profile(r.Context(), slug)
	writeResult(w, h.logger, v, err)
}

func (h *handler) handleCompany(w http.ResponseWriter, r *http.Request) {
	if !h.rejectQueryParams(w, r) {
		return
	}
	slug := r.PathValue("slug")
	if !linkedin.ValidSlug(slug) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_slug", "Expected a LinkedIn company slug.")
		return
	}
	v, err := h.service.Company(r.Context(), slug)
	writeResult(w, h.logger, v, err)
}

func (h *handler) parseRawQuery(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_query", "This route does not accept these query parameters.")
		return nil, false
	}
	return query, true
}

func (h *handler) rejectQueryParams(w http.ResponseWriter, r *http.Request) bool {
	query, ok := h.parseRawQuery(w, r)
	if !ok {
		return false
	}
	if len(query) > 0 {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_query", "This route does not accept these query parameters.")
		return false
	}
	return true
}

func parseFeedQuery(values url.Values) (int, error) {
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) != 1 || len(values["limit"]) != 1 {
		return 0, errors.New("Only one limit parameter is supported.")
	}
	limit, err := strconv.Atoi(values.Get("limit"))
	if err != nil || limit < 1 || limit > linkedin.MaxFeedLimit {
		return 0, errors.New("limit must be an integer from 1 to " + strconv.Itoa(linkedin.MaxFeedLimit) + ".")
	}
	return limit, nil
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
