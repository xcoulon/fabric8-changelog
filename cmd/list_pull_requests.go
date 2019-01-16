package cmd

import (
	"bytes"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var fetchPullRequestsTmpl template.Template

func init() {

	fetchPullRequestsTmpl = newTextTemplate("fetch PRs",
		`{
		"query": "query  {
			repository(owner:\"{{ .Owner }}\", name:\"{{ .Name }}\") {
				pullRequests(last:{{ .Last }}, states:[{{ .State }}], orderBy:{field:UPDATED_AT, direction:ASC}{{ if .Before }}, before:\"{{ .Before}}\"{{ end }}) {
					pageInfo {
						endCursor
					}
					nodes {
						number
						title
						mergedAt
						permalink
						
					}
				}
			}
		}"
	}`)
}

func listMergedPRs(repos []string, since time.Time) map[string]map[int64]PullRequest {
	wg := sync.WaitGroup{}
	result := make(map[string]map[int64]PullRequest)
	for i, repo := range repos {
		wg.Add(1)
		// process in a go routine to parallelize the I/O tasks
		go func(idx int, repo string) {
			defer wg.Done()
			remote := strings.Split(repo, "/")
			if len(remote) != 2 {
				log.Errorf("'%s' is not a valid GH repository (fornat: '<owner>/<name>')", repo)
				return
			}
			// query the repo until no more data is needed
			pulls, err := fetchPullRequests(remote[0], remote[1], "MERGED", since)
			if err != nil {
				log.Errorf("failed to fetch merged pull requests for %s: %v", repo, err)
				return
			}
			if len(pulls) > 0 {
				result[repo] = pulls
			}
		}(i, repo)
	}
	wg.Wait()
	return result
}

const (
	ghDateFormat = "2006-01-02T15:04:05Z"
)

func fetchPullRequests(owner, name, state string, since time.Time) (map[int64]PullRequest, error) {
	before := ""
	pulls := map[int64]PullRequest{}
	for {
		found := false
		// subset, endCursor, err := listPullRequests(owner, name, state, before, 10)
		queryBuf := bytes.NewBuffer(nil)
		err := fetchPullRequestsTmpl.Execute(queryBuf, struct {
			Owner  string
			Name   string
			State  string
			Before string
			Last   int
		}{
			Owner:  owner,
			Name:   name,
			State:  state,
			Before: before,
			Last:   10,
		})
		var response PullRequestsResponse
		err = queryGitHub(queryBuf.String(), &response)
		if err != nil {
			return pulls, errors.Wrapf(err, "unable to get list of merged pull requests")
		}

		subset := []PullRequest{}
		endCursor := response.Data.Repository.PullRequests.PageInfo.EndCursor
		for _, pr := range response.Data.Repository.PullRequests.Nodes {
			subset = append(subset, pr)
		}

		// need to iterate on the data in reverse order
		for i := len(subset) - 1; i >= 0; i-- {
			pr := subset[i]
			mergedAt, err := time.Parse(ghDateFormat, pr.MergedAt)
			if err != nil {
				return pulls, errors.Wrapf(err, "failed to parse 'mergedAt' date '%s'", pr.MergedAt)
			}
			log.Debugf("processing %s merged at %s (valid: %t)", pr.Title, mergedAt, mergedAt.After(since))
			// ignore last result when using the cursor, because the resultset contains the last item of the previous "page"
			if _, found := pulls[pr.Number]; !found && mergedAt.After(since) {
				pulls[pr.Number] = pr
				found = true
			}
		}
		// if none of the PR matched, then assume it's all done
		if !found {
			return pulls, nil
		}
		before = endCursor
	}
}

// Example response:
// {
// 	"data": {
// 	  "repository": {
// 		"pullRequests": {
// 		  "pageInfo": {
// 			"endCursor": "Y3Vyc29yOnYyOpHODZiInQ=="
// 		  },
// 		  "nodes": [
// 			{
// 			  "number": 709,
// 			  "title": "Upgrade to go v11.1 for test-coverage CI",
// 			  "mergedAt": "2018-11-01T07:30:04Z",
// 			  "permalink": "https://github.com/fabric8-services/fabric8-auth/pull/709"
// 			},
// 			{
// 			  "number": 710,
// 			  "title": "Move back to centos go and disable gofmt check in coverage job",
// 			  "mergedAt": "2018-11-05T02:44:39Z",
// 			  "permalink": "https://github.com/fabric8-services/fabric8-auth/pull/710"
// 			}
// 		  ]
// 		}
// 	  }
// 	}
// }

type PullRequestsResponse struct {
	Data PullRequestsData `json:"data"`
}

type PullRequestsData struct {
	Repository Repository `json:"repository"`
}

type Repository struct {
	PullRequests PullRequests `json:"pullRequests"`
}

type PullRequests struct {
	PageInfo PageInfo      `json:"pageInfo"`
	Nodes    []PullRequest `json:"nodes"`
}

type PageInfo struct {
	EndCursor string `json:"endCursor"`
}

type PullRequest struct {
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	MergedAt  string `json:"mergedAt"`
	Permalink string `json:"permalink"`
}
