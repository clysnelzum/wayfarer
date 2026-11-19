package linkedin

import "strings"

const (
	maxCompanyHighlights   = 10
	maxCompanyBenefits     = 20
	maxCompanyCommitments  = 10
	maxCompanyLocations    = 50
	maxCompanyRelatedPages = 15
)

type wireCompanyResponse struct {
	Company  *wireCompany `json:"company"`
	NotFound bool         `json:"not_found"`
}

type wireCompany struct {
	Slug                   string                      `json:"slug"`
	URL                    string                      `json:"url"`
	Name                   string                      `json:"name"`
	ID                     string                      `json:"id"`
	Tagline                string                      `json:"tagline"`
	Description            string                      `json:"description"`
	Website                string                      `json:"website"`
	Phone                  string                      `json:"phone"`
	Industry               string                      `json:"industry"`
	CompanySize            string                      `json:"company_size"`
	AssociatedMembersCount *int64                      `json:"associated_members_count"`
	AssociatedMembersLabel string                      `json:"associated_members_label"`
	FollowersCount         *int64                      `json:"followers_count"`
	FollowersLabel         string                      `json:"followers_label"`
	EmployeesLabel         string                      `json:"employees_label"`
	Headquarters           string                      `json:"headquarters"`
	Founded                string                      `json:"founded"`
	CompanyType            string                      `json:"company_type"`
	Specialties            string                      `json:"specialties"`
	Verified               bool                        `json:"verified"`
	VerifiedPage           string                      `json:"verified_page"`
	Premium                bool                        `json:"premium"`
	LogoURL                string                      `json:"logo_url"`
	CoverImageURL          string                      `json:"cover_image_url"`
	Highlights             []string                    `json:"highlights"`
	WorkplacePolicy        *wireCompanyWorkplacePolicy `json:"workplace_policy"`
	Commitments            []wireCompanyCommitment     `json:"commitments"`
	LocationsCount         *int                        `json:"locations_count"`
	Locations              []wireCompanyLocation       `json:"locations"`
	AffiliatedPages        []wireCompanyRelatedPage    `json:"affiliated_pages"`
	SimilarPages           []wireCompanyRelatedPage    `json:"similar_pages"`
}

type wireCompanyWorkplacePolicy struct {
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	DefaultPresence string   `json:"default_presence"`
	Benefits        []string `json:"benefits"`
}

type wireCompanyCommitment struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type wireCompanyLocation struct {
	Label          string `json:"label"`
	Address        string `json:"address"`
	IsHeadquarters bool   `json:"is_headquarters"`
	MapURL         string `json:"map_url"`
}

type wireCompanyRelatedPage struct {
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	Name           string `json:"name"`
	Industry       string `json:"industry"`
	FollowersLabel string `json:"followers_label"`
}

