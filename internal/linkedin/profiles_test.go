package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestProfileAndCompanyAdapters(t *testing.T) {
	responses := map[string]string{
		defaultBaseURL + "/in/example-person/":           `{"profile":{"slug":"example-person","name":"Example Person","headline":"Engineer","url":"ignored"}}`,
		defaultBaseURL + "/in/SatyaNadella/":             `{"profile":{"slug":"satyanadella","name":"Satya Nadella","url":"ignored"}}`,
		defaultBaseURL + "/in/satyanadella/":             `{"profile":{"slug":"ſatyanadella","name":"Bad"}}`,
		defaultBaseURL + "/company/example-co/about/":    `{"company":{"slug":"example-co","name":"Example Company","industry":"Technology","url":"ignored"}}`,
		defaultBaseURL + "/company/Microsoft/about/":     `{"company":{"slug":"microsoft","name":"Microsoft","url":"ignored"}}`,
		defaultBaseURL + "/in/missing/":                  `{"not_found":true}`,
		defaultBaseURL + "/company/missing/about/":       `{"not_found":true}`,
		defaultBaseURL + "/company/wrong-content/about/": `{"company":{"slug":"another-company","name":"Another Company"}}`,
	}
	mock := &renderedReaderMock{
		readFn: func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
			body, ok := responses[pageURL]
			if !ok || expression == "" {
				t.Fatalf("unexpected upstream query %q", pageURL)
			}
			return json.RawMessage(body), nil
		},
		readRenderedFn: func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
			body, ok := responses[pageURL]
			if !ok || expression == "" {
				t.Fatalf("unexpected upstream rendered query %q", pageURL)
			}
			return json.RawMessage(body), nil
		},
	}
	c := mustNewClient(t, mock)
	ctx := context.Background()
	p, err := c.Profile(ctx, "example-person")
	if err != nil || p.URL != defaultBaseURL+"/in/example-person/" || p.Headline != "Engineer" {
		t.Fatalf("unexpected personal profile: %+v (%v)", p, err)
	}
	pCase, err := c.Profile(ctx, "SatyaNadella")
	if err != nil || pCase.Slug != "satyanadella" || pCase.URL != defaultBaseURL+"/in/satyanadella/" {
		t.Fatalf("unexpected canonical profile: %+v (%v)", pCase, err)
	}
	co, err := c.Company(ctx, "example-co")
	if err != nil || co.URL != defaultBaseURL+"/company/example-co/" || co.Industry != "Technology" {
		t.Fatalf("unexpected company profile: %+v (%v)", co, err)
	}
	coCase, err := c.Company(ctx, "Microsoft")
	if err != nil || coCase.Slug != "microsoft" || coCase.URL != defaultBaseURL+"/company/microsoft/" {
		t.Fatalf("unexpected canonical company: %+v (%v)", coCase, err)
	}
	if _, err := c.Profile(ctx, "satyanadella"); err == nil {
		t.Fatal("expected invalid upstream slug to be rejected")
	} else {
		assertContractError(t, err)
	}
	if _, err := c.Profile(ctx, "missing"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected missing profile error, got %v", err)
	}
	if _, err := c.Company(ctx, "missing"); !errors.Is(err, ErrCompanyNotFound) {
		t.Fatalf("expected missing company error, got %v", err)
	}
	if _, err := c.Company(ctx, "wrong-content"); err == nil {
		t.Fatal("expected a slug mismatch error")
	} else {
		assertContractError(t, err)
	}
	if _, err := c.Profile(ctx, "invalid/slug"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid profile slug, got %v", err)
	}
	if _, err := c.Company(ctx, "invalid/slug"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid company slug, got %v", err)
	}
	if len(mock.renderedCalls) != 4 || len(mock.readCalls) != 4 {
		t.Fatalf("expected 4 rendered (profiles) and 4 standard (companies) calls, got rendered=%d read=%d", len(mock.renderedCalls), len(mock.readCalls))
	}
}

