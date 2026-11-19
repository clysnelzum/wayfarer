package linkedin

// MaxSearchPage is the highest one-based job search results page supported.
const MaxSearchPage = 100

// MaxJobRecommendationLimit is the maximum number of job recommendations that may be requested.
const MaxJobRecommendationLimit = 50

// JobQuery describes a LinkedIn job recommendations, recommendation-source, or keyword search request.
type JobQuery struct {
	Keywords            string
	Source              string
	SourceContext       string
	Page                int
	RecommendationLimit int
	Filters             JobFilters
}

// JobFilters specifies optional LinkedIn job search filters requiring Keywords.
type JobFilters struct {
	LocationID        string
	LocationIDs       []string
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

// JobResults holds job cards and accompanying search or recommendation metadata.
type JobResults struct {
	Items                []JobSummary   `json:"items"`
	RecommendationStatus string         `json:"recommendation_status,omitempty"`
	Sections             []JobSection   `json:"sections,omitempty"`
	Pagination           *JobPagination `json:"pagination,omitempty"`
	Search               *JobSearchInfo `json:"search,omitempty"`
}

// JobSection groups recommended jobs displayed under a LinkedIn home section heading.
type JobSection struct {
	Title         string   `json:"title"`
	Source        string   `json:"source,omitempty"`
	SourceContext string   `json:"-"`
	JobIDs        []string `json:"job_ids"`
}

// JobPagination summarizes page state for a paginated job search.
type JobPagination struct {
	Page       int  `json:"page"`
	HasMore    bool `json:"has_more"`
	NextPage   int  `json:"next_page,omitempty"`
	OutOfRange bool `json:"out_of_range,omitempty"`
}

// JobSearchInfo carries observed metadata and dynamic filter suggestions for a job search.
type JobSearchInfo struct {
	Source               string              `json:"source,omitempty"`
	SourceContext        string              `json:"-"`
	MatchMode            string              `json:"match_mode"`
	ResultCount          *int64              `json:"result_count,omitempty"`
	ResultCountQualifier string              `json:"result_count_qualifier,omitempty"`
	LocationLabel        string              `json:"location_label,omitempty"`
	ResultCountLabel     string              `json:"result_count_label,omitempty"`
	Notice               string              `json:"notice,omitempty"`
	SelectedJobID        string              `json:"selected_job_id,omitempty"`
	AlertEnabled         *bool               `json:"alert_enabled,omitempty"`
	SuggestedFacets      []JobSuggestedFacet `json:"suggested_facets,omitempty"`
}

// JobSuggestedFacet describes a dynamic filter pill observed in the job search toolbar.
type JobSuggestedFacet struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
}

// JobSummary represents a job card returned in recommendations, search results, or related jobs.
type JobSummary struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Company           string   `json:"company"`
	Location          string   `json:"location"`
	URL               string   `json:"url"`
	Workplace         string   `json:"workplace,omitempty"`
	PostedLabel       string   `json:"posted_label,omitempty"`
	CompensationLabel string   `json:"compensation_label,omitempty"`
	BenefitsLabel     string   `json:"benefits_label,omitempty"`
	NetworkLabel      string   `json:"network_label,omitempty"`
	Verified          bool     `json:"verified,omitempty"`
	Promoted          bool     `json:"promoted,omitempty"`
	Viewed            bool     `json:"viewed,omitempty"`
	Saved             bool     `json:"saved,omitempty"`
	EasyApply         bool     `json:"easy_apply,omitempty"`
	Signals           []string `json:"signals,omitempty"`
}

// Job represents the full details of a single LinkedIn job posting.
type Job struct {
	ID                     string                  `json:"id"`
	Title                  string                  `json:"title"`
	Company                string                  `json:"company"`
	URL                    string                  `json:"url"`
	Description            string                  `json:"description"`
	DescriptionBlocks      []JobDescriptionBlock   `json:"description_blocks,omitempty"`
	CompanySlug            string                  `json:"company_slug,omitempty"`
	Location               string                  `json:"location,omitempty"`
	PostedLabel            string                  `json:"posted_label,omitempty"`
	Workplace              string                  `json:"workplace,omitempty"`
	EmploymentType         string                  `json:"employment_type,omitempty"`
	ApplyMethod            string                  `json:"apply_method,omitempty"`
	ApplicationStatus      string                  `json:"application_status,omitempty"`
	CompensationLabel      string                  `json:"compensation_label,omitempty"`
	SimilarJobAlertEnabled *bool                   `json:"similar_job_alert_enabled,omitempty"`
	ApplicationActivity    *JobApplicationActivity `json:"application_activity,omitempty"`
	ViewerApplication      *JobViewerApplication   `json:"viewer_application,omitempty"`
	Verified               bool                    `json:"verified,omitempty"`
	Promoted               bool                    `json:"promoted,omitempty"`
	Saved                  *bool                   `json:"saved,omitempty"`
	RelatedJobs            []JobSummary            `json:"related_jobs,omitempty"`
}

// JobDescriptionBlock represents a plain-text paragraph, heading, or flat list from a job description.
type JobDescriptionBlock struct {
	Kind    string   `json:"kind"`
	Text    string   `json:"text,omitempty"`
	Ordered *bool    `json:"ordered,omitempty"`
	Items   []string `json:"items,omitempty"`
}

// JobApplicationActivity summarizes observed applicant or apply-click counts for a job posting.
type JobApplicationActivity struct {
	Kind      string `json:"kind"`
	Count     int64  `json:"count"`
	Qualifier string `json:"qualifier"`
}

// JobViewerApplication describes the signed-in viewer's observed application status for a job.
type JobViewerApplication struct {
	Status         string `json:"status"`
	SubmittedLabel string `json:"submitted_label,omitempty"`
}

// JobLocations holds location typeahead suggestions returned by LinkedIn.
type JobLocations struct {
	Items  []JobLocation `json:"items"`
	Status string        `json:"status"`
}

// JobLocation represents a single LinkedIn geography autocomplete suggestion.
type JobLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// JobCompanies holds company typeahead suggestions returned by LinkedIn.
type JobCompanies struct {
	Items  []JobCompany `json:"items"`
	Status string       `json:"status"`
}

// JobCompany represents a single LinkedIn organization autocomplete suggestion.
type JobCompany struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Industry string `json:"industry,omitempty"`
}