func (w wireCompanyResponse) toDomain(requestedSlug, baseURL string) (Company, error) {
	const op = "company"
	if w.NotFound {
		return Company{}, ErrCompanyNotFound
	}
	if w.Company == nil {
		return Company{}, contractError(op, "missing company object", nil)
	}
	co := w.Company
	if !ValidSlug(co.Slug) || !strings.EqualFold(co.Slug, requestedSlug) || strings.TrimSpace(co.Name) == "" {
		return Company{}, contractError(op, "invalid or mismatched company slug/name", nil)
	}
	if co.ID != "" && !companyIDPattern.MatchString(co.ID) {
		return Company{}, contractError(op, "invalid company id", nil)
	}
	if co.AssociatedMembersCount != nil && *co.AssociatedMembersCount < 0 {
		return Company{}, contractError(op, "negative associated_members_count", nil)
	}
	if co.FollowersCount != nil && *co.FollowersCount < 0 {
		return Company{}, contractError(op, "negative followers_count", nil)
	}
	if co.LocationsCount != nil && *co.LocationsCount < 0 {
		return Company{}, contractError(op, "negative locations_count", nil)
	}
	for _, rawURL := range []string{co.LogoURL, co.CoverImageURL} {
		if rawURL != "" && !validHTTPURL(rawURL) {
			return Company{}, contractError(op, "invalid company url field", nil)
		}
	}

	highlights, err := validateNonEmptyStrings(op, "highlights", co.Highlights, maxCompanyHighlights)
	if err != nil {
		return Company{}, err
	}

	var workplacePolicy *CompanyWorkplacePolicy
	if co.WorkplacePolicy != nil {
		if strings.TrimSpace(co.WorkplacePolicy.Title) == "" {
			return Company{}, contractError(op, "missing workplace_policy title", nil)
		}
		benefits, err := validateNonEmptyStrings(op, "workplace_policy.benefits", co.WorkplacePolicy.Benefits, maxCompanyBenefits)
		if err != nil {
			return Company{}, err
		}
		workplacePolicy = &CompanyWorkplacePolicy{
			Title:           co.WorkplacePolicy.Title,
			Description:     co.WorkplacePolicy.Description,
			DefaultPresence: co.WorkplacePolicy.DefaultPresence,
			Benefits:        benefits,
		}
	}

	var commitments []CompanyCommitment
	if len(co.Commitments) > maxCompanyCommitments {
		return Company{}, contractError(op, "commitments exceeds max items", nil)
	}
	if len(co.Commitments) > 0 {
		commitments = make([]CompanyCommitment, len(co.Commitments))
		for i, cm := range co.Commitments {
			if strings.TrimSpace(cm.Title) == "" {
				return Company{}, contractError(op, "missing commitment title", nil)
			}
			commitments[i] = CompanyCommitment{Title: cm.Title, Description: cm.Description}
		}
	}

	var locations []CompanyLocation
	if len(co.Locations) > maxCompanyLocations {
		return Company{}, contractError(op, "locations exceeds max items", nil)
	}
	if len(co.Locations) > 0 {
		locations = make([]CompanyLocation, len(co.Locations))
		for i, loc := range co.Locations {
			if strings.TrimSpace(loc.Address) == "" && strings.TrimSpace(loc.Label) == "" {
				return Company{}, contractError(op, "empty company location", nil)
			}
			locations[i] = CompanyLocation{
				Label:          loc.Label,
				Address:        loc.Address,
				IsHeadquarters: loc.IsHeadquarters,
				MapURL:         optionalHTTPURL(loc.MapURL),
			}
		}
	}

	affiliated, err := convertCompanyRelatedPages(op, "affiliated_pages", co.AffiliatedPages)
	if err != nil {
		return Company{}, err
	}
	similar, err := convertCompanyRelatedPages(op, "similar_pages", co.SimilarPages)
	if err != nil {
		return Company{}, err
	}

	return Company{
		Slug:                   co.Slug,
		URL:                    baseURL + "/company/" + co.Slug + "/",
		Name:                   co.Name,
		ID:                     co.ID,
		Tagline:                co.Tagline,
		Description:            co.Description,
		Website:                optionalHTTPURL(co.Website),
		Phone:                  co.Phone,
		Industry:               co.Industry,
		CompanySize:            co.CompanySize,
		AssociatedMembersCount: co.AssociatedMembersCount,
		AssociatedMembersLabel: co.AssociatedMembersLabel,
		FollowersCount:         co.FollowersCount,
		FollowersLabel:         co.FollowersLabel,
		EmployeesLabel:         co.EmployeesLabel,
		Headquarters:           co.Headquarters,
		Founded:                co.Founded,
		CompanyType:            co.CompanyType,
		Specialties:            co.Specialties,
		Verified:               co.Verified,
		VerifiedPage:           co.VerifiedPage,
		Premium:                co.Premium,
		LogoURL:                co.LogoURL,
		CoverImageURL:          co.CoverImageURL,
		Highlights:             highlights,
		WorkplacePolicy:        workplacePolicy,
		Commitments:            commitments,
		LocationsCount:         co.LocationsCount,
		Locations:              locations,
		AffiliatedPages:        affiliated,
		SimilarPages:           similar,
	}, nil
}

func convertCompanyRelatedPages(op, field string, raw []wireCompanyRelatedPage) ([]CompanyRelatedPage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxCompanyRelatedPages {
		return nil, contractError(op, field+" exceeds max items", nil)
	}
	out := make([]CompanyRelatedPage, len(raw))
	for i, page := range raw {
		if !ValidSlug(page.Slug) || strings.TrimSpace(page.Name) == "" || !validHTTPURL(page.URL) {
			return nil, contractError(op, "invalid "+field+" entry", nil)
		}
		out[i] = CompanyRelatedPage{
			Slug:           page.Slug,
			URL:            page.URL,
			Name:           page.Name,
			Industry:       page.Industry,
			FollowersLabel: page.FollowersLabel,
		}
	}
	return out, nil
}
