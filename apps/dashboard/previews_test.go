package main

import "testing"

func TestPreviewOf(t *testing.T) {
	tests := []struct {
		name      string
		c         container
		ok        bool
		project   string
		pr        int
		url       string
		prURL     string
		commitURL string
	}{
		{
			name: "recognised by its Traefik rule",
			c: container{Names: []string{"/myapp-pr-253"}, State: "running", Labels: map[string]string{
				"traefik.enable":                   "true",
				"traefik.http.routers.pr-253.rule": "Host(`pr-253.preview.myapp.com`)",
			}},
			ok: true, project: "myapp", pr: 253, url: "https://pr-253.preview.myapp.com",
		},
		{
			name: "rule alongside TLS labels",
			c: container{Names: []string{"/shop-web-pr-24"}, Labels: map[string]string{
				"traefik.enable": "true",
				"traefik.http.routers.shop-web-pr-24.rule":                "Host(`pr-24.preview.shop.dev`)",
				"traefik.http.routers.shop-web-pr-24.tls.domains[0].main": "*.preview.shop.dev",
			}},
			ok: true, project: "shop", pr: 24, url: "https://pr-24.preview.shop.dev",
		},
		{
			name: "preview labels win",
			c: container{Names: []string{"/shop-web-pr-24"}, Labels: map[string]string{
				"traefik.enable":              "true",
				"traefik.http.routers.x.rule": "Host(`pr-24.preview.shop.dev`)",
				"preview.project":             "shop-web",
				"preview.pr":                  "24",
				"preview.repo":                "my-org/shop",
				"preview.sha":                 "9b68601abcdef",
			}},
			ok: true, project: "shop-web", pr: 24, url: "https://pr-24.preview.shop.dev",
			prURL:     "https://github.com/my-org/shop/pull/24",
			commitURL: "https://github.com/my-org/shop/commit/9b68601abcdef",
		},
		{
			name: "flat hostname",
			c: container{Names: []string{"/x"}, Labels: map[string]string{
				"traefik.enable":              "true",
				"traefik.http.routers.x.rule": "Host(`pr-7-preview.example.com`)",
			}},
			ok: true, project: "example", pr: 7, url: "https://pr-7-preview.example.com",
		},
		{
			name: "the dashboard itself",
			c: container{Names: []string{"/preview-host-dashboard-1"}, Labels: map[string]string{
				"traefik.enable":                         "true",
				"traefik.http.routers.preview-host.rule": "Host(`preview-host.example-tailnet.ts.net`)",
			}},
		},
		{
			name: "not routed",
			c: container{Names: []string{"/myapp-pr-1"}, Labels: map[string]string{
				"traefik.http.routers.pr-1.rule": "Host(`pr-1.preview.myapp.com`)",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := previewOf(tt.c)
			if ok != tt.ok {
				t.Fatalf("previewOf ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if p.Project != tt.project || p.PR != tt.pr || p.URL != tt.url {
				t.Errorf("got project %q, PR %d, URL %q; want %q, %d, %q", p.Project, p.PR, p.URL, tt.project, tt.pr, tt.url)
			}
			if p.PRURL() != tt.prURL || p.CommitURL() != tt.commitURL {
				t.Errorf("got links %q, %q; want %q, %q", p.PRURL(), p.CommitURL(), tt.prURL, tt.commitURL)
			}
		})
	}
}

func TestByProject(t *testing.T) {
	got := byProject([]preview{
		{Project: "shop", PR: 12},
		{Project: "myapp", PR: 3},
		{Project: "shop", PR: 250},
	})
	if len(got) != 2 || got[0].Name != "myapp" || got[1].Name != "shop" {
		t.Fatalf("projects = %+v", got)
	}
	if prs := got[1].Previews; prs[0].PR != 250 || prs[1].PR != 12 {
		t.Errorf("shop previews not newest first: %+v", prs)
	}
}
