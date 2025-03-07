package main

import (
	"fmt"
	cl "github.com/aep/apogy/client"
	kv "github.com/aep/apogy/kv/cmd"
	"github.com/aep/apogy/mkmtls"
	sr "github.com/aep/apogy/server"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "cli",
	Short: "A CLI application",
}

var mkmtlsCmd = &cobra.Command{
	Use:  "mkmtls [dns_name1] [dns_name2]",
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mkmtls.Main(args)
	},
}

func init() {
	rootCmd.AddCommand(sr.CMD)
	rootCmd.AddCommand(kv.CMD)
	rootCmd.AddCommand(mkmtlsCmd)
	cl.RegisterCommands(rootCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
