package cmd

import (
	"sort"
	"sync"
	"time"

	"github.com/fabric8-services/fabric8-changelog/client/github"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var name string
var endDate string

// NewCreateMilestoneCmd returns a new command to generate a milestone on a list of repositories
func NewCreateMilestoneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "new-milestone",
		Short: "Creates a new milestone",
		RunE:  generateMilestone,
		Args:  cobra.ExactArgs(0),
	}
	c.Flags().StringVarP(&name, "name", "n", "", "the name of the milestone to create (ef: 'Sprint 123')")
	c.Flags().StringVarP(&endDate, "end", "e", "", "the end date for the sprint (format: '2006-01-02')")
	return c
}

func generateMilestone(cmd *cobra.Command, args []string) error {
	sort.Strings(repos)
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return errors.Wrap(err, "invalid value for the 'end' date")
	}
	log.Debugf("creating milestone '%s' with end date '%s' on %v...", name, end.String(), repos)

	wg := sync.WaitGroup{}
	for i, repo := range repos {
		wg.Add(1)
		// process in a go routine to parallelize the I/O tasks
		go func(idx int, repo string) {
			defer wg.Done()
			log.Debugf("creating milestone '%s' for repo '%s'...", name, repo)
			m, err := github.CreateMilestone(repo, name, end)
			if err != nil {
				log.WithError(err).Errorf("failed to create milestone '%s' for repo '%s'", name, repo)
				return
			}
			log.Infof("created milestone for repo: '%s'", m.URL)
		}(i, repo)
	}
	wg.Wait()
	return nil
}
