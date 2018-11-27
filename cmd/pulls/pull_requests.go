package pulls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var tmpl template.Template

func init() {
	tmpl = newTextTemplate("list PRs",
		`{
			"query": "query  {
				repository(owner:\"{{ .Owner }}\", name:\"{{ .Name }}\") {
					pullRequests(last:{{ .Last }}, states:[{{ .State }}]{{ if .Before }}, before:\"{{ .Before}}\"{{ end }}) {
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

func newTextTemplate(name, src string, funcs ...template.FuncMap) template.Template {
	t := template.New(name)
	for _, f := range funcs {
		t.Funcs(f)
	}
	t, err := t.Parse(src)
	if err != nil {
		log.Fatalf("failed to initialize '%s' template: %s", name, err.Error())
	}
	return *t
}

var sinceArg string

const (
	ghDateFormat = "2006-01-02T15:04:05Z"
)

// NewListMergedPRsCommand a command to list the pull requests that were merged since the given date on the given org/repo
func NewListMergedPRsCommand() *cobra.Command {
	c := &cobra.Command{
		Short: "list merged pull requests after a date",
		Use:   "list",
		Run:   listMergedPRs,
		Args:  cobra.ExactArgs(1),
	}
	c.Flags().StringVarP(&sinceArg, "since", "s", "", "the date after which PRs were merged (format: '2006-01-02')")
	return c
}

func listMergedPRs(cmd *cobra.Command, args []string) {
	repos := strings.Split(args[0], ",")
	since, err := time.Parse("2006-01-02", sinceArg)
	if err != nil {
		log.Errorf("invalid value for the 'since' date: %v", err)
		return
	}
	for _, repo := range repos {
		remote := strings.Split(repo, "/")
		if len(remote) != 2 {
			log.Errorf("'%s' is not a valid GH repository (fornat: '<owner>/<name>')", repo)
			continue
		}
		// query the repo until no more data is needed
		result, err := getPullRequests(remote[0], remote[1], "MERGED", since)
		if err != nil {
			log.Errorf("failed to fetch merged pull requests for %s: %v", repo, err)
		} else {
			spew.Dump(result)
		}

	}
}

func getPullRequests(owner, name, state string, since time.Time) ([]PullRequest, error) {
	before := ""
	pulls := []PullRequest{}
	for {
		subset, endCursor, err := getData(owner, name, state, before, 2)
		if err != nil {
			return pulls, err
		}
		for i, pr := range subset {
			mergedAt, err := time.Parse(ghDateFormat, pr.MergedAt)
			if err != nil {
				return pulls, errors.Wrapf(err, "failed to parse 'mergedAt' date '%s'", pr.MergedAt)
			}
			if !mergedAt.After(since) {
				// exit function if this PR is older than the 'since' limit
				return pulls, nil
			}
			// ignore first result when using the cursor, because the resultset contains the last item of the previous "page"
			if before == "" || i > 0 {
				pulls = append(pulls, pr)
			}
		}
		before = endCursor

	}

	return pulls, nil
}

func getData(owner, name, state, before string, last int) ([]PullRequest, string, error) {
	result := []PullRequest{}
	queryBuf := bytes.NewBuffer(nil)
	err := tmpl.Execute(queryBuf, struct {
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
		Last:   last,
	})
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get data")
	}
	log.Infof("request body: %s", queryBuf.String())
	query := strings.Replace(strings.Replace(queryBuf.String(), "\n", " ", -1), "\t", "", -1)
	req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewReader([]byte(query)))
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get data")
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get data")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get data")
	}
	if resp.StatusCode != 200 {
		return result, "", errors.Errorf("failed to execute query: %s", string(body))
	}
	var response PullRequestsResponse
	log.Debugf("raw response: %s", string(body))
	err = json.Unmarshal(body, &response)
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get data")
	}
	log.Debugf("response: %s", spew.Sdump(response))
	endCursor := response.Data.Repository.PullRequests.PageInfo.EndCursor
	for _, pr := range response.Data.Repository.PullRequests.Nodes {
		result = append(result, pr)
	}
	return result, endCursor, nil
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
	Number    int    `json:"number"`
	Title     string `json:"title"`
	MergedAt  string `json:"mergedAt"`
	Permalink string `json:"permalink"`
}
