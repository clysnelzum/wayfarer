package linkedin

import "strings"

const defaultFeedBatchLimit = 15

type wireFeedResults struct {
	Items      *[]wireFeedItem `json:"items"`
	Sort       string          `json:"sort"`
	HasMore    bool            `json:"has_more"`
	LoadStatus string          `json:"load_status"`
}

type wireFeedItem struct {
	ID             string              `json:"id"`
	Text           string              `json:"text"`
	Content        *string             `json:"content"`
	URL            string              `json:"url"`
	Author         *wireFeedAuthor     `json:"author"`
	PublishedLabel string              `json:"published_label"`
	Hashtags       []string            `json:"hashtags"`
	Media          []wireFeedMedia     `json:"media"`
	Engagement     *wireFeedEngagement `json:"engagement"`
	Sponsored      bool                `json:"sponsored"`
}

type wireFeedAuthor struct {
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type wireFeedMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Alt  string `json:"alt"`
}

type wireFeedEngagement struct {
	ReactionCount *int64 `json:"reaction_count"`
	CommentCount  *int64 `json:"comment_count"`
	RepostCount   *int64 `json:"repost_count"`
}

func (w wireFeedResults) toDomain(limit int) (FeedResults, error) {
	const op = "feed"
	if w.Items == nil {
		return FeedResults{}, contractError(op, "missing items array", nil)
	}
	maxItems := limit
	if maxItems == 0 {
		maxItems = defaultFeedBatchLimit
	}
	if len(*w.Items) > maxItems {
		return FeedResults{}, contractError(op, "items exceed requested limit", nil)
	}
	switch w.LoadStatus {
	case "initial", "limit_reached", "budget_reached", "no_more_observed", "no_progress":
	default:
		return FeedResults{}, contractError(op, "unrecognized feed load_status", nil)
	}
	if (limit == 0) != (w.LoadStatus == "initial") {
		return FeedResults{}, contractError(op, "inconsistent feed load_status", nil)
	}
	if w.Sort != "" && w.Sort != "recent" && w.Sort != "top" {
		return FeedResults{}, contractError(op, "unrecognized feed sort", nil)
	}

	items := make([]FeedItem, len(*w.Items))
	seen := make(map[string]struct{}, len(*w.Items))
	for i, raw := range *w.Items {
		item, err := raw.toDomain(op)
		if err != nil {
			return FeedResults{}, err
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return FeedResults{}, contractError(op, "duplicate feed item id", nil)
		}
		seen[item.ID] = struct{}{}
		items[i] = item
	}

	return FeedResults{
		Items:      items,
		Sort:       w.Sort,
		HasMore:    w.HasMore,
		LoadStatus: w.LoadStatus,
	}, nil
}

func (w wireFeedItem) toDomain(op string) (FeedItem, error) {
	if strings.TrimSpace(w.ID) == "" {
		return FeedItem{}, contractError(op, "missing feed item id", nil)
	}
	if strings.TrimSpace(w.Text) == "" {
		return FeedItem{}, contractError(op, "missing feed item text", nil)
	}
	if w.Content == nil {
		return FeedItem{}, contractError(op, "missing feed item content", nil)
	}
	if w.URL != "" && !validHTTPURL(w.URL) {
		return FeedItem{}, contractError(op, "invalid feed item url", nil)
	}

	var author *FeedAuthor
	if w.Author != nil {
		if w.Author.Kind != "person" && w.Author.Kind != "company" {
			return FeedItem{}, contractError(op, "invalid feed author kind", nil)
		}
		if !ValidSlug(w.Author.Slug) || strings.TrimSpace(w.Author.Name) == "" || !validHTTPURL(w.Author.URL) {
			return FeedItem{}, contractError(op, "invalid feed author", nil)
		}
		author = &FeedAuthor{
			Kind: w.Author.Kind,
			Slug: w.Author.Slug,
			Name: w.Author.Name,
			URL:  w.Author.URL,
		}
	}

	var hashtags []string
	if len(w.Hashtags) > 0 {
		hashtags = make([]string, len(w.Hashtags))
		for i, tag := range w.Hashtags {
			if strings.TrimSpace(tag) == "" {
				return FeedItem{}, contractError(op, "empty feed hashtag", nil)
			}
			hashtags[i] = tag
		}
	}

	var media []FeedMedia
	if len(w.Media) > 0 {
		media = make([]FeedMedia, len(w.Media))
		for i, m := range w.Media {
			if m.Type != "image" && m.Type != "video" {
				return FeedItem{}, contractError(op, "invalid feed media type", nil)
			}
			if m.URL != "" && !validHTTPURL(m.URL) {
				return FeedItem{}, contractError(op, "invalid feed media url", nil)
			}
			media[i] = FeedMedia{
				Type: m.Type,
				URL:  m.URL,
				Alt:  m.Alt,
			}
		}
	}

	var engagement *FeedEngagement
	if w.Engagement != nil {
		for _, count := range []*int64{w.Engagement.ReactionCount, w.Engagement.CommentCount, w.Engagement.RepostCount} {
			if count != nil && *count < 0 {
				return FeedItem{}, contractError(op, "negative feed engagement count", nil)
			}
		}
		engagement = &FeedEngagement{
			ReactionCount: w.Engagement.ReactionCount,
			CommentCount:  w.Engagement.CommentCount,
			RepostCount:   w.Engagement.RepostCount,
		}
	}

	return FeedItem{
		ID:             w.ID,
		Text:           w.Text,
		Content:        *w.Content,
		URL:            w.URL,
		Author:         author,
		PublishedLabel: w.PublishedLabel,
		Hashtags:       hashtags,
		Media:          media,
		Engagement:     engagement,
		Sponsored:      w.Sponsored,
	}, nil
}
