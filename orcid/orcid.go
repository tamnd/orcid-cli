// Package orcid is the library behind the orcid command line:
// the HTTP client, request shaping, and typed data models for the ORCID
// public registry (pub.orcid.org).
//
// The ORCID public API is free and requires no API key. All requests must
// include Accept: application/json to receive JSON responses. The Client
// paces requests, retries transient failures (429 and 5xx) with exponential
// backoff, and decodes the deeply nested ORCID JSON into clean typed structs.
//
// Three operations are provided: person profile (Person), list of works (Works),
// and researcher search (Search).
package orcid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Host is the site this client talks to.
const Host = "pub.orcid.org"

// BaseURL is the API base path including the API version.
const BaseURL = "https://pub.orcid.org/v3.0"

// Config holds tunable knobs for the HTTP client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for production use.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		UserAgent: "orcid-cli/0.1.0 (github.com/tamnd/orcid-cli)",
		Rate:      200 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to pub.orcid.org over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client configured with cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// PersonURL is a researcher-provided URL.
type PersonURL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Person holds the profile data for one ORCID researcher.
type Person struct {
	ORCID      string      `json:"orcid"`
	GivenName  string      `json:"given_name"`
	FamilyName string      `json:"family_name"`
	Bio        string      `json:"bio,omitempty"`
	Keywords   []string    `json:"keywords,omitempty"`
	URLs       []PersonURL `json:"urls,omitempty"`
}

// Work holds one publication entry from an ORCID profile.
type Work struct {
	PutCode int64  `json:"put_code"`
	Title   string `json:"title"`
	Type    string `json:"type,omitempty"`
	Year    int    `json:"year,omitempty"`
	Journal string `json:"journal,omitempty"`
	DOI     string `json:"doi,omitempty"`
	URL     string `json:"url,omitempty"`
}

// SearchResult is one entry returned by the search endpoint.
type SearchResult struct {
	ORCID      string `json:"orcid"`
	GivenName  string `json:"given_name,omitempty"`
	FamilyName string `json:"family_name,omitempty"`
}

// --- wire types ---

type wirePerson struct {
	Name struct {
		GivenNames struct {
			Value string `json:"value"`
		} `json:"given-names"`
		FamilyName struct {
			Value string `json:"value"`
		} `json:"family-name"`
	} `json:"name"`
	Biography struct {
		Content string `json:"content"`
	} `json:"biography"`
	Keywords struct {
		Keyword []struct {
			Content string `json:"content"`
		} `json:"keyword"`
	} `json:"keywords"`
	ResearcherUrls struct {
		ResearcherUrl []struct {
			UrlName string `json:"url-name"`
			Url     struct {
				Value string `json:"value"`
			} `json:"url"`
		} `json:"researcher-url"`
	} `json:"researcher-urls"`
}

type wireWorksSummary struct {
	Group []struct {
		WorkSummary []struct {
			PutCode int64 `json:"put-code"`
			Title   struct {
				Title struct {
					Value string `json:"value"`
				} `json:"title"`
			} `json:"title"`
			Type            string `json:"type"`
			PublicationDate struct {
				Year struct {
					Value string `json:"value"`
				} `json:"year"`
			} `json:"publication-date"`
			JournalTitle struct {
				Value string `json:"value"`
			} `json:"journal-title"`
			ExternalIds struct {
				ExternalId []struct {
					Type  string `json:"external-id-type"`
					Value string `json:"external-id-value"`
					URL   struct {
						Value string `json:"value"`
					} `json:"external-id-url"`
				} `json:"external-id"`
			} `json:"external-ids"`
		} `json:"work-summary"`
	} `json:"group"`
}

type wireSearchResp struct {
	Result []struct {
		OrcidIdentifier struct {
			Path string `json:"path"`
		} `json:"orcid-identifier"`
	} `json:"result"`
	NumFound int `json:"num-found"`
}

// Person fetches the researcher profile for the given ORCID identifier.
// The orcidID must be in the format 0000-0002-1825-0097.
func (c *Client) Person(ctx context.Context, orcidID string) (*Person, error) {
	u := c.cfg.BaseURL + "/" + orcidID + "/person"
	b, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wire wirePerson
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("decode person response: %w", err)
	}
	p := &Person{
		ORCID:      orcidID,
		GivenName:  wire.Name.GivenNames.Value,
		FamilyName: wire.Name.FamilyName.Value,
		Bio:        wire.Biography.Content,
	}
	for _, kw := range wire.Keywords.Keyword {
		if kw.Content != "" {
			p.Keywords = append(p.Keywords, kw.Content)
		}
	}
	for _, ru := range wire.ResearcherUrls.ResearcherUrl {
		if ru.Url.Value != "" {
			p.URLs = append(p.URLs, PersonURL{Name: ru.UrlName, URL: ru.Url.Value})
		}
	}
	return p, nil
}

// Works fetches the list of works for the given ORCID identifier.
// It returns up to limit works; pass 0 for all.
func (c *Client) Works(ctx context.Context, orcidID string, limit int) ([]Work, error) {
	u := c.cfg.BaseURL + "/" + orcidID + "/works"
	b, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wire wireWorksSummary
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("decode works response: %w", err)
	}
	var out []Work
	for _, grp := range wire.Group {
		if len(grp.WorkSummary) == 0 {
			continue
		}
		ws := grp.WorkSummary[0] // first summary is most preferred
		w := Work{
			PutCode: ws.PutCode,
			Title:   ws.Title.Title.Value,
			Type:    ws.Type,
			Journal: ws.JournalTitle.Value,
		}
		if yr := ws.PublicationDate.Year.Value; yr != "" {
			if n, err := strconv.Atoi(yr); err == nil {
				w.Year = n
			}
		}
		for _, eid := range ws.ExternalIds.ExternalId {
			if eid.Type == "doi" {
				w.DOI = eid.Value
				if eid.URL.Value != "" {
					w.URL = eid.URL.Value
				}
				break
			}
		}
		if w.URL == "" {
			for _, eid := range ws.ExternalIds.ExternalId {
				if eid.URL.Value != "" {
					w.URL = eid.URL.Value
					break
				}
			}
		}
		out = append(out, w)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Search searches ORCID researchers by keyword query.
// It returns up to limit results.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	u := c.cfg.BaseURL + "/search/?q=" + url.QueryEscape(query) +
		"&start=0&rows=" + strconv.Itoa(limit)
	b, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wire wireSearchResp
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	out := make([]SearchResult, 0, len(wire.Result))
	for _, r := range wire.Result {
		out = append(out, SearchResult{ORCID: r.OrcidIdentifier.Path})
	}
	return out, nil
}

// get fetches url and returns the response body. It paces and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
