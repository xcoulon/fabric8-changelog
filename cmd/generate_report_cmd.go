package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/fabric8-services/fabric8-changelog/client/zenhub"

	"github.com/bytesparadise/libasciidoc"
	"github.com/fabric8-services/fabric8-changelog/client/github"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var since string
var outputDir string
var outputFormat string

// NewGenerateReportCommand generates a new report
func NewGenerateReportCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "report",
		Short: "Generate a report based with all merged pull-requests and all issues in progress or review/QA",
		RunE:  generateReport,
		Args:  cobra.ExactArgs(0),
	}
	c.Flags().StringSliceVarP(&repos, "repositories", "r", defaultRepos, "the repositories on which the milestone will be created")
	c.Flags().StringVarP(&since, "since", "s", "", "the date after which PRs were merged (format: '2006-01-02')")
	c.Flags().StringVarP(&outputDir, "output", "o", "tmp", "the output directory, or '-' for stdout")
	c.Flags().StringVarP(&outputFormat, "format", "f", "html", "the output format ('asciidoc' or 'html' - default 'html')")

	return c
}

var renderTmpl template.Template

func init() {

	renderTmpl = newTextTemplate("report",
		`Done since last week:

{{ range $name, $prs := .MergedPRs }}* {{ $name }}:
{{ range $idx, $pr := $prs }}{{ with $pr }}** [{{ .Permalink }}[{{ .Number}}]] {{ .Title }}{{ end }}
{{ end }}
{{ end }}

Currently working on:

{{ range $name, $issues := .InProgressIssues }}* {{ $name }}:
{{ range $idx, $issue := $issues }}{{ with $issue }}** [{{ .URL }}[{{ .Number}}]] {{ .Title }}{{ end }}
{{ end }}
{{ end }}
`)
}

func generateReport(cmd *cobra.Command, args []string) error {
	sort.Strings(repos)
	s, err := time.Parse("2006-01-02", since)
	if err != nil {
		return errors.Wrap(err, "invalid value for the 'since' date")
	}

	mergedPRs := listMergedPRs(repos, s)
	inProgressIssues := listIssuesInProgress(repos)

	// output the final result
	// generate
	output, close, err := getOut(cmd, outputDir, time.Now().Format("2006-01-02"), outputFormat)
	if err != nil {
		return errors.Wrap(err, "failed to render report")
	}

	defer close()
	data := struct {
		MergedPRs        map[string]map[int64]PullRequest
		InProgressIssues map[string]map[int64]MilestoneIssue
	}{
		MergedPRs:        mergedPRs,
		InProgressIssues: inProgressIssues,
	}
	if outputFormat == "html" {
		tmpOut := bytes.NewBuffer(nil)
		err = renderTmpl.Execute(tmpOut, data)
		if err != nil {
			return errors.Wrap(err, "failed to render report")
		}
		_, err = libasciidoc.ConvertToHTML(context.Background(), tmpOut, output)
		if err != nil {
			return errors.Wrap(err, "failed to render merged pull requests")
		}
	} else {
		err = renderTmpl.Execute(output, data)
		if err != nil {
			return errors.Wrap(err, "failed to render report")
		}
	}

	return nil
}

type closeFunc func() error

func defaultCloseFunc() closeFunc {
	return func() error { return nil }
}

func newCloseFileFunc(c io.Closer) closeFunc {
	return func() error {
		return c.Close()
	}
}

func getOut(cmd *cobra.Command, outputDir, outputDate, outputFormat string) (io.Writer, closeFunc, error) {
	if outputDir == "-" {
		// outfile is STDOUT
		return cmd.OutOrStdout(), defaultCloseFunc(), nil
	}
	// check of the output dir needs to be created
	if _, err := os.Stat(outputDir); err != nil {
		err = os.Mkdir(outputDir, os.ModeDir+os.ModePerm)
		if err != nil {
			return nil, nil, err
		}
	}
	// outfile is specified in the command line
	outfile, err := os.Create(fmt.Sprintf("%s/changelog-%s.%s", outputDir, outputDate, outputFormat))
	if err != nil {
		return nil, nil, err
	}
	return outfile, newCloseFileFunc(outfile), nil
}

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
		err = github.ExecuteGraphqlQuery(queryBuf.String(), &response)
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

// PullRequestsResponse the response to the GraphQL query to list merged pull requests
type PullRequestsResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				PageInfo struct {
					EndCursor string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []PullRequest `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

// PullRequest the merged pull request
type PullRequest struct {
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	MergedAt  string `json:"mergedAt"`
	Permalink string `json:"permalink"`
}

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
		err = github.ExecuteGraphqlQuery(queryBuf.String(), &response)
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
		err := zenhub.QueryIssueEvents(repoID, number, &events)
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
