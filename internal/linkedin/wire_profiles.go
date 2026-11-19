package linkedin

import "strings"

const (
	maxProfileTopSkills       = 20
	maxProfileFeatured        = 15
	maxProfileActivity        = 10
	maxProfileExperience      = 50
	maxProfileEducation       = 30
	maxProfileCertifications  = 50
	maxProfileSkills          = 50
	maxProfileLanguages       = 30
	maxProfileProjects        = 30
	maxProfileVolunteering    = 30
	maxProfilePublications    = 30
	maxProfileHonors          = 30
	maxProfileRecommendations = 20
	maxProfileInterests       = 20
)

type wireProfileResponse struct {
	Profile  *wireProfile `json:"profile"`
	NotFound bool         `json:"not_found"`
}

type wireProfile struct {
	Slug                string                      `json:"slug"`
	URL                 string                      `json:"url"`
	Name                string                      `json:"name"`
	Pronouns            string                      `json:"pronouns"`
	Verified            bool                        `json:"verified"`
	Premium             bool                        `json:"premium"`
	TopVoice            bool                        `json:"top_voice"`
	ConnectionDegree    string                      `json:"connection_degree"`
	Headline            string                      `json:"headline"`
	Location            string                      `json:"location"`
	ProfileImageURL     string                      `json:"profile_image_url"`
	BackgroundImageURL  string                      `json:"background_image_url"`
	FollowersLabel      string                      `json:"followers_label"`
	FollowersCount      *int64                      `json:"followers_count"`
	ConnectionsLabel    string                      `json:"connections_label"`
	CurrentCompanyLabel string                      `json:"current_company_label"`
	EducationLabel      string                      `json:"education_label"`
	Website             string                      `json:"website"`
	OpenToWork          bool                        `json:"open_to_work"`
	Hiring              bool                        `json:"hiring"`
	About               string                      `json:"about"`
	TopSkills           []string                    `json:"top_skills"`
	Featured            []wireProfileFeaturedItem   `json:"featured"`
	Activity            []wireProfileActivityItem   `json:"activity"`
	Experience          []wireProfileExperience     `json:"experience"`
	Education           []wireProfileEducation      `json:"education"`
	Certifications      []wireProfileCertification  `json:"certifications"`
	Skills              []wireProfileSkill          `json:"skills"`
	Languages           []wireProfileLanguage       `json:"languages"`
	Projects            []wireProfileProject        `json:"projects"`
	Volunteering        []wireProfileVolunteering   `json:"volunteering"`
	Publications        []wireProfilePublication    `json:"publications"`
	Honors              []wireProfileHonor          `json:"honors"`
	Recommendations     []wireProfileRecommendation `json:"recommendations"`
	Interests           []wireProfileInterest       `json:"interests"`
}

