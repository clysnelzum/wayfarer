package linkedin

import (
	"context"
	_ "embed"
	"errors"
	"regexp"
	"strings"
)

//go:embed profile.js
var profileExpression string

//go:embed company.js
var companyExpression string

var (
	ErrProfileNotFound = errors.New("personal profile not found")
	ErrCompanyNotFound = errors.New("company profile not found")
	slugPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,99}$`)
)

func ValidSlug(slug string) bool { return slugPattern.MatchString(slug) }

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

type ProfileFeaturedItem struct {
	Title string `json:"title,omitempty"`
	Type  string `json:"type,omitempty"`
	URL   string `json:"url,omitempty"`
}

type ProfileActivityItem struct {
	URL            string `json:"url,omitempty"`
	PublishedLabel string `json:"published_label,omitempty"`
	Text           string `json:"text,omitempty"`
	AuthorName     string `json:"author_name,omitempty"`
	AuthorSlug     string `json:"author_slug,omitempty"`
}

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

type ProfileEducation struct {
	School      string `json:"school"`
	SchoolURL   string `json:"school_url,omitempty"`
	SchoolSlug  string `json:"school_slug,omitempty"`
	Degree      string `json:"degree,omitempty"`
	DateRange   string `json:"date_range,omitempty"`
	Description string `json:"description,omitempty"`
}

type ProfileCertification struct {
	Name          string `json:"name"`
	Issuer        string `json:"issuer,omitempty"`
	IssueDate     string `json:"issue_date,omitempty"`
	CredentialID  string `json:"credential_id,omitempty"`
	CredentialURL string `json:"credential_url,omitempty"`
}

type ProfileSkill struct {
	Name              string `json:"name"`
	EndorsementsLabel string `json:"endorsements_label,omitempty"`
}

type ProfileLanguage struct {
	Name        string `json:"name"`
	Proficiency string `json:"proficiency,omitempty"`
}

type ProfileProject struct {
	Title       string `json:"title"`
	DateRange   string `json:"date_range,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

type ProfileVolunteering struct {
	Role         string `json:"role"`
	Organization string `json:"organization,omitempty"`
	DateRange    string `json:"date_range,omitempty"`
	Cause        string `json:"cause,omitempty"`
	Description  string `json:"description,omitempty"`
}

type ProfilePublication struct {
	Title       string `json:"title"`
	Publisher   string `json:"publisher,omitempty"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

type ProfileHonor struct {
	Title       string `json:"title"`
	Issuer      string `json:"issuer,omitempty"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
}

type ProfileRecommendation struct {
	AuthorName     string `json:"author_name"`
	AuthorSlug     string `json:"author_slug"`
	AuthorURL      string `json:"author_url"`
	AuthorHeadline string `json:"author_headline,omitempty"`
	Relationship   string `json:"relationship,omitempty"`
	Text           string `json:"text,omitempty"`
}

type ProfileInterest struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	Headline       string `json:"headline,omitempty"`
	FollowersLabel string `json:"followers_label,omitempty"`
}

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

type CompanyWorkplacePolicy struct {
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	DefaultPresence string   `json:"default_presence,omitempty"`
	Benefits        []string `json:"benefits,omitempty"`
}

type CompanyCommitment struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type CompanyLocation struct {
	Label          string `json:"label,omitempty"`
	Address        string `json:"address,omitempty"`
	IsHeadquarters bool   `json:"is_headquarters,omitempty"`
	MapURL         string `json:"map_url,omitempty"`
}

type CompanyRelatedPage struct {
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	Name           string `json:"name"`
	Industry       string `json:"industry,omitempty"`
	FollowersLabel string `json:"followers_label,omitempty"`
}

func (c Client) Profile(ctx context.Context, slug string) (Profile, error) {
	if !ValidSlug(slug) {
		return Profile{}, errors.New("invalid profile slug")
	}
	v, err := read[struct {
		Data     Profile `json:"profile"`
		NotFound bool    `json:"not_found"`
	}](ctx, renderReader(c.Reader), "/in/"+slug+"/", profileExpression)
	if err != nil {
		return Profile{}, err
	}
	if v.NotFound {
		return Profile{}, ErrProfileNotFound
	}
	if !ValidSlug(v.Data.Slug) || !strings.EqualFold(v.Data.Slug, slug) || v.Data.Name == "" {
		return Profile{}, errors.New("browser result did not match the requested profile")
	}
	if err := validateProfile(v.Data); err != nil {
		return Profile{}, err
	}
	v.Data.URL = base + "/in/" + v.Data.Slug + "/"
	return v.Data, nil
}

