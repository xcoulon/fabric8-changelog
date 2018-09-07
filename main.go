package main

import (
	"fmt"
	"os"

	"github.com/fabric8-services/fabric8-changelog/cmd"
)

func main() {

	rootCmd := cmd.NewRootCommand()
	helpCommand := cmd.NewHelpCommand()
	rootCmd.SetHelpCommand(helpCommand)
	// rootCmd.SetHelpTemplate(helpTemplate)
	// rootCmd.PersistentFlags().BoolP("help", "h", false, "Print usage")
	// rootCmd.PersistentFlags().MarkShorthandDeprecated("help", "please use --help")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
