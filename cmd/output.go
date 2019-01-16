package cmd

import (
	"io"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type closeFunc func() error

func defaultCloseFunc() closeFunc {
	return func() error { return nil }
}

func newCloseFileFunc(c io.Closer) closeFunc {
	return func() error {
		return c.Close()
	}
}

func getOut(cmd *cobra.Command, outputName string) (io.Writer, closeFunc) {
	if outputName == "-" {
		// outfile is STDOUT
		return cmd.OutOrStdout(), defaultCloseFunc()
	}
	// outfile is specified in the command line
	outfile, e := os.Create(outputName)
	if e != nil {
		log.Warnf("Cannot create output file - %v", outputName)
	}
	return outfile, newCloseFileFunc(outfile)
}
