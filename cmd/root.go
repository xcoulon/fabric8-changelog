package cmd

import (
	"github.com/fabric8-services/fabric8-changelog/cmd/commits"
	"github.com/spf13/cobra"
)

// NewRootCommand initializes the root command
func NewRootCommand() *cobra.Command {

	rootCmd := &cobra.Command{
		Use:   "fabric8-changelog",
		Short: "fabric8-changelog is a CLI tool to retrieve the list of commit diffs between staging and production ",
		Run: func(cmd *cobra.Command, args []string) {
			// Do Stuff Here
		},
	}
	listCommitsCmd := commits.NewListCommand()
	rootCmd.AddCommand(listCommitsCmd)
	return rootCmd
}
