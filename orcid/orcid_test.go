package orcid_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/orcid-cli/orcid"
)

func newTestClient(ts *httptest.Server) *orcid.Client {
	cfg := orcid.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 3
	return orcid.NewClient(cfg)
}

const mockPersonResponse = `{
  "name": {
    "given-names": {"value": "Josiah"},
    "family-name": {"value": "Carberry"},
    "credit-name": null,
    "visibility": "public"
  },
  "biography": {
    "content": "Josiah Carberry is a fictional professor of psychoceramics.",
    "visibility": "public"
  },
  "keywords": {
    "keyword": [
      {"content": "psychoceramics"},
      {"content": "broken pots"}
    ]
  },
  "researcher-urls": {
    "researcher-url": [
      {
        "url-name": "Website",
        "url": {"value": "https://library.brown.edu/cds/josiah/"}
      }
    ]
  },
  "emails": {"email": []},
  "addresses": {"address": []}
}`

const mockWorksResponse = `{
  "group": [
    {
      "work-summary": [
        {
          "put-code": 1234567,
          "title": {"title": {"value": "On the Cracking of Pots"}},
          "type": "JOURNAL_ARTICLE",
          "publication-date": {"year": {"value": "2020"}, "month": null, "day": null},
          "journal-title": {"value": "Journal of Psychoceramics"},
          "external-ids": {
            "external-id": [
              {
                "external-id-type": "doi",
                "external-id-value": "10.1234/crackingpots",
                "external-id-url": {"value": "https://doi.org/10.1234/crackingpots"},
                "external-id-relationship": "SELF"
              }
            ]
          },
          "visibility": "PUBLIC",
          "path": "/0000-0002-1825-0097/work/1234567"
        }
      ]
    },
    {
      "work-summary": [
        {
          "put-code": 7654321,
          "title": {"title": {"value": "Broken Pottery Through the Ages"}},
          "type": "BOOK",
          "publication-date": {"year": {"value": "2015"}, "month": null, "day": null},
          "journal-title": {"value": null},
          "external-ids": {"external-id": []},
          "visibility": "PUBLIC",
          "path": "/0000-0002-1825-0097/work/7654321"
        }
      ]
    }
  ]
}`

const mockSearchResponse = `{
  "result": [
    {"orcid-identifier": {"path": "0000-0002-1825-0097", "host": "orcid.org", "uri": "https://orcid.org/0000-0002-1825-0097"}},
    {"orcid-identifier": {"path": "0000-0001-2345-6789", "host": "orcid.org", "uri": "https://orcid.org/0000-0001-2345-6789"}}
  ],
  "num-found": 2
}`

// TestPersonParsesNameAndBio checks that name, biography, and keywords are
// extracted from the person response.
func TestPersonParsesNameAndBio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockPersonResponse)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	p, err := c.Person(context.Background(), "0000-0002-1825-0097")
	if err != nil {
		t.Fatal(err)
	}
	if p.GivenName != "Josiah" {
		t.Errorf("GivenName = %q, want %q", p.GivenName, "Josiah")
	}
	if p.FamilyName != "Carberry" {
		t.Errorf("FamilyName = %q, want %q", p.FamilyName, "Carberry")
	}
	if p.Bio == "" {
		t.Error("Bio is empty, expected a biography")
	}
	if len(p.Keywords) != 2 {
		t.Errorf("len(Keywords) = %d, want 2", len(p.Keywords))
	}
	if p.Keywords[0] != "psychoceramics" {
		t.Errorf("Keywords[0] = %q, want %q", p.Keywords[0], "psychoceramics")
	}
}

// TestPersonSetsAcceptHeader verifies that every request carries
// Accept: application/json (required by ORCID API).
func TestPersonSetsAcceptHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if accept != "application/json" {
			t.Errorf("Accept = %q, want application/json", accept)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockPersonResponse)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Person(context.Background(), "0000-0002-1825-0097")
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorksParsesTitle verifies that work title, type, year, and DOI are extracted.
func TestWorksParsesTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockWorksResponse)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	works, err := c.Works(context.Background(), "0000-0002-1825-0097", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 2 {
		t.Fatalf("len(works) = %d, want 2", len(works))
	}
	if works[0].Title != "On the Cracking of Pots" {
		t.Errorf("works[0].Title = %q, want %q", works[0].Title, "On the Cracking of Pots")
	}
	if works[0].Type != "JOURNAL_ARTICLE" {
		t.Errorf("works[0].Type = %q, want JOURNAL_ARTICLE", works[0].Type)
	}
	if works[0].Year != 2020 {
		t.Errorf("works[0].Year = %d, want 2020", works[0].Year)
	}
	if works[0].DOI != "10.1234/crackingpots" {
		t.Errorf("works[0].DOI = %q, want %q", works[0].DOI, "10.1234/crackingpots")
	}
}

// TestSearchParsesORCID verifies that search results contain ORCID IDs.
func TestSearchParsesORCID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockSearchResponse)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "psychoceramics", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].ORCID != "0000-0002-1825-0097" {
		t.Errorf("results[0].ORCID = %q, want %q", results[0].ORCID, "0000-0002-1825-0097")
	}
}

// TestPersonHTTPError verifies that HTTP 404 yields a non-nil error.
func TestPersonHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Person(context.Background(), "0000-0000-0000-0000")
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

// TestPersonRetriesOn503 verifies that the client retries on 5xx responses.
func TestPersonRetriesOn503(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, mockPersonResponse)
	}))
	defer srv.Close()

	cfg := orcid.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := orcid.NewClient(cfg)

	start := time.Now()
	p, err := c.Person(context.Background(), "0000-0002-1825-0097")
	if err != nil {
		t.Fatal(err)
	}
	if p.GivenName != "Josiah" {
		t.Errorf("GivenName = %q after retries", p.GivenName)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
