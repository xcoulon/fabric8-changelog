package commits

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewListCommand a command to list the commits that are in diff between staging and production
func NewListCommand() *cobra.Command {
	c := &cobra.Command{
		Short: "list commits",
		Use:   "list",
		// Args:  cobra.MinimumNArgs(1),
		Run: listCommits,
	}

	return c
}

func listCommits(cmd *cobra.Command, args []string) {
	// first, retrieve the current state of the pre-production and production environments
	preprodStatus, err := getCommitStatus("https://auth.prod-preview.openshift.io/api/status")
	if err != nil {
		log.Fatalf("failed to get commit status for prod-preview: %v", err)
	}
	prodStatus, err := getCommitStatus("https://auth.openshift.io/api/status")
	if err != nil {
		log.Fatalf("failed to get commit status for prod: %v", err)
	}
	log.Infof("fetching commits in the %s...%s range", prodStatus, preprodStatus)
	// now, fetch all commits between preprod (newer) and prod (older) using GitHub's GraphQL API

	commits, err := getCommits(preprodStatus, prodStatus)
	if err != nil {
		log.Fatalf("failed to get the list of commits: %v", err)
	}
	for _, c := range commits {
		log.Infof("%s (%s)", c.Message, c.Committer)
	}

}

func getCommitStatus(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "unable to get API status for '%s'", url)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	status := APIStatus{}
	json.Unmarshal(body, &status)
	log.Debugf("current commit on %s: %s", url, status.Commit)
	return status.Commit, nil
}

type APIStatus struct {
	BuildTime           string `json:"buildTime"`
	Commit              string `json:"commit"`
	ConfigurationStatus string `json:"configurationStatus"`
	DatabaseStatus      string `json:"databaseStatus"`
	StartTime           string `json:"startTime"`
}

const query = `{
	"query": "query {
		repository(owner:\"fabric8-services\", name:\"fabric8-auth\") {
			defaultBranchRef() {
				target {
					... on Commit {
						history(%[1]s) {
							pageInfo {
								endCursor
							}
							edges {
								node {
									oid
									message
									committedDate
									committer {
										user {
											login
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}"
}`

type Commits struct {
	Data Data `json:"data"`
}

type Data struct {
	Repository Repository `json:"repository"`
}

type Repository struct {
	DefaultBranchRef DefaultBranchRef `json:"defaultBranchRef"`
}

type DefaultBranchRef struct {
	Target Target `json:"target"`
}

type Target struct {
	History History `json:"history"`
}

type History struct {
	PageInfo PageInfo        `json:"pageInfo"`
	Edges    []CommitWrapper `json:"edges"`
}

type PageInfo struct {
	// HasPreviousPage bool   `json:"hasPreviousPage"`
	// HasNextPage     bool   `json:"hasNextPage"`
	EndCursor string `json:"endCursor"`
	// StartCursor     string `json:"startCursor"`
}

type CommitWrapper struct {
	Commit Commit `json:"node"`
}

type Commit struct {
	OID           string    `json:"oid"`
	Message       string    `json:"message"`
	CommittedDate string    `json:"committedDate"`
	Committer     Committer `json:"committer"`
}

type Committer struct {
	User User `json:"user"`
}

type User struct {
	Login string `json:"login"`
}

func getCommits(from, to string) ([]Commit, error) {
	startCursor := ""
	commits := make([]Commit, 0)
	for found := false; !found; {
		subset, endCursor, err := getCommitSubSet(FirstCommits(3), AfterCursor(startCursor))
		if err != nil {
			return commits, err
		}
		for _, c := range subset {
			if c.OID != to {
				commits = append(commits, c)
			} else {
				found = true
				break
			}
		}
		startCursor = endCursor
	}

	return commits, nil
}

type FetchCommitsOption func(string) string

func FirstCommits(first int) FetchCommitsOption {
	return func(args string) string {
		return fmt.Sprintf("%[1]s first:%[2]d,", args, first)
	}
}

func AfterCursor(after string) FetchCommitsOption {
	return func(args string) string {
		if after == "" {
			return args
		}
		return fmt.Sprintf(`%[1]s after:\"%[2]s\",`, args, after)
	}
}

func getCommitSubSet(options ...FetchCommitsOption) ([]Commit, string, error) {
	result := make([]Commit, 0)
	queryOption := ""
	for _, applyOpt := range options {
		queryOption = applyOpt(queryOption)
	}
	q := fmt.Sprintf(query, queryOption)
	q = fmt.Sprintf("%s", strings.Replace(strings.Replace(q, "\n", " ", -1), "\t", "", -1))
	log.Infof("request body: %s", q)
	req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewReader([]byte(q)))
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get commits")
	}
	req.Header.Add("Authorization", "Bearer "+os.Getenv("GITHUB_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get commits")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get commits")
	}
	if resp.StatusCode != 200 {
		return result, "", errors.Errorf("failed to execute query: %s", string(body))
	}
	var commits Commits
	log.Infof("raw response: %s", string(body))
	err = json.Unmarshal(body, &commits)
	if err != nil {
		return result, "", errors.Wrapf(err, "unable to get commits")
	}
	log.Infof("response: %s", spew.Sdump(commits))
	endCursor := commits.Data.Repository.DefaultBranchRef.Target.History.PageInfo.EndCursor
	for _, w := range commits.Data.Repository.DefaultBranchRef.Target.History.Edges {
		result = append(result, w.Commit)
	}
	return result, endCursor, nil
}
