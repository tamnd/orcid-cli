// domain.go exposes orcid as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/orcid-cli/orcid"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// orcid:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone orcid binary (see cli.NewApp), so the
// binary and a host share one source of truth.
package orcid

import (
	"context"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the orcid driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "orcid",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "orcid",
			Short:  "Fetch researcher profiles from the ORCID public registry.",
			Long: `orcid fetches researcher profiles and publications from the ORCID public
registry (pub.orcid.org). No API key required.

It returns researcher bios, keywords, URLs, and publication lists for any
ORCID identifier (format: 0000-0002-1825-0097).`,
			Site: Host,
			Repo: "https://github.com/tamnd/orcid-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name: "person", Group: "read", Single: true,
		Summary: "Fetch researcher bio, keywords, and URLs by ORCID ID",
		URIType: "person", Resolver: true,
		Args: []kit.Arg{{Name: "id", Help: "ORCID identifier (0000-0002-XXXX-XXXX)"}},
	}, getPerson)

	kit.Handle(app, kit.OpMeta{
		Name: "works", Group: "read", List: true,
		Summary: "List publications by ORCID ID",
		URIType: "work",
		Args:    []kit.Arg{{Name: "id", Help: "ORCID identifier (0000-0002-XXXX-XXXX)"}},
	}, getWorks)

	kit.Handle(app, kit.OpMeta{
		Name: "search", Group: "read", List: true,
		Summary: "Search researchers by keyword or name",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, searchResearchers)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type personInput struct {
	ID     string  `kit:"arg" help:"ORCID identifier (0000-0002-XXXX-XXXX)"`
	Client *Client `kit:"inject"`
}

type worksInput struct {
	ID     string  `kit:"arg" help:"ORCID identifier (0000-0002-XXXX-XXXX)"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Query  string  `kit:"arg" help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getPerson(ctx context.Context, in personInput, emit func(*Person) error) error {
	p, err := in.Client.Person(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(p)
}

func getWorks(ctx context.Context, in worksInput, emit func(*Work) error) error {
	works, err := in.Client.Works(ctx, in.ID, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range works {
		if err := emit(&works[i]); err != nil {
			return err
		}
	}
	return nil
}

func searchResearchers(ctx context.Context, in searchInput, emit func(*SearchResult) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	results, err := in.Client.Search(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range results {
		if err := emit(&results[i]); err != nil {
			return err
		}
	}
	return nil
}

// mapErr converts a library error into the kit error kind with the right exit code.
func mapErr(err error) error {
	return errs.NotFound("%s", err.Error())
}
