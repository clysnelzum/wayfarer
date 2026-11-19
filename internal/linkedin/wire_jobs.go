package linkedin

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	maxRecommendationSections = 12
	maxDescriptionBlocks      = 128
	maxRelatedJobs            = 12
	maxTypeaheadSuggestions   = 15
)

type wireJobsResponse struct {
	Items                *[]wireJobSummary    `json:"items"`
	Sections             []wireJobSection     `json:"sections"`
	Page                 int                  `json:"page"`
	HasMore              bool                 `json:"has_more"`
	QueryEvidence        url.Values           `json:"query_evidence"`
	RecommendationStatus string               `json:"recommendation_status"`
	MatchMode            string               `json:"match_mode"`
	ResultCount          *int64               `json:"result_count"`
	ResultCountQualifier string               `json:"result_count_qualifier"`
	LocationLabel        string               `json:"location_label"`
	ResultCountLabel     string               `json:"result_count_label"`
	Notice               string               `json:"notice"`
	SelectedJobID        string               `json:"selected_job_id"`
	AlertEnabled         *bool                `json:"alert_enabled"`
	SuggestedFacets      []wireSuggestedFacet `json:"suggested_facets"`
}

type wireJobSection struct {
	Title         string            `json:"title"`
	JobIDs        []string          `json:"job_ids"`
	Source        string            `json:"source"`
	SourceContext *jobSourceContext `json:"source_context"`
}

type wireSuggestedFacet struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Selected *bool  `json:"selected"`
}

type wireJobSummary struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Company           string   `json:"company"`
	Location          string   `json:"location"`
	URL               string   `json:"url"`
	Workplace         string   `json:"workplace"`
	PostedLabel       string   `json:"posted_label"`
	CompensationLabel string   `json:"compensation_label"`
	BenefitsLabel     string   `json:"benefits_label"`
	NetworkLabel      string   `json:"network_label"`
	Verified          bool     `json:"verified"`
	Promoted          bool     `json:"promoted"`
	Viewed            bool     `json:"viewed"`
	Saved             bool     `json:"saved"`
	EasyApply         bool     `json:"easy_apply"`
	Signals           []string `json:"signals"`
}

type wireJobResponse struct {
	Job      *wireJob `json:"job"`
	NotFound bool     `json:"not_found"`
}

type wireJob struct {
	ID                     string                      `json:"id"`
	Title                  string                      `json:"title"`
	Company                string                      `json:"company"`
	URL                    string                      `json:"url"`
	Description            string                      `json:"description"`
	DescriptionBlocks      *[]wireJobDescriptionBlock  `json:"description_blocks"`
	CompanySlug            string                      `json:"company_slug"`
	Location               string                      `json:"location"`
	PostedLabel            string                      `json:"posted_label"`
	Workplace              string                      `json:"workplace"`
	EmploymentType         string                      `json:"employment_type"`
	ApplyMethod            string                      `json:"apply_method"`
	ApplicationStatus      string                      `json:"application_status"`
	CompensationLabel      string                      `json:"compensation_label"`
	SimilarJobAlertEnabled *bool                       `json:"similar_job_alert_enabled"`
	ApplicationActivity    *wireJobApplicationActivity `json:"application_activity"`
	ViewerApplication      *wireJobViewerApplication   `json:"viewer_application"`
	Verified               bool                        `json:"verified"`
	Promoted               bool                        `json:"promoted"`
	Saved                  *bool                       `json:"saved"`
	RelatedJobs            []wireJobSummary            `json:"related_jobs"`
}

type wireJobDescriptionBlock struct {
	Kind    string   `json:"kind"`
	Text    string   `json:"text"`
	Ordered *bool    `json:"ordered"`
	Items   []string `json:"items"`
}

type wireJobApplicationActivity struct {
	Kind      string `json:"kind"`
	Count     *int64 `json:"count"`
	Qualifier string `json:"qualifier"`
}

type wireJobViewerApplication struct {
	Status         string `json:"status"`
	SubmittedLabel string `json:"submitted_label"`
}

