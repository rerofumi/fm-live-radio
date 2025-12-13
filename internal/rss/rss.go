package rss

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

var ErrNoRSS = errors.New("no rss urls")
var ErrNoNewArticle = errors.New("no new article")

type Article struct {
	FeedTitle string
	FeedURL   string
	Title     string
	Link      string
	Content   string
}

type Picker struct {
	client  *http.Client
	parser  *gofeed.Parser
	maxTry  int
	maxItem int
}

func NewPicker() *Picker {
	return &Picker{
		client: &http.Client{Timeout: 10 * time.Second},
		parser: gofeed.NewParser(),
		maxTry: 5,
		// read at most N items per feed for speed
		maxItem: 30,
	}
}

func (p *Picker) Pick(ctx context.Context, rssUrls []string, used map[string]bool) (Article, error) {
	urls := make([]string, 0, len(rssUrls))
	for _, u := range rssUrls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		return Article{}, ErrNoRSS
	}

	// Try up to maxTry different feeds.
	for try := 0; try < p.maxTry; try++ {
		u := urls[rand.IntN(len(urls))]
		art, ok, err := p.pickFromOne(ctx, u, used)
		if err != nil {
			// silent fail for this feed
			continue
		}
		if ok {
			return art, nil
		}
	}
	return Article{}, ErrNoNewArticle
}

func (p *Picker) pickFromOne(ctx context.Context, feedURL string, used map[string]bool) (Article, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return Article{}, false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return Article{}, false, err
	}
	defer resp.Body.Close()

	feed, err := p.parser.Parse(resp.Body)
	if err != nil {
		return Article{}, false, err
	}

	items := feed.Items
	if len(items) > p.maxItem {
		items = items[:p.maxItem]
	}
	for _, it := range items {
		link := strings.TrimSpace(it.Link)
		if link == "" {
			continue
		}
		if used != nil && used[link] {
			continue
		}
		content := strings.TrimSpace(it.Content)
		if content == "" {
			content = strings.TrimSpace(it.Description)
		}
		return Article{
			FeedTitle: feed.Title,
			FeedURL:   feedURL,
			Title:     strings.TrimSpace(it.Title),
			Link:      link,
			Content:   content,
		}, true, nil
	}
	return Article{}, false, nil
}
