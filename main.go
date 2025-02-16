package main

import (
	"fmt"
	cl "github.com/aep/apogy/client"
	kv "github.com/aep/apogy/kv/cmd"
	sr "github.com/aep/apogy/server"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "cli",
	Short: "A CLI application",
}

func init() {
	rootCmd.AddCommand(sr.CMD)
	rootCmd.AddCommand(kv.CMD)
	cl.RegisterCommands(rootCmd)

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