type wireTypeahead[Item any] struct {
	Items  *[]Item `json:"items"`
	Status string  `json:"status"`
}

type wireJobLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireJobCompany struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Industry string `json:"industry"`
}

func (w wireJobsResponse) toDomain(q JobQuery, path string, resolvedSourceCtx string) (JobResults, error) {
	const op = "jobs"
	if w.Page != q.Page {
		return JobResults{}, fmt.Errorf("%w: page", ErrSearchMismatch)
	}
	if q.Page > 0 {
		if err := validateObservedSearch(path, w.QueryEvidence); err != nil {
			return JobResults{}, err
		}
	}

	limit := searchPageSize
	if q.RecommendationLimit > 0 {
		limit = q.RecommendationLimit
	}
	if w.Items == nil {
		return JobResults{}, contractError(op, "missing items array", nil)
	}
	if len(*w.Items) > limit {
		return JobResults{}, contractError(op, "items exceed maximum page or limit size", nil)
	}

	items := make([]JobSummary, len(*w.Items))
	ids := make(map[string]struct{}, len(*w.Items))
	for i, raw := range *w.Items {
		summary, err := raw.toDomain(op)
		if err != nil {
			return JobResults{}, err
		}
		if _, duplicate := ids[summary.ID]; duplicate {
			return JobResults{}, contractError(op, "duplicate job id in items", nil)
		}
		ids[summary.ID] = struct{}{}
		items[i] = summary
	}

	result := JobResults{Items: items}
	if q.Page > 0 {
		searchInfo, err := w.buildSearchInfo(op, q.Source, resolvedSourceCtx)
		if err != nil {
			return JobResults{}, err
		}
		result.Search = searchInfo
		result.Pagination = &JobPagination{
			Page:    q.Page,
			HasMore: w.HasMore && len(items) > 0 && q.Page < MaxSearchPage,
		}
		// Only classify an empty page as out of range when a positive exact
		// total corroborates it. An approximate or missing total cannot do so.
		if q.Page > 1 && len(items) == 0 && w.MatchMode == "no_results" &&
			w.ResultCount != nil && w.ResultCountQualifier == "exact" {
			total := *w.ResultCount
			result.Pagination.OutOfRange = total > 0 && int64((q.Page-1)*searchPageSize) >= total
		}
		if result.Pagination.HasMore {
			result.Pagination.NextPage = q.Page + 1
		}
		return result, nil
	}

	if len(w.Sections) > maxRecommendationSections {
		return JobResults{}, contractError(op, "too many recommendation sections", nil)
	}
	if len(w.Sections) > 0 {
		sections := make([]JobSection, len(w.Sections))
		for i, sec := range w.Sections {
			converted, err := sec.toDomain(op, ids)
			if err != nil {
				return JobResults{}, err
			}
			sections[i] = converted
		}
		result.Sections = sections
	}

	if q.RecommendationLimit > 0 {
		switch w.RecommendationStatus {
		case "limit_reached", "budget_reached", "no_more_observed":
			if (len(items) == q.RecommendationLimit) != (w.RecommendationStatus == "limit_reached") {
				return JobResults{}, contractError(op, "inconsistent recommendation_status", nil)
			}
		default:
			return JobResults{}, contractError(op, "missing or invalid recommendation_status", nil)
		}
		result.RecommendationStatus = w.RecommendationStatus
	} else if w.RecommendationStatus != "" {
		return JobResults{}, contractError(op, "unexpected recommendation_status without limit", nil)
	}

	return result, nil
}