type wireProfileFeaturedItem struct {
	Title string `json:"title"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

type wireProfileActivityItem struct {
	URL            string `json:"url"`
	PublishedLabel string `json:"published_label"`
	Text           string `json:"text"`
	AuthorName     string `json:"author_name"`
	AuthorSlug     string `json:"author_slug"`
}

type wireProfileExperience struct {
	Title          string `json:"title"`
	Company        string `json:"company"`
	CompanyURL     string `json:"company_url"`
	CompanySlug    string `json:"company_slug"`
	EmploymentType string `json:"employment_type"`
	DateRange      string `json:"date_range"`
	Duration       string `json:"duration"`
	Location       string `json:"location"`
	Description    string `json:"description"`
	Current        bool   `json:"current"`
}

type wireProfileEducation struct {
	School      string `json:"school"`
	SchoolURL   string `json:"school_url"`
	SchoolSlug  string `json:"school_slug"`
	Degree      string `json:"degree"`
	DateRange   string `json:"date_range"`
	Description string `json:"description"`
}

type wireProfileCertification struct {
	Name          string `json:"name"`
	Issuer        string `json:"issuer"`
	IssueDate     string `json:"issue_date"`
	CredentialID  string `json:"credential_id"`
	CredentialURL string `json:"credential_url"`
}

type wireProfileSkill struct {
	Name              string `json:"name"`
	EndorsementsLabel string `json:"endorsements_label"`
}

type wireProfileLanguage struct {
	Name        string `json:"name"`
	Proficiency string `json:"proficiency"`
}

type wireProfileProject struct {
	Title       string `json:"title"`
	DateRange   string `json:"date_range"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type wireProfileVolunteering struct {
	Role         string `json:"role"`
	Organization string `json:"organization"`
	DateRange    string `json:"date_range"`
	Cause        string `json:"cause"`
	Description  string `json:"description"`
}

type wireProfilePublication struct {
	Title       string `json:"title"`
	Publisher   string `json:"publisher"`
	Date        string `json:"date"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type wireProfileHonor struct {
	Title       string `json:"title"`
	Issuer      string `json:"issuer"`
	Date        string `json:"date"`
	Description string `json:"description"`
}

type wireProfileRecommendation struct {
	AuthorName     string `json:"author_name"`
	AuthorSlug     string `json:"author_slug"`
	AuthorURL      string `json:"author_url"`
	AuthorHeadline string `json:"author_headline"`
	Relationship   string `json:"relationship"`
	Text           string `json:"text"`
}

type wireProfileInterest struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Slug           string `json:"slug"`
	URL            string `json:"url"`
	Headline       string `json:"headline"`
	FollowersLabel string `json:"followers_label"`
}

func (w wireProfileResponse) toDomain(requestedSlug, baseURL string) (Profile, error) {
	const op = "profile"
	if w.NotFound {
		return Profile{}, ErrProfileNotFound
	}
	if w.Profile == nil {
		return Profile{}, contractError(op, "missing profile object", nil)
	}
	p := w.Profile
	if !ValidSlug(p.Slug) || !strings.EqualFold(p.Slug, requestedSlug) || strings.TrimSpace(p.Name) == "" {
		return Profile{}, contractError(op, "invalid or mismatched profile slug/name", nil)
	}
	if p.FollowersCount != nil && *p.FollowersCount < 0 {
		return Profile{}, contractError(op, "negative followers_count", nil)
	}
	for _, rawURL := range []string{p.ProfileImageURL, p.BackgroundImageURL, p.Website} {
		if rawURL != "" && !validHTTPURL(rawURL) {
			return Profile{}, contractError(op, "invalid profile url field", nil)
		}
	}

	topSkills, err := validateNonEmptyStrings(op, "top_skills", p.TopSkills, maxProfileTopSkills)
	if err != nil {
		return Profile{}, err
	}
	featured, err := convertProfileFeatured(op, p.Featured)
	if err != nil {
		return Profile{}, err
	}
	activity, err := convertProfileActivity(op, p.Activity)
	if err != nil {
		return Profile{}, err
	}
	experience, err := convertProfileExperience(op, p.Experience)
	if err != nil {
		return Profile{}, err
	}
	education, err := convertProfileEducation(op, p.Education)
	if err != nil {
		return Profile{}, err
	}
	certifications, err := convertProfileCertifications(op, p.Certifications)
	if err != nil {
		return Profile{}, err
	}
	skills, err := convertProfileSkills(op, p.Skills)
	if err != nil {
		return Profile{}, err
	}
	languages, err := convertProfileLanguages(op, p.Languages)
	if err != nil {
		return Profile{}, err
	}
	projects, err := convertProfileProjects(op, p.Projects)
	if err != nil {
		return Profile{}, err
	}
	volunteering, err := convertProfileVolunteering(op, p.Volunteering)
	if err != nil {
		return Profile{}, err
	}
	publications, err := convertProfilePublications(op, p.Publications)
	if err != nil {
		return Profile{}, err
	}
	honors, err := convertProfileHonors(op, p.Honors)
	if err != nil {
		return Profile{}, err
	}
	recommendations, err := convertProfileRecommendations(op, p.Recommendations)
	if err != nil {
		return Profile{}, err
	}
	interests, err := convertProfileInterests(op, p.Interests)
	if err != nil {
		return Profile{}, err
	}

	return Profile{
		Slug:                p.Slug,
		URL:                 baseURL + "/in/" + p.Slug + "/",
		Name:                p.Name,
		Pronouns:            p.Pronouns,
		Verified:            p.Verified,
		Premium:             p.Premium,
		TopVoice:            p.TopVoice,
		ConnectionDegree:    p.ConnectionDegree,
		Headline:            p.Headline,
		Location:            p.Location,
		ProfileImageURL:     p.ProfileImageURL,
		BackgroundImageURL:  p.BackgroundImageURL,
		FollowersLabel:      p.FollowersLabel,
		FollowersCount:      p.FollowersCount,
		ConnectionsLabel:    p.ConnectionsLabel,
		CurrentCompanyLabel: p.CurrentCompanyLabel,
		EducationLabel:      p.EducationLabel,
		Website:             p.Website,
		OpenToWork:          p.OpenToWork,
		Hiring:              p.Hiring,
		About:               p.About,
		TopSkills:           topSkills,
		Featured:            featured,
		Activity:            activity,
		Experience:          experience,
		Education:           education,
		Certifications:      certifications,
		Skills:              skills,
		Languages:           languages,
		Projects:            projects,
		Volunteering:        volunteering,
		Publications:        publications,
		Honors:              honors,
		Recommendations:     recommendations,
		Interests:           interests,
	}, nil
}

func validateNonEmptyStrings(op, field string, raw []string, limit int) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > limit {
		return nil, contractError(op, field+" exceeds max items", nil)
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		if strings.TrimSpace(v) == "" {
			return nil, contractError(op, "empty item in "+field, nil)
		}
		out[i] = v
	}
	return out, nil
}

func convertProfileFeatured(op string, raw []wireProfileFeaturedItem) ([]ProfileFeaturedItem, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileFeatured {
		return nil, contractError(op, "featured exceeds max items", nil)
	}
	out := make([]ProfileFeaturedItem, len(raw))
	for i, item := range raw {
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.URL) == "" {
			return nil, contractError(op, "empty featured item", nil)
		}
		if item.URL != "" && !validHTTPURL(item.URL) {
			return nil, contractError(op, "invalid featured item url", nil)
		}
		out[i] = ProfileFeaturedItem{Title: item.Title, Type: item.Type, URL: item.URL}
	}
	return out, nil
}

func convertProfileActivity(op string, raw []wireProfileActivityItem) ([]ProfileActivityItem, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileActivity {
		return nil, contractError(op, "activity exceeds max items", nil)
	}
	out := make([]ProfileActivityItem, len(raw))
	for i, item := range raw {
		if strings.TrimSpace(item.Text) == "" && strings.TrimSpace(item.URL) == "" {
			return nil, contractError(op, "empty activity item", nil)
		}
		if item.URL != "" && !validHTTPURL(item.URL) {
			return nil, contractError(op, "invalid activity item url", nil)
		}
		out[i] = ProfileActivityItem{
			URL:            item.URL,
			PublishedLabel: item.PublishedLabel,
			Text:           item.Text,
			AuthorName:     item.AuthorName,
			AuthorSlug:     item.AuthorSlug,
		}
	}
	return out, nil
}

func convertProfileExperience(op string, raw []wireProfileExperience) ([]ProfileExperience, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileExperience {
		return nil, contractError(op, "experience exceeds max items", nil)
	}
	out := make([]ProfileExperience, len(raw))
	for i, exp := range raw {
		if strings.TrimSpace(exp.Title) == "" {
			return nil, contractError(op, "missing experience title", nil)
		}
		if exp.CompanyURL != "" && !validHTTPURL(exp.CompanyURL) {
			return nil, contractError(op, "invalid experience company_url", nil)
		}
		out[i] = ProfileExperience{
			Title:          exp.Title,
			Company:        exp.Company,
			CompanyURL:     exp.CompanyURL,
			CompanySlug:    exp.CompanySlug,
			EmploymentType: exp.EmploymentType,
			DateRange:      exp.DateRange,
			Duration:       exp.Duration,
			Location:       exp.Location,
			Description:    exp.Description,
			Current:        exp.Current,
		}
	}
	return out, nil
}

func convertProfileEducation(op string, raw []wireProfileEducation) ([]ProfileEducation, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileEducation {
		return nil, contractError(op, "education exceeds max items", nil)
	}
	out := make([]ProfileEducation, len(raw))
	for i, edu := range raw {
		if strings.TrimSpace(edu.School) == "" {
			return nil, contractError(op, "missing education school", nil)
		}
		if edu.SchoolURL != "" && !validHTTPURL(edu.SchoolURL) {
			return nil, contractError(op, "invalid education school_url", nil)
		}
		out[i] = ProfileEducation{
			School:      edu.School,
			SchoolURL:   edu.SchoolURL,
			SchoolSlug:  edu.SchoolSlug,
			Degree:      edu.Degree,
			DateRange:   edu.DateRange,
			Description: edu.Description,
		}
	}
	return out, nil
}

func convertProfileCertifications(op string, raw []wireProfileCertification) ([]ProfileCertification, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileCertifications {
		return nil, contractError(op, "certifications exceeds max items", nil)
	}
	out := make([]ProfileCertification, len(raw))
	for i, cert := range raw {
		if strings.TrimSpace(cert.Name) == "" {
			return nil, contractError(op, "missing certification name", nil)
		}
		if cert.CredentialURL != "" && !validHTTPURL(cert.CredentialURL) {
			return nil, contractError(op, "invalid certification credential_url", nil)
		}
		out[i] = ProfileCertification{
			Name:          cert.Name,
			Issuer:        cert.Issuer,
			IssueDate:     cert.IssueDate,
			CredentialID:  cert.CredentialID,
			CredentialURL: cert.CredentialURL,
		}
	}
	return out, nil
}

func convertProfileSkills(op string, raw []wireProfileSkill) ([]ProfileSkill, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileSkills {
		return nil, contractError(op, "skills exceeds max items", nil)
	}
	out := make([]ProfileSkill, len(raw))
	for i, skill := range raw {
		if strings.TrimSpace(skill.Name) == "" {
			return nil, contractError(op, "missing skill name", nil)
		}
		out[i] = ProfileSkill{Name: skill.Name, EndorsementsLabel: skill.EndorsementsLabel}
	}
	return out, nil
}

func convertProfileLanguages(op string, raw []wireProfileLanguage) ([]ProfileLanguage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileLanguages {
		return nil, contractError(op, "languages exceeds max items", nil)
	}
	out := make([]ProfileLanguage, len(raw))
	for i, lang := range raw {
		if strings.TrimSpace(lang.Name) == "" {
			return nil, contractError(op, "missing language name", nil)
		}
		out[i] = ProfileLanguage{Name: lang.Name, Proficiency: lang.Proficiency}
	}
	return out, nil
}

func convertProfileProjects(op string, raw []wireProfileProject) ([]ProfileProject, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileProjects {
		return nil, contractError(op, "projects exceeds max items", nil)
	}
	out := make([]ProfileProject, len(raw))
	for i, proj := range raw {
		if strings.TrimSpace(proj.Title) == "" {
			return nil, contractError(op, "missing project title", nil)
		}
		if proj.URL != "" && !validHTTPURL(proj.URL) {
			return nil, contractError(op, "invalid project url", nil)
		}
		out[i] = ProfileProject{
			Title:       proj.Title,
			DateRange:   proj.DateRange,
			Description: proj.Description,
			URL:         proj.URL,
		}
	}
	return out, nil
}

func convertProfileVolunteering(op string, raw []wireProfileVolunteering) ([]ProfileVolunteering, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileVolunteering {
		return nil, contractError(op, "volunteering exceeds max items", nil)
	}
	out := make([]ProfileVolunteering, len(raw))
	for i, vol := range raw {
		if strings.TrimSpace(vol.Role) == "" {
			return nil, contractError(op, "missing volunteering role", nil)
		}
		out[i] = ProfileVolunteering{
			Role:         vol.Role,
			Organization: vol.Organization,
			DateRange:    vol.DateRange,
			Cause:        vol.Cause,
			Description:  vol.Description,
		}
	}
	return out, nil
}

func convertProfilePublications(op string, raw []wireProfilePublication) ([]ProfilePublication, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfilePublications {
		return nil, contractError(op, "publications exceeds max items", nil)
	}
	out := make([]ProfilePublication, len(raw))
	for i, pub := range raw {
		if strings.TrimSpace(pub.Title) == "" {
			return nil, contractError(op, "missing publication title", nil)
		}
		if pub.URL != "" && !validHTTPURL(pub.URL) {
			return nil, contractError(op, "invalid publication url", nil)
		}
		out[i] = ProfilePublication{
			Title:       pub.Title,
			Publisher:   pub.Publisher,
			Date:        pub.Date,
			Description: pub.Description,
			URL:         pub.URL,
		}
	}
	return out, nil
}

func convertProfileHonors(op string, raw []wireProfileHonor) ([]ProfileHonor, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileHonors {
		return nil, contractError(op, "honors exceeds max items", nil)
	}
	out := make([]ProfileHonor, len(raw))
	for i, honor := range raw {
		if strings.TrimSpace(honor.Title) == "" {
			return nil, contractError(op, "missing honor title", nil)
		}
		out[i] = ProfileHonor{
			Title:       honor.Title,
			Issuer:      honor.Issuer,
			Date:        honor.Date,
			Description: honor.Description,
		}
	}
	return out, nil
}

func convertProfileRecommendations(op string, raw []wireProfileRecommendation) ([]ProfileRecommendation, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileRecommendations {
		return nil, contractError(op, "recommendations exceeds max items", nil)
	}
	out := make([]ProfileRecommendation, len(raw))
	for i, rec := range raw {
		if strings.TrimSpace(rec.AuthorName) == "" || !ValidSlug(rec.AuthorSlug) || !validHTTPURL(rec.AuthorURL) {
			return nil, contractError(op, "invalid recommendation author", nil)
		}
		out[i] = ProfileRecommendation{
			AuthorName:     rec.AuthorName,
			AuthorSlug:     rec.AuthorSlug,
			AuthorURL:      rec.AuthorURL,
			AuthorHeadline: rec.AuthorHeadline,
			Relationship:   rec.Relationship,
			Text:           rec.Text,
		}
	}
	return out, nil
}

func convertProfileInterests(op string, raw []wireProfileInterest) ([]ProfileInterest, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxProfileInterests {
		return nil, contractError(op, "interests exceeds max items", nil)
	}
	out := make([]ProfileInterest, len(raw))
	for i, in := range raw {
		if strings.TrimSpace(in.Name) == "" || !validInterestKind(in.Kind) || strings.TrimSpace(in.Slug) == "" || !validHTTPURL(in.URL) {
			return nil, contractError(op, "invalid profile interest", nil)
		}
		out[i] = ProfileInterest{
			Name:           in.Name,
			Kind:           in.Kind,
			Slug:           in.Slug,
			URL:            in.URL,
			Headline:       in.Headline,
			FollowersLabel: in.FollowersLabel,
		}
	}
	return out, nil
}

func validInterestKind(kind string) bool {
	switch kind {
	case "person", "company", "school", "group", "newsletter":
		return true
	default:
		return false
	}
}
