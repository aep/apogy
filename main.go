package main

import (
	cl "apogy/client"
	kv "apogy/kv/cmd"
	sr "apogy/server"
	"fmt"
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
	rootCmd.AddCommand(cl.CMD)

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