func (w wireJobsResponse) buildSearchInfo(op, source, sourceContext string) (*JobSearchInfo, error) {
	switch w.MatchMode {
	case "results", "broadened", "no_results":
	default:
		return nil, contractError(op, "missing or invalid search match_mode", nil)
	}
	if w.ResultCount != nil {
		if *w.ResultCount < 0 {
			return nil, contractError(op, "negative search result_count", nil)
		}
		if !validCountQualifier(w.ResultCountQualifier) {
			return nil, contractError(op, "missing or invalid result_count_qualifier", nil)
		}
	} else if w.ResultCountQualifier != "" && !validCountQualifier(w.ResultCountQualifier) {
		return nil, contractError(op, "invalid result_count_qualifier", nil)
	}
	if w.SelectedJobID != "" && !jobIDPattern.MatchString(w.SelectedJobID) {
		return nil, contractError(op, "invalid selected_job_id", nil)
	}
	if len(w.SuggestedFacets) > maxSuggestedFacets {
		return nil, contractError(op, "too many suggested_facets", nil)
	}

	var facets []JobSuggestedFacet
	if len(w.SuggestedFacets) > 0 {
		facets = make([]JobSuggestedFacet, len(w.SuggestedFacets))
		seen := make(map[string]struct{}, len(w.SuggestedFacets))
		for i, f := range w.SuggestedFacets {
			if !suggestedFacetID.MatchString(f.ID) || strings.TrimSpace(f.Label) == "" || f.Selected == nil {
				return nil, contractError(op, "invalid suggested_facet", nil)
			}
			if _, duplicate := seen[f.ID]; duplicate {
				return nil, contractError(op, "duplicate suggested_facet id", nil)
			}
			seen[f.ID] = struct{}{}
			facets[i] = JobSuggestedFacet{
				ID:       f.ID,
				Label:    f.Label,
				Selected: *f.Selected,
			}
		}
	}

	return &JobSearchInfo{
		Source:               source,
		SourceContext:        sourceContext,
		MatchMode:            w.MatchMode,
		ResultCount:          w.ResultCount,
		ResultCountQualifier: w.ResultCountQualifier,
		LocationLabel:        w.LocationLabel,
		ResultCountLabel:     w.ResultCountLabel,
		Notice:               w.Notice,
		SelectedJobID:        w.SelectedJobID,
		AlertEnabled:         w.AlertEnabled,
		SuggestedFacets:      facets,
	}, nil
}

func (w wireJobSection) toDomain(op string, itemIDs map[string]struct{}) (JobSection, error) {
	if w.Source != "" {
		if _, ok := recommendationOrigins[w.Source]; !ok {
			return JobSection{}, contractError(op, "invalid recommendation section source", nil)
		}
	}
	if strings.TrimSpace(w.Title) == "" || len(w.JobIDs) == 0 {
		return JobSection{}, contractError(op, "incomplete recommendation section", nil)
	}
	seenIDs := make(map[string]struct{}, len(w.JobIDs))
	jobIDs := make([]string, len(w.JobIDs))
	for i, id := range w.JobIDs {
		if _, ok := itemIDs[id]; !ok {
			return JobSection{}, contractError(op, "recommendation section refers to absent job", nil)
		}
		if _, duplicate := seenIDs[id]; duplicate {
			return JobSection{}, contractError(op, "duplicate job id in recommendation section", nil)
		}
		seenIDs[id] = struct{}{}
		jobIDs[i] = id
	}
	var encodedCtx string
	if w.SourceContext != nil && w.SourceContext.validFor(w.Source) {
		encoded, err := w.SourceContext.encode()
		if err != nil {
			return JobSection{}, err
		}
		encodedCtx = encoded
	}
	return JobSection{
		Title:         w.Title,
		Source:        w.Source,
		SourceContext: encodedCtx,
		JobIDs:        jobIDs,
	}, nil
}

