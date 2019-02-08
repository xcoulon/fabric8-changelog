package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewRootCommand initializes the root command
func NewRootCommand() *cobra.Command {
	c := &cobra.Command{
		Use:              "fabric8-changelog",
		Short:            "fabric8-changelog is a CLI tool to manage issues on GitHub and ZenHub",
		PersistentPreRun: setLoggerLevel,
		Args:             cobra.ExactArgs(1),
	}
	c.PersistentFlags().StringSliceVarP(&repos, "repositories", "r", defaultRepos, "the repositories on which the command applies")
	c.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "prints the debug statements")
	c.AddCommand(NewGenerateReportCommand())
	c.AddCommand(NewCreateMilestoneCmd())
	c.AddCommand(NewMoveIssuesToMilestoneCmd())
	c.AddCommand(NewCloseMilestoneCmd())
	return c
}

// -----------------------------------------
// repositories on which the command applies
// -----------------------------------------
var repos []string
var defaultRepos []string

func init() {
	defaultRepos = []string{
		"fabric8-services/admin-console",
		"fabric8-services/fabric8-auth",
		"fabric8-services/fabric8-auth-client",
		"fabric8-services/fabric8-common",
		"fabric8-services/fabric8-cluster",
		"fabric8-services/fabric8-cluster-client",
		"fabric8-services/fabric8-devdoc",
		"fabric8-services/fabric8-env",
		"fabric8-services/fabric8-env-client",
		"fabric8-services/fabric8-notification",
		"fabric8-services/fabric8-oso-proxy",
		"fabric8-services/fabric8-tenant",
		"fabric8-services/toolchain-operator",
	}
}

// -----------------------------------------
// debugging
// -----------------------------------------

// a flag to print the debug statements
var debug bool

func setLoggerLevel(cmd *cobra.Command, args []string) {
	if debug {
		logrus.Warn("setting logger level to 'debug'")
		logrus.SetLevel(logrus.DebugLevel)
	}
}