func TestCompanyRichFieldsAndValidation(t *testing.T) {
	richPayload := `{"company":{
		"slug":"microsoft",
		"url":"ignored",
		"name":"Microsoft",
		"id":"1035",
		"tagline":"Every company has a mission. What's ours? To empower every person and every organization to achieve more.",
		"description":"Microsoft enables digital transformation.",
		"website":"https://news.microsoft.com/",
		"phone":"+1 425-882-8080",
		"industry":"Software Development",
		"company_size":"+10.001 empleados",
		"associated_members_count":231102,
		"associated_members_label":"231.102 miembros asociados",
		"followers_count":23000000,
		"followers_label":"23 M seguidores",
		"employees_label":"231 mil empleados",
		"headquarters":"Redmond, Washington",
		"founded":"1975",
		"company_type":"Public Company",
		"specialties":"Business Software, Developer Tools",
		"verified":true,
		"verified_page":"10 de octubre de 2023",
		"premium":true,
		"logo_url":"https://media.licdn.com/dms/image/logo.png",
		"cover_image_url":"https://media.licdn.com/dms/image/cover.png",
		"highlights":["Great Place to Work"],
		"workplace_policy":{"title":"Trabajo híbrido","description":"Flexible hybrid work","default_presence":"Presencialidad por norma general","benefits":["Flexible hours","Health insurance"]},
		"commitments":[{"title":"Sustainability","description":"Carbon negative by 2030"}],
		"locations_count":45,
		"locations":[{"label":"Redmond","address":"One Microsoft Way, Redmond, WA, 98052, US","is_headquarters":true,"map_url":"https://www.bing.com/maps"}],
		"affiliated_pages":[{"slug":"github","url":"https://www.linkedin.com/company/github/","name":"GitHub","industry":"Software Development","followers_label":"5 M seguidores"}],
		"similar_pages":[{"slug":"google","url":"https://www.linkedin.com/company/google/","name":"Google","industry":"Software Development","followers_label":"35 M seguidores"}]
	}}`
	invalidPayloads := map[string]string{
		"bad-id":          `{"company":{"slug":"bad-id","name":"Bad ID","id":"0123"}}`,
		"bad-id-alpha":    `{"company":{"slug":"bad-id-alpha","name":"Bad Alpha","id":"abc"}}`,
		"neg-members":     `{"company":{"slug":"neg-members","name":"Neg","associated_members_count":-1}}`,
		"neg-followers":   `{"company":{"slug":"neg-followers","name":"Neg","followers_count":-5}}`,
		"neg-locations":   `{"company":{"slug":"neg-locations","name":"Neg","locations_count":-2}}`,
		"bad-website":     `{"company":{"slug":"bad-website","name":"Web","website":"/relative"}}`,
		"empty-wp-title":  `{"company":{"slug":"empty-wp-title","name":"WP","workplace_policy":{"title":"  "}}}`,
		"empty-commit":    `{"company":{"slug":"empty-commit","name":"Commit","commitments":[{"title":""}]}}`,
		"empty-location":  `{"company":{"slug":"empty-location","name":"Loc","locations":[{"is_headquarters":true}]}}`,
		"bad-related":     `{"company":{"slug":"bad-related","name":"Rel","affiliated_pages":[{"slug":"../bad","url":"https://www.linkedin.com/company/bad/","name":"Bad"}]}}`,
		"bad-related-url": `{"company":{"slug":"bad-related-url","name":"Rel","similar_pages":[{"slug":"google","url":"not-a-url","name":"Google"}]}}`,
	}
	c := mustNewClient(t, readerFunc(func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
		if expression == "" {
			t.Fatal("expected non-empty expression")
		}
		if pageURL == defaultBaseURL+"/company/microsoft/about/" {
			return json.RawMessage(richPayload), nil
		}
		for slug, body := range invalidPayloads {
			if pageURL == defaultBaseURL+"/company/"+slug+"/about/" {
				return json.RawMessage(body), nil
			}
		}
		t.Fatalf("unexpected upstream query %q", pageURL)
		return nil, nil
	}))
	ctx := context.Background()
	co, err := c.Company(ctx, "microsoft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if co.URL != defaultBaseURL+"/company/microsoft/" || co.ID != "1035" || !co.Verified || !co.Premium {
		t.Fatalf("unexpected company header: %+v", co)
	}
	if co.AssociatedMembersCount == nil || *co.AssociatedMembersCount != 231102 ||
		co.FollowersCount == nil || *co.FollowersCount != 23000000 ||
		co.LocationsCount == nil || *co.LocationsCount != 45 {
		t.Fatalf("unexpected company counts: %+v", co)
	}
	if co.WorkplacePolicy == nil || co.WorkplacePolicy.Title != "Trabajo híbrido" || len(co.WorkplacePolicy.Benefits) != 2 {
		t.Fatalf("unexpected workplace policy: %+v", co.WorkplacePolicy)
	}
	if len(co.Commitments) != 1 || co.Commitments[0].Title != "Sustainability" {
		t.Fatalf("unexpected commitments: %+v", co.Commitments)
	}
	if len(co.Locations) != 1 || !co.Locations[0].IsHeadquarters || co.Locations[0].Label != "Redmond" {
		t.Fatalf("unexpected locations: %+v", co.Locations)
	}
	if len(co.AffiliatedPages) != 1 || co.AffiliatedPages[0].Slug != "github" ||
		len(co.SimilarPages) != 1 || co.SimilarPages[0].Slug != "google" {
		t.Fatalf("unexpected related pages: affiliated=%+v similar=%+v", co.AffiliatedPages, co.SimilarPages)
	}
	for slug := range invalidPayloads {
		t.Run("invalid/"+slug, func(t *testing.T) {
			_, err := c.Company(ctx, slug)
			assertContractError(t, err)
		})
	}
}