func (w wireJobSummary) toDomain(op string) (JobSummary, error) {
	if !jobIDPattern.MatchString(w.ID) {
		return JobSummary{}, contractError(op, "invalid job summary id", nil)
	}
	if strings.TrimSpace(w.Title) == "" {
		return JobSummary{}, contractError(op, "missing job summary title", nil)
	}
	if strings.TrimSpace(w.Company) == "" {
		return JobSummary{}, contractError(op, "missing job summary company", nil)
	}
	if strings.TrimSpace(w.Location) == "" {
		return JobSummary{}, contractError(op, "missing job summary location", nil)
	}
	if !validHTTPURL(w.URL) {
		return JobSummary{}, contractError(op, "invalid job summary url", nil)
	}
	if w.Workplace != "" && !validWorkplace(w.Workplace) {
		return JobSummary{}, contractError(op, "invalid job summary workplace", nil)
	}
	var signals []string
	if len(w.Signals) > 0 {
		signals = make([]string, len(w.Signals))
		seen := make(map[string]struct{}, len(w.Signals))
		for i, sig := range w.Signals {
			if sig != "early_applicant" && sig != "actively_reviewing" {
				return JobSummary{}, contractError(op, "invalid job summary signal", nil)
			}
			if _, duplicate := seen[sig]; duplicate {
				return JobSummary{}, contractError(op, "duplicate job summary signal", nil)
			}
			seen[sig] = struct{}{}
			signals[i] = sig
		}
	}

	return JobSummary{
		ID:                w.ID,
		Title:             w.Title,
		Company:           w.Company,
		Location:          w.Location,
		URL:               w.URL,
		Workplace:         w.Workplace,
		PostedLabel:       w.PostedLabel,
		CompensationLabel: w.CompensationLabel,
		BenefitsLabel:     w.BenefitsLabel,
		NetworkLabel:      w.NetworkLabel,
		Verified:          w.Verified,
		Promoted:          w.Promoted,
		Viewed:            w.Viewed,
		Saved:             w.Saved,
		EasyApply:         w.EasyApply,
		Signals:           signals,
	}, nil
}

func (w wireJobResponse) toDomain(requestedID, baseURL string) (Job, error) {
	const op = "job"
	if w.NotFound {
		return Job{}, ErrJobNotFound
	}
	if w.Job == nil {
		return Job{}, contractError(op, "missing job object", nil)
	}
	j := w.Job
	if j.ID != requestedID {
		return Job{}, contractError(op, "mismatched job id", nil)
	}
	if strings.TrimSpace(j.Title) == "" {
		return Job{}, contractError(op, "missing job title", nil)
	}
	if strings.TrimSpace(j.Company) == "" {
		return Job{}, contractError(op, "missing job company", nil)
	}
	if strings.TrimSpace(j.Description) == "" {
		return Job{}, contractError(op, "missing job description", nil)
	}
	if j.CompanySlug != "" && !ValidSlug(j.CompanySlug) {
		return Job{}, contractError(op, "invalid job company_slug", nil)
	}
	if j.Workplace != "" && !validWorkplace(j.Workplace) {
		return Job{}, contractError(op, "invalid job workplace", nil)
	}
	if j.EmploymentType != "" && !validEmploymentType(j.EmploymentType) {
		return Job{}, contractError(op, "invalid job employment_type", nil)
	}
	if j.ApplyMethod != "" && j.ApplyMethod != "external" && j.ApplyMethod != "easy_apply" {
		return Job{}, contractError(op, "invalid job apply_method", nil)
	}
	if j.ApplicationStatus != "" && j.ApplicationStatus != "open" && j.ApplicationStatus != "closed" {
		return Job{}, contractError(op, "invalid job application_status", nil)
	}

	var blocks []JobDescriptionBlock
	if j.DescriptionBlocks != nil {
		rawBlocks := *j.DescriptionBlocks
		if len(rawBlocks) == 0 || len(rawBlocks) > maxDescriptionBlocks {
			return Job{}, contractError(op, "invalid description_blocks length", nil)
		}
		blocks = make([]JobDescriptionBlock, len(rawBlocks))
		for i, b := range rawBlocks {
			converted, err := b.toDomain(op)
			if err != nil {
				return Job{}, err
			}
			blocks[i] = converted
		}
	}

	var activity *JobApplicationActivity
	if j.ApplicationActivity != nil {
		a := j.ApplicationActivity
		if a.Kind != "apply_clicks" && a.Kind != "applications" {
			return Job{}, contractError(op, "invalid application_activity kind", nil)
		}
		if a.Count == nil || *a.Count < 0 {
			return Job{}, contractError(op, "missing or negative application_activity count", nil)
		}
		if !validCountQualifier(a.Qualifier) {
			return Job{}, contractError(op, "invalid application_activity qualifier", nil)
		}
		activity = &JobApplicationActivity{
			Kind:      a.Kind,
			Count:     *a.Count,
			Qualifier: a.Qualifier,
		}
	}

	var viewerApp *JobViewerApplication
	if j.ViewerApplication != nil {
		if j.ViewerApplication.Status != "submitted" {
			return Job{}, contractError(op, "invalid viewer_application status", nil)
		}
		viewerApp = &JobViewerApplication{
			Status:         j.ViewerApplication.Status,
			SubmittedLabel: j.ViewerApplication.SubmittedLabel,
		}
	}

	var related []JobSummary
	if len(j.RelatedJobs) > maxRelatedJobs {
		return Job{}, contractError(op, "too many related_jobs", nil)
	}
	if len(j.RelatedJobs) > 0 {
		related = make([]JobSummary, len(j.RelatedJobs))
		seen := make(map[string]struct{}, len(j.RelatedJobs))
		for i, r := range j.RelatedJobs {
			summary, err := r.toDomain(op)
			if err != nil {
				return Job{}, err
			}
			if summary.ID == requestedID {
				return Job{}, contractError(op, "related_jobs includes primary job id", nil)
			}
			if _, duplicate := seen[summary.ID]; duplicate {
				return Job{}, contractError(op, "duplicate related_jobs id", nil)
			}
			seen[summary.ID] = struct{}{}
			related[i] = summary
		}
	}

	return Job{
		ID:                     j.ID,
		Title:                  j.Title,
		Company:                j.Company,
		URL:                    baseURL + "/jobs/view/" + j.ID + "/",
		Description:            j.Description,
		DescriptionBlocks:      blocks,
		CompanySlug:            j.CompanySlug,
		Location:               j.Location,
		PostedLabel:            j.PostedLabel,
		Workplace:              j.Workplace,
		EmploymentType:         j.EmploymentType,
		ApplyMethod:            j.ApplyMethod,
		ApplicationStatus:      j.ApplicationStatus,
		CompensationLabel:      j.CompensationLabel,
		SimilarJobAlertEnabled: j.SimilarJobAlertEnabled,
		ApplicationActivity:    activity,
		ViewerApplication:      viewerApp,
		Verified:               j.Verified,
		Promoted:               j.Promoted,
		Saved:                  j.Saved,
		RelatedJobs:            related,
	}, nil
}

