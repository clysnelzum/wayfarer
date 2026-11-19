package linkedin

// MaxFeedLimit is the maximum number of feed items that may be requested in one read.
const MaxFeedLimit = 30

// FeedResults holds the cards observed in a personalized LinkedIn feed read.
type FeedResults struct {
	Items      []FeedItem `json:"items"`
	Sort       string     `json:"sort,omitempty"`
	HasMore    bool       `json:"has_more,omitempty"`
	LoadStatus string     `json:"load_status"`
}

// FeedItem represents a single rendered card from the LinkedIn feed.
type FeedItem struct {
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

// Post is an alias for FeedItem preserved for compatibility.
type Post = FeedItem

// FeedAuthor identifies the person or company that authored a feed post.
type FeedAuthor struct {
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// FeedMedia describes an image or video attachment on a feed post.
type FeedMedia struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	Alt  string `json:"alt,omitempty"`
}

// FeedEngagement holds exact observed engagement counts when not abbreviated by LinkedIn.
type FeedEngagement struct {
	ReactionCount *int64 `json:"reaction_count,omitempty"`
	CommentCount  *int64 `json:"comment_count,omitempty"`
	RepostCount   *int64 `json:"repost_count,omitempty"`
}
