package rss

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
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

const minUsefulContentRunes = 120

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
		if len([]rune(content)) < minUsefulContentRunes {
			if pageContent, err := p.fetchArticleContent(ctx, link); err == nil && len([]rune(pageContent)) >= minUsefulContentRunes {
				content = pageContent
			}
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

func (p *Picker) fetchArticleContent(ctx context.Context, articleURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	candidates := p.articleSelectors(articleURL)
	for _, candidate := range candidates {
		text := collectStructuredText(doc, candidate)
		if len([]rune(text)) >= minUsefulContentRunes {
			return text, nil
		}
	}

	best := ""
	for _, candidate := range candidates {
		text := collectStructuredText(doc, candidate)
		if len([]rune(text)) > len([]rune(best)) {
			best = text
		}
	}
	return best, nil
}

func (p *Picker) articleSelectors(articleURL string) []string {
	u, err := url.Parse(articleURL)
	if err == nil {
		host := strings.ToLower(u.Hostname())
		switch {
		case strings.HasSuffix(host, "pc.watch.impress.co.jp"), strings.HasSuffix(host, "forest.watch.impress.co.jp"):
			return []string{
				"#main .main-contents .contents-section > p",
				"#main .main-contents .contents-section > ul > li",
				"#main .main-contents .contents-section > ol > li",
				"article[role='main'] .main-contents .contents-section > p",
				"article[role='main'] .main-contents .contents-section > ul > li",
				"article[role='main'] .main-contents .contents-section > ol > li",
			}
		}
	}

	return []string{
		"article p",
		"article li",
		"main p",
		"main li",
		".article-body p",
		".article__body p",
		".entry-content p",
		".post-content p",
		"#article p",
	}
}

func collectStructuredText(doc *goquery.Document, selector string) string {
	parts := make([]string, 0, 16)
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		text := normalizeText(s.Text())
		if len([]rune(text)) < 10 {
			return
		}
		parts = append(parts, text)
	})
	return strings.Join(parts, "\n")
}

func normalizeText(s string) string {
	lines := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	return strings.TrimSpace(strings.Join(lines, " "))
}