func (b wireJobDescriptionBlock) toDomain(op string) (JobDescriptionBlock, error) {
	switch b.Kind {
	case "paragraph", "heading":
		if strings.TrimSpace(b.Text) == "" || b.Ordered != nil || len(b.Items) > 0 {
			return JobDescriptionBlock{}, contractError(op, "invalid text description block", nil)
		}
		return JobDescriptionBlock{
			Kind: b.Kind,
			Text: b.Text,
		}, nil
	case "list":
		if b.Ordered == nil || len(b.Items) == 0 || len(b.Items) > maxDescriptionBlocks || b.Text != "" {
			return JobDescriptionBlock{}, contractError(op, "invalid list description block", nil)
		}
		items := make([]string, len(b.Items))
		for i, item := range b.Items {
			if strings.TrimSpace(item) == "" {
				return JobDescriptionBlock{}, contractError(op, "empty list description block item", nil)
			}
			items[i] = item
		}
		return JobDescriptionBlock{
			Kind:    b.Kind,
			Ordered: b.Ordered,
			Items:   items,
		}, nil
	default:
		return JobDescriptionBlock{}, contractError(op, "invalid description block kind", nil)
	}
}

func validWorkplace(w string) bool {
	switch w {
	case "remote", "hybrid", "onsite":
		return true
	default:
		return false
	}
}

func validEmploymentType(t string) bool {
	switch t {
	case "full_time", "part_time", "contract", "internship", "volunteer", "temporary":
		return true
	default:
		return false
	}
}

func validCountQualifier(q string) bool {
	switch q {
	case "exact", "more_than", "at_least":
		return true
	default:
		return false
	}
}
