package linkedin

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"
)

//go:embed jobs.js
var jobsExpression string

//go:embed job.js
var jobExpression string

//go:embed job_cards.js
var jobCardExpression string

func jobScript(extractor string) string {
	return "() => (" + extractor + ")((" + jobCardExpression + "))"
}

const base = "https://www.linkedin.com"

var ErrNotFound = errors.New("job not found")

type Reader interface {
	Read(ctx context.Context, pageURL, expression string) (json.RawMessage, error)
}

type Client struct{ Reader Reader }

type pageReadFunc func(context.Context, string, string) (json.RawMessage, error)

func (f pageReadFunc) Read(ctx context.Context, path, expression string) (json.RawMessage, error) {
	return f(ctx, path, expression)
}

func renderReader(reader Reader) Reader {
	if rendering, ok := reader.(interface {
		ReadRendered(context.Context, string, string) (json.RawMessage, error)
	}); ok {
		return pageReadFunc(rendering.ReadRendered)
	}
	return reader
}

type JobResults struct {
	Items                []JobSummary   `json:"items"`
	Pagination           *JobPagination `json:"pagination,omitempty"`
	Search               *JobSearchInfo `json:"search,omitempty"`
	Sections             []JobSection   `json:"sections,omitempty"`
	RecommendationStatus string         `json:"recommendation_status,omitempty"`
}

type JobSection struct {
	Title         string            `json:"title"`
	JobIDs        []string          `json:"job_ids"`
	Source        string            `json:"source,omitempty"`
	SourceRef     string            `json:"source_ref,omitempty"`
	SourceContext *JobSourceContext `json:"-"`
}

type JobPagination struct {
	Page       int  `json:"page"`
	HasMore    bool `json:"has_more"`
	NextPage   int  `json:"next_page,omitempty"`
	OutOfRange bool `json:"out_of_range,omitempty"`
}

type JobSearchInfo struct {
	Source               string              `json:"source,omitempty"`
	SourceRef            string              `json:"source_ref,omitempty"`
	SourceContext        *JobSourceContext   `json:"-"`
	MatchMode            string              `json:"match_mode"`
	ResultCount          *int64              `json:"result_count,omitempty"`
	ResultCountQualifier string              `json:"result_count_qualifier,omitempty"`
	LocationLabel        string              `json:"location_label,omitempty"`
	ResultCountLabel     string              `json:"result_count_label,omitempty"`
	Notice               string              `json:"notice,omitempty"`
	SelectedJobID        string              `json:"selected_job_id,omitempty"`
	SuggestedFacets      []JobSuggestedFacet `json:"suggested_facets,omitempty"`
}

type JobSuggestedFacet struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
}

type JobSummary struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Company     string   `json:"company"`
	Location    string   `json:"location"`
	URL         string   `json:"url"`
	Workplace   string   `json:"workplace,omitempty"`
	PostedLabel string   `json:"posted_label,omitempty"`
	Verified    bool     `json:"verified,omitempty"`
	Promoted    bool     `json:"promoted,omitempty"`
	Viewed      bool     `json:"viewed,omitempty"`
	Saved       bool     `json:"saved,omitempty"`
	EasyApply   bool     `json:"easy_apply,omitempty"`
	Signals     []string `json:"signals,omitempty"`
}

type Job struct {
	ID                  string                  `json:"id"`
	Title               string                  `json:"title"`
	Company             string                  `json:"company"`
	URL                 string                  `json:"url"`
	Description         string                  `json:"description"`
	CompanySlug         string                  `json:"company_slug,omitempty"`
	Location            string                  `json:"location,omitempty"`
	PostedLabel         string                  `json:"posted_label,omitempty"`
	Workplace           string                  `json:"workplace,omitempty"`
	EmploymentType      string                  `json:"employment_type,omitempty"`
	ApplyMethod         string                  `json:"apply_method,omitempty"`
	ApplicationStatus   string                  `json:"application_status,omitempty"`
	ApplicationActivity *JobApplicationActivity `json:"application_activity,omitempty"`
	ViewerApplication   *JobViewerApplication   `json:"viewer_application,omitempty"`
	Verified            bool                    `json:"verified,omitempty"`
	Promoted            bool                    `json:"promoted,omitempty"`
	Saved               *bool                   `json:"saved,omitempty"`
	RelatedJobs         []JobSummary            `json:"related_jobs,omitempty"`
}

type JobApplicationActivity struct {
	Kind      string `json:"kind"`
	Count     int64  `json:"count"`
	Qualifier string `json:"qualifier"`
}

type JobViewerApplication struct {
	Status         string `json:"status"`
	SubmittedLabel string `json:"submitted_label,omitempty"`
}

func read[T any](ctx context.Context, r Reader, path, expr string) (T, error) {
	var value T
	body, err := r.Read(ctx, base+path, expr)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return value, fmt.Errorf("decode browser result: %w", err)
	}
	return value, nil
}

