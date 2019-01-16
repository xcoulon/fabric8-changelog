package cmd

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/bytesparadise/libasciidoc"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var sinceArg string
var outputLocation string
var outputFormat string

// NewRootCommand initializes the root command
func NewRootCommand() *cobra.Command {

	rootCmd := &cobra.Command{
		Use:   "fabric8-changelog",
		Short: "fabric8-changelog is a CLI tool to retrieve the list of commit diffs between staging and production ",
		RunE:  generateReport,
		Args:  cobra.ExactArgs(1),
	}
	rootCmd.Flags().StringVarP(&sinceArg, "since", "s", "", "the date after which PRs were merged (format: '2006-01-02')")
	rootCmd.Flags().StringVarP(&outputLocation, "output", "o", "", "the output location('-' for stdout)")
	rootCmd.Flags().StringVarP(&outputFormat, "format", "f", "html", "the output format ('asciidoc' or 'html' - default 'html')")

	return rootCmd
}

var renderTmpl template.Template

// var renderPullRequestsTmpl template.Template
// var renderInProgressIssuesTmpl template.Template

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
	repos := strings.Split(args[0], ",")
	sort.Strings(repos)
	since, err := time.Parse("2006-01-02", sinceArg)
	if err != nil {
		return errors.Wrap(err, "invalid value for the 'since' date")
	}

	mergedPRs := listMergedPRs(repos, since)
	inProgressIssues := listIssuesInProgress(repos)

	// output the final result
	// generate
	output, close := getOut(cmd, outputLocation)
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
