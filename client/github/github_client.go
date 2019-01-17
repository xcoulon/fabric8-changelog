package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Milestone data for a milestone
type Milestone struct {
	Number int64  `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

// Issue data for an issue
type Issue struct {
	Number    int64     `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	Milestone Milestone `json:"milestone"`
}

// ExecuteGraphqlQuery executes the given GraphQL query on the GitHub API endpoint
func ExecuteGraphqlQuery(query string, result interface{}) error {
	query = strings.Replace(strings.Replace(query, "\n", " ", -1), "\t", "", -1)
	return execute("POST", "https://api.github.com/graphql", bytes.NewReader([]byte(query)), result)
}

// CreateMilestone creates a new milestone (using the Rest v3 API)
func CreateMilestone(repo, name string, endDate time.Time) (Milestone, error) {
	// curl -X POST https://api.github.com/repos/fabric8-services/fabric8-tenant/milestones
	// -H "Authorization: Bearer $GITHUB_TOKEN"
	// -d '{
	// 	"title": "Sprint 161",
	// 	"state": "open",
	// 	"due_on": "2019-02-05T00:00:00Z"
	//   }'

	result := Milestone{}
	url := fmt.Sprintf("https://api.github.com/repos/%s/milestones", repo)
	payload := fmt.Sprintf(`{
		"title": "%s",
		"state": "open",
		"due_on": "%s"
	}`, name, endDate.Format("2006-01-02T00:00:00Z"))
	err := execute("POST", url, bytes.NewReader([]byte(payload)), &result)
	return result, err
}

// CloseMilestone closes the given milestone
func CloseMilestone(milestone *Milestone) error {
	// see https://developer.github.com/v3/issues/milestones/#update-a-milestone
	// PATCH /repos/:owner/:repo/milestones/:number
	// state: closed
	payload := `{"state":"closed"}`
	return execute("PATCH", milestone.URL, bytes.NewReader([]byte(payload)), milestone)
}

// FetchMilestone fetches the open milestone with the given title
func FetchMilestone(repo, title string) (Milestone, error) {
	milestones, err := ListMilestones(repo)
	if err != nil {
		return Milestone{}, errors.Wrapf(err, "failed to retrieve milestones for repository '%s'", repo)
	}
	for _, m := range milestones {
		if m.Title == title {
			return m, nil
		}
	}
	return Milestone{}, errors.Errorf("unable to find milestone with title '%s' in repository '%s'", title, repo)
}

// FetchMilestoneIssues fetches all open issues for the milestone given its number, on the given repository
func FetchMilestoneIssues(repo string, number int64) ([]Issue, error) {
	// see https://developer.github.com/v3/issues/#list-issues-for-a-repository to retrieve all open issues for the given milestone (using its name)
	// e.g.: curl https://api.github.com/repos/fabric8-services/fabric8-cluster/issues?milestone=3
	result := []Issue{}
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues?state=open&milestone=%d", repo, number)
	err := execute("GET", url, nil, &result)
	return result, err
}

// MoveIssue moves the given issue to the given milestone
func MoveIssue(issue *Issue, milestone Milestone) error {
	// see https://developer.github.com/v3/issues/#edit-an-issue to change the milestone
	// PATCH /repos/:owner/:repo/issues/:number
	// milestone: integer
	payload := fmt.Sprintf(`{"milestone":%d}`, milestone.Number)
	return execute("PATCH", issue.URL, bytes.NewReader([]byte(payload)), issue)
}

// ListMilestones lists *all* milestones for the given repo (using the Rest v3 API)
func ListMilestones(repo string) ([]Milestone, error) {
	// see https://developer.github.com/v3/issues/milestones/#list-milestones-for-a-repository
	// e.g.: curl https://api.github.com/repos/fabric8-services/fabric8-cluster/milestones
	result := []Milestone{}
	url := fmt.Sprintf("https://api.github.com/repos/%s/milestones?state=all&direction=desc", repo)
	err := execute("GET", url, nil, &result)
	return result, err
}

func execute(method, url string, payload io.Reader, result interface{}) error {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return errors.Wrapf(err, "unable to execute HTTP request")
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "unable to execute HTTP request")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "unable to execute HTTP request")
	}
	if resp.StatusCode >= 300 {
		return errors.Errorf("failed to execute query: %d, %s", resp.StatusCode, string(body))
	}
	log.Debugf("raw response: %s", string(body))
	return json.Unmarshal(body, result)
}
