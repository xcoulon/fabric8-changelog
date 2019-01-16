package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"text/template"

	"github.com/davecgh/go-spew/spew"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var fetchMilestoneIssuesTmpl template.Template

func init() {
	fetchMilestoneIssuesTmpl = newTextTemplate("fetch repo id", `{
		"query": "query {
			repository(owner:\"{{ .Org }}\", name:\"{{ .Name }}\") {
				databaseId
				milestones(states:OPEN, first:1, orderBy:{
					field:DUE_DATE
					direction: ASC
				}) {
					nodes {
						title
						issues(states:OPEN, first:2, orderBy:
						{
							field: UPDATED_AT
							direction: DESC
							
						}{{ if .After }}
						after:\"{{ .After}}\"
						{{ end }}) {
							pageInfo {
								endCursor
								hasNextPage
							}
							nodes {
								number
								title
								url
							}
						}
					}
				}
			}
		}"
	}`)

}

func listIssuesInProgress(repos []string) map[string]map[int64]MilestoneIssue {
	wg := sync.WaitGroup{}
	result := make(map[string]map[int64]MilestoneIssue)
	// allPRs := make([]interface{}, len(repos))
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
			// first, retrieve the repository ID on GitHub
			repoID, issues, err := fetchMilestoneIssues(remote[0], remote[1])
			if err != nil {
				log.Errorf("unable to list work-in-progress issues for repo '%s': %v", repo, err)
				return
			}
			log.Debugf("repo '%s': %d", repo, repoID)
			log.Debugf("repo issues: %s", spew.Sdump(issues))
			// then fetch events for each issue on ZenHub
			err = filterInProgressIssues(repoID, issues)
			if err != nil {
				log.Errorf("unable to list work-in-progress issues for repo '%s': %v", repo, err)
				return
			}
			log.Debugf("WIP issues: %s", spew.Sdump(issues))
			if len(issues) > 0 {
				result[repo] = issues
			}

		}(i, repo)
	}
	wg.Wait()

	return result
}

func fetchMilestoneIssues(org, name string) (int64, map[int64]MilestoneIssue, error) {
	issues := map[int64]MilestoneIssue{}
	var after string
	var databaseID int64
	for {
		queryBuf := bytes.NewBuffer(nil)
		err := fetchMilestoneIssuesTmpl.Execute(queryBuf, struct {
			Org   string
			Name  string
			After string
		}{
			Org:   org,
			Name:  name,
			After: after,
		})
		// log.Debugf("repository ID graphql query: %s", queryBuf.String())
		if err != nil {
			return -1, issues, errors.Wrapf(err, "unable to get milestone issues")
		}

		var response MilestoneIssuesResponse
		err = queryGitHub(queryBuf.String(), &response)
		if err != nil {
			return -1, issues, errors.Wrapf(err, "unable to get milestone issues")
		}
		if log.GetLevel() == log.DebugLevel {
			r, _ := json.Marshal(response)
			log.Debugf("unmarshalled response: %s", string(r))
		}
		if len(response.Data.Repository.Milestones.Nodes) == 0 {
			return -1, issues, errors.Wrapf(err, "unable to get current milestone")
		}
		databaseID = response.Data.Repository.DatabaseID
		milestone := response.Data.Repository.Milestones.Nodes[0]
		for _, issue := range milestone.Issues.Nodes {
			issues[issue.Number] = issue
		}
		if !milestone.Issues.PageInfo.HasNextPage {
			break
		}
		after = milestone.Issues.PageInfo.EndCursor
	}
	return databaseID, issues, nil
}

// example response:
// {
// 	"data": {
// 	  "repository": {
// 		"databaseId": 144640567,
// 		"milestones": {
// 		  "nodes": [
// 			{
// 			  "title": "Sprint 160",
// 			  "issues": {
// 				"pageInfo": {
// 				  "endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0wMS0xNFQxMjoyMjoyMCswMTowMM4XsXVm",
// 				  "hasNextPage": false
// 				},
// 				"nodes": [
// 					{
// 						"number": 59,
// 						"title": "Endpoint to obtain cluster info by API URL"
//						"url": "https://github.com/fabric8-services/fabric8-cluster/issues/59"
//   				}
//				]
// 			  }
// 			}
// 		  ]
// 		}
// 	  }
// 	}
//   }

// MilestoneIssuesResponse the response wrapper for the milestone issues
type MilestoneIssuesResponse struct {
	Data struct {
		Repository struct {
			DatabaseID int64 `json:"databaseID"`
			Milestones struct {
				Nodes []struct {
					Title  string `json:"title"`
					Issues struct {
						PageInfo struct {
							EndCursor   string `json:"endCursor"`
							HasNextPage bool   `json:"hasNextPage"`
						} `json:"pageInfo"`
						Nodes []MilestoneIssue `json:"nodes"`
					} `json:"issues"`
				} `json:"nodes"`
			} `json:"milestones"`
		} `json:"repository"`
	} `json:"data"`
}

// MilestoneIssue the milestone issue
type MilestoneIssue struct {
	Number int64  `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

const (
	// InProgress the "In Progress" pipeline
	InProgress string = "In Progress"
	// ReviewQA the "Review/QA" pipeline
	ReviewQA string = "Review/QA"
)

func filterInProgressIssues(repoID int64, issues map[int64]MilestoneIssue) error {
	// for each issue, check the most recent event on ZenHub to see if the it is in the `In Progress` pipeline
	// https://api.zenhub.io/p1/repositories/144640567/issues/59/events
	events := []IssueEvent{}
	for number := range issues {
		err := queryEvents(repoID, number, &events)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			// issue is untriagged
			delete(issues, number)
			continue
		}
		if events[0].ToPipeline.Name != InProgress && events[0].ToPipeline.Name != ReviewQA {
			delete(issues, number)
		}
	}
	return nil
}

// IssueEvent a single event for an issue on ZenHub
type IssueEvent struct {
	ToPipeline struct {
		Name string `json:"name"`
	} `json:"to_pipeline"`
}
