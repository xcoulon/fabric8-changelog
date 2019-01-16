package main

import (
	"fmt"
	"os"

	"github.com/fabric8-services/fabric8-changelog/cmd"
)

func main() {
	// logrus.SetLevel(logrus.DebugLevel)
	rootCmd := cmd.NewRootCommand()
	helpCommand := cmd.NewHelpCommand()
	rootCmd.SetHelpCommand(helpCommand)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
