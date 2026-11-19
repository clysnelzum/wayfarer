package linkedin

// Profile represents a rendered LinkedIn personal profile.
type Profile struct {
	Slug                string                  `json:"slug"`
	URL                 string                  `json:"url"`
	Name                string                  `json:"name"`
	Pronouns            string                  `json:"pronouns,omitempty"`
	Verified            bool                    `json:"verified,omitempty"`
	Premium             bool                    `json:"premium,omitempty"`
	TopVoice            bool                    `json:"top_voice,omitempty"`
	ConnectionDegree    string                  `json:"connection_degree,omitempty"`
	Headline            string                  `json:"headline,omitempty"`
	Location            string                  `json:"location,omitempty"`
	ProfileImageURL     string                  `json:"profile_image_url,omitempty"`
	BackgroundImageURL  string                  `json:"background_image_url,omitempty"`
	FollowersLabel      string                  `json:"followers_label,omitempty"`
	FollowersCount      *int64                  `json:"followers_count,omitempty"`
	ConnectionsLabel    string                  `json:"connections_label,omitempty"`
	CurrentCompanyLabel string                  `json:"current_company_label,omitempty"`
	EducationLabel      string                  `json:"education_label,omitempty"`
	Website             string                  `json:"website,omitempty"`
	OpenToWork          bool                    `json:"open_to_work,omitempty"`
	Hiring              bool                    `json:"hiring,omitempty"`
	About               string                  `json:"about,omitempty"`
	TopSkills           []string                `json:"top_skills,omitempty"`
	Featured            []ProfileFeaturedItem   `json:"featured,omitempty"`
	Activity            []ProfileActivityItem   `json:"activity,omitempty"`
	Experience          []ProfileExperience     `json:"experience,omitempty"`
	Education           []ProfileEducation      `json:"education,omitempty"`
	Certifications      []ProfileCertification  `json:"certifications,omitempty"`
	Skills              []ProfileSkill          `json:"skills,omitempty"`
	Languages           []ProfileLanguage       `json:"languages,omitempty"`
	Projects            []ProfileProject        `json:"projects,omitempty"`
	Volunteering        []ProfileVolunteering   `json:"volunteering,omitempty"`
	Publications        []ProfilePublication    `json:"publications,omitempty"`
	Honors              []ProfileHonor          `json:"honors,omitempty"`
	Recommendations     []ProfileRecommendation `json:"recommendations,omitempty"`
	Interests           []ProfileInterest       `json:"interests,omitempty"`
}

// ProfileFeaturedItem describes an item pinned in a profile's Featured section.
type ProfileFeaturedItem struct {
	Title string `json:"title,omitempty"`
	Type  string `json:"type,omitempty"`
	URL   string `json:"url,omitempty"`
}

// ProfileActivityItem describes a recent post or activity entry visible on a profile.
type ProfileActivityItem struct {
	URL            string `json:"url,omitempty"`
	PublishedLabel string `json:"published_label,omitempty"`
	Text           string `json:"text,omitempty"`
	AuthorName     string `json:"author_name,omitempty"`
	AuthorSlug     string `json:"author_slug,omitempty"`
}

// ProfileExperience describes a single role in a profile's Experience section.
type ProfileExperience struct {
	Title          string `json:"title"`
	Company        string `json:"company,omitempty"`
	CompanyURL     string `json:"company_url,omitempty"`
	CompanySlug    string `json:"company_slug,omitempty"`
	EmploymentType string `json:"employment_type,omitempty"`
	DateRange      string `json:"date_range,omitempty"`
	Duration       string `json:"duration,omitempty"`
	Location       string `json:"location,omitempty"`
	Description    string `json:"description,omitempty"`
	Current        bool   `json:"current,omitempty"`
}

// ProfileEducation describes an entry in a profile's Education section.
type ProfileEducation struct {
	School      string `json:"school"`
	SchoolURL   string `json:"school_url,omitempty"`
	SchoolSlug  string `json:"school_slug,omitempty"`
	Degree      string `json:"degree,omitempty"`
	DateRange   string `json:"date_range,omitempty"`
	Description string `json:"description,omitempty"`
}

// ProfileCertification describes a license or certification listed on a profile.
type ProfileCertification struct {
	Name          string `json:"name"`
	Issuer        string `json:"issuer,omitempty"`
	IssueDate     string `json:"issue_date,omitempty"`
	CredentialID  string `json:"credential_id,omitempty"`
	CredentialURL string `json:"credential_url,omitempty"`
}

// ProfileSkill describes a skill entry and optional endorsement label on a profile.
type ProfileSkill struct {
	Name              string `json:"name"`
	EndorsementsLabel string `json:"endorsements_label,omitempty"`
}

// ProfileLanguage describes a language and proficiency listed on a profile.
type ProfileLanguage struct {
	Name        string `json:"name"`
	Proficiency string `json:"proficiency,omitempty"`
}

// ProfileProject describes a project listed on a profile.
type ProfileProject struct {
	Title       string `json:"title"`
	DateRange   string `json:"date_range,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// ProfileVolunteering describes a volunteering role listed on a profile.
type ProfileVolunteering struct {
	Role         string `json:"role"`
	Organization string `json:"organization,omitempty"`
	DateRange    string `json:"date_range,omitempty"`
	Cause        string `json:"cause,omitempty"`
	Description  string `json:"description,omitempty"`
}

// ProfilePublication describes a publication listed on a profile.
type ProfilePublication struct {
	Title       string `json:"title"`
	Publisher   string `json:"publisher,omitempty"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// ProfileHonor describes an honor or award listed on a profile.
type ProfileHonor struct {
	Title       string `json:"title"`
	Issuer      string `json:"issuer,omitempty"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
}

// ProfileRecommendation describes a recommendation received by the profile owner.
type ProfileRecommendation struct {
	AuthorName     string `json:"author_name"`
	AuthorSlug     string `json:"author_slug"`
	AuthorURL      string `json:"author_url"`
	AuthorHeadline string `json:"author_headline,omitempty"`
	Relationship   string `json:"relationship,omitempty"`
	Text           string `json:"text,omitempty"`
}

// ProfileInterest describes an entity followed in a profile's Interests section.
type ProfileInterest struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	Headline       string `json:"headline,omitempty"`
	FollowersLabel string `json:"followers_label,omitempty"`
}
