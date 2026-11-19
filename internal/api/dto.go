package api

import "github.com/clysnelzum/wayfarer/internal/linkedin"

type jobResultsResponse struct {
	Items                []linkedin.JobSummary   `json:"items"`
	RecommendationStatus string                  `json:"recommendation_status,omitempty"`
	Sections             []jobSectionResponse    `json:"sections,omitempty"`
	Pagination           *linkedin.JobPagination `json:"pagination,omitempty"`
	Search               *jobSearchInfoResponse  `json:"search,omitempty"`
}

type jobSectionResponse struct {
	Title     string   `json:"title"`
	Source    string   `json:"source,omitempty"`
	SourceRef string   `json:"source_ref,omitempty"`
	JobIDs    []string `json:"job_ids"`
}

type jobSearchInfoResponse struct {
	Source               string                       `json:"source,omitempty"`
	SourceRef            string                       `json:"source_ref,omitempty"`
	MatchMode            string                       `json:"match_mode"`
	ResultCount          *int64                       `json:"result_count,omitempty"`
	ResultCountQualifier string                       `json:"result_count_qualifier,omitempty"`
	LocationLabel        string                       `json:"location_label,omitempty"`
	ResultCountLabel     string                       `json:"result_count_label,omitempty"`
	Notice               string                       `json:"notice,omitempty"`
	SelectedJobID        string                       `json:"selected_job_id,omitempty"`
	AlertEnabled         *bool                        `json:"alert_enabled,omitempty"`
	SuggestedFacets      []linkedin.JobSuggestedFacet `json:"suggested_facets,omitempty"`
}

func toJobResultsResponse(v linkedin.JobResults, refs *sourceRefs, requestRef string) jobResultsResponse {
	items := v.Items
	if items == nil {
		items = []linkedin.JobSummary{}
	}
	resp := jobResultsResponse{
		Items:                items,
		RecommendationStatus: v.RecommendationStatus,
		Pagination:           v.Pagination,
	}
	if len(v.Sections) > 0 {
		resp.Sections = make([]jobSectionResponse, len(v.Sections))
		for i, sec := range v.Sections {
			var sourceRef string
			if sec.SourceContext != "" {
				sourceRef = refs.seal(sec.Source, sec.SourceContext)
			}
			resp.Sections[i] = jobSectionResponse{
				Title:     sec.Title,
				Source:    sec.Source,
				SourceRef: sourceRef,
				JobIDs:    sec.JobIDs,
			}
		}
	}
	if v.Search != nil {
		var sourceRef string
		if v.Search.SourceContext != "" {
			sourceRef = requestRef
			if sourceRef == "" {
				sourceRef = refs.seal(v.Search.Source, v.Search.SourceContext)
			}
		}
		resp.Search = &jobSearchInfoResponse{
			Source:               v.Search.Source,
			SourceRef:            sourceRef,
			MatchMode:            v.Search.MatchMode,
			ResultCount:          v.Search.ResultCount,
			ResultCountQualifier: v.Search.ResultCountQualifier,
			LocationLabel:        v.Search.LocationLabel,
			ResultCountLabel:     v.Search.ResultCountLabel,
			Notice:               v.Search.Notice,
			SelectedJobID:        v.Search.SelectedJobID,
			AlertEnabled:         v.Search.AlertEnabled,
			SuggestedFacets:      v.Search.SuggestedFacets,
		}
	}
	return resp
}