func (c Client) Jobs(ctx context.Context, q JobQuery) (JobResults, error) {
	path, err := q.searchPath()
	if err != nil {
		return JobResults{}, err
	}
	link := q.SourceContext
	if q.Source != "" {
		if link == nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 26*time.Second)
			defer cancel()
			resolved, err := c.resolveJobSource(ctx, q.Source)
			if err != nil {
				return JobResults{}, err
			}
			if !resolved.validFor(q.Source) {
				return JobResults{}, errors.New("invalid recommendation search link from Chrome")
			}
			link = &resolved
		}
		path = link.searchPath(q.Page)
	}
	reader := c.Reader
	if q.RecommendationLimit > 0 {
		reader = renderReader(reader)
	}
	type upstreamSection struct {
		JobSection
		Context *JobSourceContext `json:"source_context"`
	}
	upstream, err := read[struct {
		Items                []JobSummary      `json:"items"`
		Sections             []upstreamSection `json:"sections,omitempty"`
		Page                 int               `json:"page"`
		HasMore              bool              `json:"has_more"`
		QueryEvidence        url.Values        `json:"query_evidence"`
		RecommendationStatus string            `json:"recommendation_status"`
		JobSearchInfo
	}](ctx, reader, path, jobListScript(q.RecommendationLimit, ""))
	if err != nil {
		return JobResults{}, err
	}
	if upstream.Page != q.Page {
		return JobResults{}, fmt.Errorf("%w: page", ErrSearchMismatch)
	}
	limit := searchPageSize
	if q.RecommendationLimit > 0 {
		limit = q.RecommendationLimit
	}
	if upstream.Items == nil || len(upstream.Items) > limit {
		return JobResults{}, errors.New("unexpected size for browser jobs result")
	}
	ids := make(map[string]struct{}, len(upstream.Items))
	for _, item := range upstream.Items {
		if _, duplicate := ids[item.ID]; item.ID == "" || duplicate {
			return JobResults{}, errors.New("missing or duplicate job ID in browser result")
		}
		ids[item.ID] = struct{}{}
	}
	result := JobResults{Items: upstream.Items}
	if q.Page > 0 {
		if err := validateObservedSearch(path, upstream.QueryEvidence); err != nil {
			return JobResults{}, err
		}
		if upstream.MatchMode != "" {
			if len(upstream.SuggestedFacets) > maxSuggestedFacets {
				return JobResults{}, errors.New("unexpected number of suggested job facets")
			}
			seen := make(map[string]bool, len(upstream.SuggestedFacets))
			for _, facet := range upstream.SuggestedFacets {
				if !suggestedFacetID.MatchString(facet.ID) || facet.Label == "" || seen[facet.ID] {
					return JobResults{}, errors.New("invalid or duplicate suggested job facet")
				}
				seen[facet.ID] = true
			}
			upstream.JobSearchInfo.Source = q.Source
			upstream.JobSearchInfo.SourceRef = ""
			upstream.JobSearchInfo.SourceContext = link
			result.Search = &upstream.JobSearchInfo
		}
		result.Pagination = &JobPagination{Page: q.Page, HasMore: upstream.HasMore && len(upstream.Items) > 0 && q.Page < MaxSearchPage}
		// Only classify an empty page as out of range when a positive exact
		// total corroborates it. An approximate or missing total cannot do so.
		if q.Page > 1 && len(upstream.Items) == 0 && upstream.MatchMode == "no_results" &&
			upstream.ResultCount != nil && upstream.ResultCountQualifier == "exact" {
			total := *upstream.ResultCount
			result.Pagination.OutOfRange = total > 0 && int64((q.Page-1)*searchPageSize) >= total
		}
		if result.Pagination.HasMore {
			result.Pagination.NextPage = q.Page + 1
		}
	} else {
		for _, section := range upstream.Sections {
			if _, ok := recommendationOrigins[section.Source]; section.Source != "" && !ok {
				return JobResults{}, errors.New("invalid recommendation section source")
			}
			if section.Title == "" || len(section.JobIDs) == 0 {
				return JobResults{}, errors.New("incomplete recommendation section")
			}
			for _, id := range section.JobIDs {
				if _, ok := ids[id]; !ok {
					return JobResults{}, errors.New("recommendation section refers to an absent job")
				}
			}
			section.SourceRef = ""
			if section.Context != nil && section.Context.validFor(section.Source) {
				section.SourceContext = section.Context
			}
			result.Sections = append(result.Sections, section.JobSection)
		}
		if q.RecommendationLimit > 0 {
			switch upstream.RecommendationStatus {
			case "limit_reached", "budget_reached", "no_more_observed":
				if (len(upstream.Items) == q.RecommendationLimit) != (upstream.RecommendationStatus == "limit_reached") {
					return JobResults{}, errors.New("inconsistent recommendation load status")
				}
			default:
				return JobResults{}, errors.New("missing or invalid recommendation load status")
			}
			result.RecommendationStatus = upstream.RecommendationStatus
		}
	}
	return result, nil
}

func (c Client) Job(ctx context.Context, id string) (Job, error) {
	v, err := read[struct {
		Job      Job  `json:"job"`
		NotFound bool `json:"not_found"`
	}](ctx, c.Reader, "/jobs/view/"+id+"/", jobScript(jobExpression))
	if err != nil {
		return Job{}, err
	}
	if v.NotFound {
		return Job{}, ErrNotFound
	}
	if v.Job.ID != id || v.Job.Description == "" || v.Job.Title == "" {
		return Job{}, errors.New("unexpected job in browser result")
	}
	return v.Job, nil
}
