package linkedin

// Company represents a LinkedIn organization's About page and sidebar details.
type Company struct {
	Slug                   string                  `json:"slug"`
	URL                    string                  `json:"url"`
	Name                   string                  `json:"name"`
	ID                     string                  `json:"id,omitempty"`
	Tagline                string                  `json:"tagline,omitempty"`
	Description            string                  `json:"description,omitempty"`
	Website                string                  `json:"website,omitempty"`
	Phone                  string                  `json:"phone,omitempty"`
	Industry               string                  `json:"industry,omitempty"`
	CompanySize            string                  `json:"company_size,omitempty"`
	AssociatedMembersCount *int64                  `json:"associated_members_count,omitempty"`
	AssociatedMembersLabel string                  `json:"associated_members_label,omitempty"`
	FollowersCount         *int64                  `json:"followers_count,omitempty"`
	FollowersLabel         string                  `json:"followers_label,omitempty"`
	EmployeesLabel         string                  `json:"employees_label,omitempty"`
	Headquarters           string                  `json:"headquarters,omitempty"`
	Founded                string                  `json:"founded,omitempty"`
	CompanyType            string                  `json:"company_type,omitempty"`
	Specialties            string                  `json:"specialties,omitempty"`
	Verified               bool                    `json:"verified,omitempty"`
	VerifiedPage           string                  `json:"verified_page,omitempty"`
	Premium                bool                    `json:"premium,omitempty"`
	LogoURL                string                  `json:"logo_url,omitempty"`
	CoverImageURL          string                  `json:"cover_image_url,omitempty"`
	Highlights             []string                `json:"highlights,omitempty"`
	WorkplacePolicy        *CompanyWorkplacePolicy `json:"workplace_policy,omitempty"`
	Commitments            []CompanyCommitment     `json:"commitments,omitempty"`
	LocationsCount         *int                    `json:"locations_count,omitempty"`
	Locations              []CompanyLocation       `json:"locations,omitempty"`
	AffiliatedPages        []CompanyRelatedPage    `json:"affiliated_pages,omitempty"`
	SimilarPages           []CompanyRelatedPage    `json:"similar_pages,omitempty"`
}

// CompanyWorkplacePolicy describes the workplace policy module on a company About page.
type CompanyWorkplacePolicy struct {
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	DefaultPresence string   `json:"default_presence,omitempty"`
	Benefits        []string `json:"benefits,omitempty"`
}

// CompanyCommitment describes a company commitment block on the About page.
type CompanyCommitment struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// CompanyLocation describes an office or headquarters location on a company About page.
type CompanyLocation struct {
	Label          string `json:"label,omitempty"`
	Address        string `json:"address,omitempty"`
	IsHeadquarters bool   `json:"is_headquarters,omitempty"`
	MapURL         string `json:"map_url,omitempty"`
}

// CompanyRelatedPage describes an affiliated, showcase, or similar company page.
type CompanyRelatedPage struct {
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	Name           string `json:"name"`
	Industry       string `json:"industry,omitempty"`
	FollowersLabel string `json:"followers_label,omitempty"`
}
