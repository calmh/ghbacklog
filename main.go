package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/calmh/github"
)

var (
	repo          = "syncthing/syncthing"
	listen        = ":8080"
	cacheTime     = time.Hour
	includeDue    = true
	includeNonDue = false
)

func main() {
	flag.StringVar(&repo, "repo", repo, "Repository name")
	flag.StringVar(&listen, "listen", listen, "Listen address")
	flag.DurationVar(&cacheTime, "cache", cacheTime, "Cache life time")
	flag.BoolVar(&includeDue, "due", includeDue, "Include milestone with a due date")
	flag.BoolVar(&includeNonDue, "nondue", includeNonDue, "Include milestone without a due date")
	flag.Parse()

	http.HandleFunc("/", handle)
	log.Fatal(http.ListenAndServe(listen, nil))
}

var cache = struct {
	data    []byte
	updated time.Time
	sync.Mutex
}{}

func handle(w http.ResponseWriter, req *http.Request) {
	cache.Lock()
	defer cache.Unlock()

	if time.Since(cache.updated) > cacheTime {
		cache.data = generateOverview()
		cache.updated = time.Now()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(cache.data)
}

type milestone struct {
	github.Milestone
	Issues []github.Issue
}

func generateOverview() []byte {
	fm := template.FuncMap{
		"split": strings.Split,
		"labelClass": func(label string) string {
			switch label {
			case "bug":
				return "danger"
			case "enhancement":
				return "success"
			default:
				return "default"
			}
		},
		"now": time.Now,
	}
	tmpl := template.Must(template.New("index.go.html").Funcs(fm).ParseGlob("*.go.html"))

	query := make(url.Values)
	query.Add("sort", "due_date")
	query.Add("direction", "asc")
	ms, err := github.LoadMilestones("syncthing/syncthing", query)
	if err != nil {
		log.Fatal(err)
	}

	var filteredMilestones []milestone
	for _, m := range ms {
		if m.Due != nil && includeDue || m.Due == nil && includeNonDue {
			filteredMilestones = append(filteredMilestones, milestone{Milestone: m})
		}
	}

	for i := range filteredMilestones {
		m := &filteredMilestones[i]

		query := make(url.Values)
		query.Add("milestone", fmt.Sprint(m.Number))

		issues, err := github.LoadIssues("syncthing/syncthing", query)
		if err != nil {
			log.Fatal(err)
		}
		for i := range issues {
			issues[i].Body = strings.Replace(issues[i].Body, "\r\n", "\n", -1)
		}
		m.Issues = issues
	}

	sort.Sort(byTitle(filteredMilestones))

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, map[string]interface{}{
		"repo":       repo,
		"milestones": filteredMilestones,
	}); err != nil {
		log.Fatalf("template execution: %s", err)
	}

	return buf.Bytes()
}

type byTitle []milestone

func (l byTitle) Len() int {
	return len(l)
}

func (l byTitle) Less(a, b int) bool {
	return l[a].Title < l[b].Title
}

func (l byTitle) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
