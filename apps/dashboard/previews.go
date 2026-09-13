package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A preview is one pull request's environment: a container Traefik routes to.
type preview struct {
	Name    string // the container's
	Project string
	PR      int
	URL     string
	Repo    string // owner/name on GitHub
	Branch  string
	SHA     string
	RunURL  string // the Actions run that deployed it
	State   string // Docker's: running, exited, restarting, …
	Status  string // Docker's summary, like "Up 2 hours"
}

func (p preview) PRURL() string {
	if p.Repo == "" {
		return ""
	}
	return "https://github.com/" + p.Repo + "/pull/" + strconv.Itoa(p.PR)
}

func (p preview) CommitURL() string {
	if p.Repo == "" || p.SHA == "" {
		return ""
	}
	return "https://github.com/" + p.Repo + "/commit/" + p.SHA
}

func (p preview) ShortSHA() string {
	if len(p.SHA) > 7 {
		return p.SHA[:7]
	}
	return p.SHA
}

type project struct {
	Name     string
	Previews []preview
}

var (
	hostRule = regexp.MustCompile("Host\\(`([^`]+)`\\)")
	// prHost matches a preview's hostname: `pr-<number>.<domain>`, or the flat
	// `pr-<number>-<domain>` that a one-level wildcard certificate needs.
	prHost = regexp.MustCompile(`^pr-(\d+)[.-](.+)$`)
)

// previewOf reports whether c is a preview and describes it. The `preview.*`
// labels a project's workflow sets are authoritative; a container without
// them is recognised by the `pr-<number>` hostname in its Traefik rule.
func previewOf(c container) (preview, bool) {
	l := c.Labels
	if l["traefik.enable"] != "true" {
		return preview{}, false
	}
	p := preview{
		Name:    c.Name(),
		Project: l["preview.project"],
		URL:     l["preview.url"],
		Repo:    l["preview.repo"],
		Branch:  l["preview.branch"],
		SHA:     l["preview.sha"],
		RunURL:  l["preview.run-url"],
		State:   c.State,
		Status:  c.Status,
	}
	p.PR, _ = strconv.Atoi(l["preview.pr"])
	host := routerHost(l)
	if m := prHost.FindStringSubmatch(host); m != nil {
		if p.PR == 0 {
			p.PR, _ = strconv.Atoi(m[1])
		}
		if p.Project == "" {
			p.Project = projectOf(m[2])
		}
	}
	if p.PR <= 0 {
		return preview{}, false
	}
	if p.URL == "" && host != "" {
		p.URL = "https://" + host
	}
	if p.Project == "" {
		p.Project = "unknown"
	}
	return p, true
}

// routerHost returns the first hostname in the container's Traefik rules.
func routerHost(labels map[string]string) string {
	var rules []string
	for k := range labels {
		if strings.HasPrefix(k, "traefik.http.routers.") && strings.HasSuffix(k, ".rule") {
			rules = append(rules, k)
		}
	}
	slices.Sort(rules)
	for _, k := range rules {
		if m := hostRule.FindStringSubmatch(labels[k]); m != nil {
			return m[1]
		}
	}
	return ""
}

// projectOf names a project after its preview domain:
// `preview.myapp.com` → `myapp`.
func projectOf(domain string) string {
	name, _, _ := strings.Cut(strings.TrimPrefix(domain, "preview."), ".")
	return name
}

// byProject groups previews by project, newest pull request first.
func byProject(ps []preview) []project {
	slices.SortFunc(ps, func(a, b preview) int {
		if c := strings.Compare(a.Project, b.Project); c != 0 {
			return c
		}
		return b.PR - a.PR
	})
	var out []project
	for _, p := range ps {
		if len(out) == 0 || out[len(out)-1].Name != p.Project {
			out = append(out, project{Name: p.Project})
		}
		out[len(out)-1].Previews = append(out[len(out)-1].Previews, p)
	}
	return out
}