func (c Client) Company(ctx context.Context, slug string) (Company, error) {
	if !ValidSlug(slug) {
		return Company{}, errors.New("invalid company slug")
	}
	v, err := read[struct {
		Data     Company `json:"company"`
		NotFound bool    `json:"not_found"`
	}](ctx, c.Reader, "/company/"+slug+"/about/", companyExpression)
	if err != nil {
		return Company{}, err
	}
	if v.NotFound {
		return Company{}, ErrCompanyNotFound
	}
	if !ValidSlug(v.Data.Slug) || !strings.EqualFold(v.Data.Slug, slug) || v.Data.Name == "" {
		return Company{}, errors.New("browser result did not match the requested company")
	}
	if err := validateCompany(v.Data); err != nil {
		return Company{}, err
	}
	v.Data.URL = base + "/company/" + v.Data.Slug + "/"
	return v.Data, nil
}

func validateCompany(co Company) error {
	if co.ID != "" && !companyIDPattern.MatchString(co.ID) {
		return errors.New("invalid company id from Chrome")
	}
	if co.AssociatedMembersCount != nil && *co.AssociatedMembersCount < 0 {
		return errors.New("invalid associated members count from Chrome")
	}
	if co.FollowersCount != nil && *co.FollowersCount < 0 {
		return errors.New("invalid followers count from Chrome")
	}
	if co.LocationsCount != nil && *co.LocationsCount < 0 {
		return errors.New("invalid locations count from Chrome")
	}
	if co.WorkplacePolicy != nil && strings.TrimSpace(co.WorkplacePolicy.Title) == "" {
		return errors.New("invalid workplace policy from Chrome")
	}
	for _, cm := range co.Commitments {
		if strings.TrimSpace(cm.Title) == "" {
			return errors.New("invalid company commitment from Chrome")
		}
	}
	for _, loc := range co.Locations {
		if strings.TrimSpace(loc.Address) == "" && strings.TrimSpace(loc.Label) == "" {
			return errors.New("invalid company location from Chrome")
		}
	}
	for _, page := range co.AffiliatedPages {
		if !ValidSlug(page.Slug) || strings.TrimSpace(page.Name) == "" || strings.TrimSpace(page.URL) == "" {
			return errors.New("invalid affiliated page from Chrome")
		}
	}
	for _, page := range co.SimilarPages {
		if !ValidSlug(page.Slug) || strings.TrimSpace(page.Name) == "" || strings.TrimSpace(page.URL) == "" {
			return errors.New("invalid similar page from Chrome")
		}
	}
	return nil
}

func validateProfile(p Profile) error {
	if p.FollowersCount != nil && *p.FollowersCount < 0 {
		return errors.New("invalid followers count from Chrome")
	}
	for _, item := range p.Featured {
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.URL) == "" {
			return errors.New("invalid featured item from Chrome")
		}
	}
	for _, item := range p.Activity {
		if strings.TrimSpace(item.Text) == "" && strings.TrimSpace(item.URL) == "" {
			return errors.New("invalid activity item from Chrome")
		}
	}
	for _, exp := range p.Experience {
		if strings.TrimSpace(exp.Title) == "" {
			return errors.New("invalid profile experience from Chrome")
		}
	}
	for _, edu := range p.Education {
		if strings.TrimSpace(edu.School) == "" {
			return errors.New("invalid profile education from Chrome")
		}
	}
	for _, cert := range p.Certifications {
		if strings.TrimSpace(cert.Name) == "" {
			return errors.New("invalid profile certification from Chrome")
		}
	}
	for _, skill := range p.Skills {
		if strings.TrimSpace(skill.Name) == "" {
			return errors.New("invalid profile skill from Chrome")
		}
	}
	for _, lang := range p.Languages {
		if strings.TrimSpace(lang.Name) == "" {
			return errors.New("invalid profile language from Chrome")
		}
	}
	for _, proj := range p.Projects {
		if strings.TrimSpace(proj.Title) == "" {
			return errors.New("invalid profile project from Chrome")
		}
	}
	for _, vol := range p.Volunteering {
		if strings.TrimSpace(vol.Role) == "" {
			return errors.New("invalid profile volunteering from Chrome")
		}
	}
	for _, pub := range p.Publications {
		if strings.TrimSpace(pub.Title) == "" {
			return errors.New("invalid profile publication from Chrome")
		}
	}
	for _, honor := range p.Honors {
		if strings.TrimSpace(honor.Title) == "" {
			return errors.New("invalid profile honor from Chrome")
		}
	}
	for _, rec := range p.Recommendations {
		if strings.TrimSpace(rec.AuthorName) == "" || !ValidSlug(rec.AuthorSlug) || strings.TrimSpace(rec.AuthorURL) == "" {
			return errors.New("invalid profile recommendation from Chrome")
		}
	}
	for _, in := range p.Interests {
		if strings.TrimSpace(in.Name) == "" || !validInterestKind(in.Kind) || strings.TrimSpace(in.Slug) == "" || strings.TrimSpace(in.URL) == "" {
			return errors.New("invalid profile interest from Chrome")
		}
	}
	return nil
}

func validInterestKind(kind string) bool {
	switch kind {
	case "person", "company", "school", "group", "newsletter":
		return true
	default:
		return false
	}
}
