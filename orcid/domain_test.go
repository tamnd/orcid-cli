package orcid

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the domain driver's wiring, which
// need no network. The client's HTTP behaviour is covered in orcid_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "orcid" {
		t.Errorf("Scheme = %q, want orcid", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "orcid" {
		t.Errorf("Identity.Binary = %q, want orcid", info.Identity.Binary)
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the domain is
// registered and can be looked up.
func TestHostWiring(t *testing.T) {
	_, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	dom, ok := kit.Lookup("orcid")
	if !ok {
		t.Fatal("domain orcid not registered")
	}
	info := dom.Info()
	if info.Scheme != "orcid" {
		t.Errorf("Scheme = %q, want orcid", info.Scheme)
	}
}
