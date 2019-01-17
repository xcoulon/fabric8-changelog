package cmd

import (
	"sync"

	"github.com/fabric8-services/fabric8-changelog/client/github"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewMoveIssuesToMilestoneCmd returns a new command to move issues from a milestone to another one
func NewMoveIssuesToMilestoneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "move-issues",
		Short: "Move all open issues to a new milestones for all the given repositories",
		RunE:  moveIssues,
		Args:  cobra.ExactArgs(0),
	}
	c.Flags().StringVarP(&from, "from", "", "", "the milestone to move the issues from (ef: 'Sprint 123')")
	c.Flags().StringVarP(&to, "to", "", "", "the milestone to move the issues to (ef: 'Sprint 124')")
	return c
}

var from, to string

func moveIssues(cmd *cobra.Command, args []string) error {
	wg := sync.WaitGroup{}
	for _, repo := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			// first, we need to retrieve the milestone numbers, given their name
			fromMilestone, err := github.FetchMilestone(repo, from)
			if err != nil {
				log.WithError(err).Errorf("unable to move issues in repository '%s'", repo)
				return
			}
			// next, list all open issues in the "from" milestone
			issues, err := github.FetchMilestoneIssues(repo, fromMilestone.Number)
			if err != nil {
				log.WithError(err).Errorf("unable to move issues in repository '%s'", repo)
				return
			}
			toMilestone, err := github.FetchMilestone(repo, to)
			for _, issue := range issues {
				err := github.MoveIssue(&issue, toMilestone)
				if err != nil {
					log.WithError(err).Errorf("unable to move issues in repository '%s'", repo)
					return
				}
				log.Infof("moved issue %s to milestone %s", issue.URL, toMilestone.URL)
			}
		}(repo)
	}
	wg.Wait()
	log.Debug("done")
	return nil
}
