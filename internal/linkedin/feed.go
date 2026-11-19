package linkedin

import (
	"context"
	"errors"
)

const MaxFeedLimit = 30

// FeedQuery with a zero limit returns the first rendered batch without moving
// the feed. A nonzero limit enables bounded scrolling and loading.
type FeedQuery struct {
	Limit int
}

type FeedResults struct {
	Items      []Post `json:"items"`
	Sort       string `json:"sort,omitempty"`
	HasMore    bool   `json:"has_more,omitempty"`
	LoadStatus string `json:"load_status"`
}

type Post struct {
	ID             string          `json:"id"`
	Text           string          `json:"text"`
	Content        string          `json:"content"`
	URL            string          `json:"url,omitempty"`
	Author         *FeedAuthor     `json:"author,omitempty"`
	PublishedLabel string          `json:"published_label,omitempty"`
	Hashtags       []string        `json:"hashtags,omitempty"`
	Media          []FeedMedia     `json:"media,omitempty"`
	Engagement     *FeedEngagement `json:"engagement,omitempty"`
	Sponsored      bool            `json:"sponsored,omitempty"`
}

type FeedAuthor struct {
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type FeedMedia struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	Alt  string `json:"alt,omitempty"`
}

type FeedEngagement struct {
	ReactionCount *int64 `json:"reaction_count,omitempty"`
	CommentCount  *int64 `json:"comment_count,omitempty"`
	RepostCount   *int64 `json:"repost_count,omitempty"`
}

func (c Client) Feed(ctx context.Context, q FeedQuery) (FeedResults, error) {
	if q.Limit < 0 || q.Limit > MaxFeedLimit {
		return FeedResults{}, errors.New("feed limit out of range")
	}
	inv, err := c.scripts().Feed(q.Limit)
	if err != nil {
		return FeedResults{}, err
	}
	result, err := read[FeedResults](ctx, c.Reader, "/feed/foryou/", inv.Expression)
	if err != nil {
		return FeedResults{}, err
	}
	maxItems := q.Limit
	if maxItems == 0 {
		maxItems = 15
	}
	if result.Items == nil || len(result.Items) > maxItems || (q.Limit == 0) != (result.LoadStatus == "initial") {
		return FeedResults{}, errors.New("incomplete or inconsistent feed result from Chrome")
	}
	switch result.LoadStatus {
	case "initial", "limit_reached", "budget_reached", "no_more_observed", "no_progress":
	default:
		return FeedResults{}, errors.New("unrecognized feed load status from Chrome")
	}
	seen := make(map[string]struct{}, len(result.Items))
	for _, post := range result.Items {
		if _, duplicate := seen[post.ID]; duplicate || post.ID == "" || post.Text == "" {
			return FeedResults{}, errors.New("invalid or duplicate feed card from Chrome")
		}
		seen[post.ID] = struct{}{}
	}
	return result, nil
}
