package cmd

import (
	"sync"

	"github.com/fabric8-services/fabric8-changelog/client/github"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewCloseMilestoneCmd returns a new command to close a milestone
func NewCloseMilestoneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "close-milestone",
		Short: "Close the milestone",
		RunE:  closeMilestone,
		Args:  cobra.ExactArgs(0),
	}
	c.Flags().StringVarP(&name, "name", "", "", "the milestone to close (ef: 'Sprint 123')")
	return c
}

func closeMilestone(cmd *cobra.Command, args []string) error {
	wg := sync.WaitGroup{}
	for _, repo := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			// first, we need to retrieve the milestone numbers, given their name
			m, err := github.FetchMilestone(repo, name)
			if err != nil {
				log.WithError(err).Errorf("unable to close milestone '%s' in repository '%s'", name, repo)
				return
			}
			if m.State != "open" {
				log.Errorf("milestone '%s' in repository '%s' is already closed.", name, repo)
				return
			}
			// finally, close the old milestone
			err = github.CloseMilestone(&m)
			if err != nil {
				log.WithError(err).Errorf("unable to close milestone '%s'", m.URL)
				return
			}
			log.Infof("closed milestone %s", m.URL)
		}(repo)
	}
	wg.Wait()
	log.Debug("done")
	return nil
}