func TestProfileRichFieldsAndValidation(t *testing.T) {
	richPayload := `{"profile":{
		"slug":"satyanadella",
		"url":"ignored",
		"name":"Satya Nadella",
		"pronouns":"he/him",
		"verified":true,
		"premium":true,
		"top_voice":true,
		"connection_degree":"2nd",
		"headline":"Chairman and CEO at Microsoft",
		"location":"Redmond, Washington, United States",
		"profile_image_url":"https://media.licdn.com/dms/image/profile.jpg",
		"background_image_url":"https://media.licdn.com/dms/image/bg.jpg",
		"followers_label":"11345678 seguidores",
		"followers_count":11345678,
		"connections_label":"500+ contactos",
		"current_company_label":"Microsoft",
		"education_label":"The University of Chicago Booth School of Business",
		"website":"https://news.microsoft.com/",
		"open_to_work":true,
		"hiring":true,
		"about":"Empowering every person and every organization on the planet to achieve more.",
		"top_skills":["Cloud Computing","Leadership"],
		"featured":[{"title":"Annual Letter","type":"Article","url":"https://www.linkedin.com/pulse/annual-letter/"}],
		"activity":[{"url":"https://www.linkedin.com/feed/update/urn:li:activity:1/","published_label":"2d","text":"Excited to share","author_name":"Satya Nadella","author_slug":"satyanadella"}],
		"experience":[{"title":"Chairman and CEO","company":"Microsoft","company_url":"https://www.linkedin.com/company/microsoft/","company_slug":"microsoft","employment_type":"Full-time","date_range":"feb. 2014 - actualidad","duration":"10 yrs","location":"Redmond, WA","description":"Leading Microsoft","current":true}],
		"education":[{"school":"The University of Chicago Booth School of Business","school_url":"https://www.linkedin.com/school/chicagobooth/","school_slug":"chicagobooth","degree":"MBA","date_range":"1994 - 1996","description":"Business Administration"}],
		"certifications":[{"name":"Cloud Architect","issuer":"Microsoft","issue_date":"2020","credential_id":"CERT-123","credential_url":"https://learn.microsoft.com/cert/123"}],
		"skills":[{"name":"Distributed Systems","endorsements_label":"99 validaciones"}],
		"languages":[{"name":"English","proficiency":"Native or bilingual proficiency"}],
		"projects":[{"title":"Project Natick","date_range":"2018 - 2020","description":"Underwater datacenter","url":"https://natick.research.microsoft.com/"}],
		"volunteering":[{"role":"Board Member","organization":"Fred Hutch","date_range":"2016 - 2022","cause":"Health","description":"Cancer research"}],
		"publications":[{"title":"Hit Refresh","publisher":"HarperBusiness","date":"2017","description":"Book on transformation","url":"https://news.microsoft.com/hit-refresh/"}],
		"honors":[{"title":"Padma Bhushan","issuer":"Government of India","date":"2022","description":"Trade and Industry"}],
		"recommendations":[{"author_name":"Bill Gates","author_slug":"williamhgates","author_url":"https://www.linkedin.com/in/williamhgates/","author_headline":"Co-chair, Bill & Melinda Gates Foundation","relationship":"Worked with Satya at Microsoft","text":"Great leader."}],
		"interests":[{"name":"Microsoft","kind":"company","slug":"microsoft","url":"https://www.linkedin.com/company/microsoft/","headline":"Software Development","followers_label":"23 M seguidores"}]
	}}`
	invalidPayloads := map[string]string{
		"neg-followers":       `{"profile":{"slug":"neg-followers","name":"Neg","followers_count":-1}}`,
		"bad-website":         `{"profile":{"slug":"bad-website","name":"Web","website":"javascript:alert(1)"}}`,
		"empty-featured":      `{"profile":{"slug":"empty-featured","name":"Feat","featured":[{"type":"Post"}]}}`,
		"empty-activity":      `{"profile":{"slug":"empty-activity","name":"Act","activity":[{"published_label":"1d"}]}}`,
		"empty-exp-title":     `{"profile":{"slug":"empty-exp-title","name":"Exp","experience":[{"company":"Acme"}]}}`,
		"empty-edu-school":    `{"profile":{"slug":"empty-edu-school","name":"Edu","education":[{"degree":"BS"}]}}`,
		"empty-cert-name":     `{"profile":{"slug":"empty-cert-name","name":"Cert","certifications":[{"issuer":"Org"}]}}`,
		"empty-skill-name":    `{"profile":{"slug":"empty-skill-name","name":"Skill","skills":[{"endorsements_label":"5"}]}}`,
		"empty-lang-name":     `{"profile":{"slug":"empty-lang-name","name":"Lang","languages":[{"proficiency":"Fluent"}]}}`,
		"empty-proj-title":    `{"profile":{"slug":"empty-proj-title","name":"Proj","projects":[{"date_range":"2020"}]}}`,
		"empty-vol-role":      `{"profile":{"slug":"empty-vol-role","name":"Vol","volunteering":[{"organization":"Red Cross"}]}}`,
		"empty-pub-title":     `{"profile":{"slug":"empty-pub-title","name":"Pub","publications":[{"publisher":"Press"}]}}`,
		"empty-honor-title":   `{"profile":{"slug":"empty-honor-title","name":"Honor","honors":[{"issuer":"Org"}]}}`,
		"empty-rec-author":    `{"profile":{"slug":"empty-rec-author","name":"Rec","recommendations":[{"author_slug":"jane-doe","author_url":"https://www.linkedin.com/in/jane-doe/"}]}}`,
		"bad-rec-slug":        `{"profile":{"slug":"bad-rec-slug","name":"Rec","recommendations":[{"author_name":"Jane","author_slug":"../bad","author_url":"https://www.linkedin.com/in/bad/"}]}}`,
		"empty-rec-url":       `{"profile":{"slug":"empty-rec-url","name":"Rec","recommendations":[{"author_name":"Jane","author_slug":"jane-doe"}]}}`,
		"empty-interest-name": `{"profile":{"slug":"empty-interest-name","name":"Int","interests":[{"kind":"company","slug":"microsoft","url":"https://www.linkedin.com/company/microsoft/"}]}}`,
		"bad-interest-kind":   `{"profile":{"slug":"bad-interest-kind","name":"Int","interests":[{"name":"Microsoft","kind":"unknown","slug":"microsoft","url":"https://www.linkedin.com/company/microsoft/"}]}}`,
		"empty-interest-slug": `{"profile":{"slug":"empty-interest-slug","name":"Int","interests":[{"name":"Microsoft","kind":"company","url":"https://www.linkedin.com/company/microsoft/"}]}}`,
	}
	c := mustNewClient(t, readerFunc(func(_ context.Context, pageURL, expression string) (json.RawMessage, error) {
		if expression == "" {
			t.Fatal("expected non-empty expression")
		}
		if pageURL == defaultBaseURL+"/in/satyanadella/" {
			return json.RawMessage(richPayload), nil
		}
		for slug, body := range invalidPayloads {
			if pageURL == defaultBaseURL+"/in/"+slug+"/" {
				return json.RawMessage(body), nil
			}
		}
		t.Fatalf("unexpected upstream query %q", pageURL)
		return nil, nil
	}))
	ctx := context.Background()
	p, err := c.Profile(ctx, "satyanadella")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.URL != defaultBaseURL+"/in/satyanadella/" || p.Pronouns != "he/him" || !p.Verified || !p.Premium || !p.TopVoice ||
		!p.OpenToWork || !p.Hiring || p.ConnectionDegree != "2nd" {
		t.Fatalf("unexpected top card badges/fields: %+v", p)
	}
	if p.FollowersCount == nil || *p.FollowersCount != 11345678 || p.FollowersLabel != "11345678 seguidores" ||
		p.ConnectionsLabel != "500+ contactos" || p.CurrentCompanyLabel != "Microsoft" ||
		p.EducationLabel != "The University of Chicago Booth School of Business" ||
		p.Website != "https://news.microsoft.com/" ||
		p.ProfileImageURL == "" || p.BackgroundImageURL == "" {
		t.Fatalf("unexpected profile counts/media/metadata: %+v", p)
	}
	if len(p.TopSkills) != 2 || len(p.Featured) != 1 || len(p.Activity) != 1 {
		t.Fatalf("unexpected summary sections: top_skills=%+v featured=%+v activity=%+v", p.TopSkills, p.Featured, p.Activity)
	}
	if len(p.Experience) != 1 || !p.Experience[0].Current || p.Experience[0].CompanySlug != "microsoft" ||
		len(p.Education) != 1 || p.Education[0].SchoolSlug != "chicagobooth" ||
		len(p.Certifications) != 1 || p.Certifications[0].CredentialID != "CERT-123" ||
		len(p.Skills) != 1 || p.Skills[0].EndorsementsLabel != "99 validaciones" ||
		len(p.Languages) != 1 || p.Languages[0].Proficiency == "" ||
		len(p.Projects) != 1 || p.Projects[0].URL == "" ||
		len(p.Volunteering) != 1 || p.Volunteering[0].Cause != "Health" ||
		len(p.Publications) != 1 || p.Publications[0].Publisher != "HarperBusiness" ||
		len(p.Honors) != 1 || p.Honors[0].Issuer != "Government of India" ||
		len(p.Recommendations) != 1 || p.Recommendations[0].AuthorSlug != "williamhgates" ||
		len(p.Interests) != 1 || p.Interests[0].Kind != "company" {
		t.Fatalf("unexpected profile sections: %+v", p)
	}
	for slug := range invalidPayloads {
		t.Run("invalid/"+slug, func(t *testing.T) {
			_, err := c.Profile(ctx, slug)
			assertContractError(t, err)
		})
	}
}
