package main

import (
	"fmt"
	"os"

	"github.com/fabric8-services/fabric8-changelog/cmd"
)

func main() {
	rootCmd := cmd.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
