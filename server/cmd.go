package server

import (
	"github.com/spf13/cobra"
)

var CMD = &cobra.Command{
	Use:   "server",
	Short: "start a grpc server",
	Run: func(cmd *cobra.Command, args []string) {
		Main()
	},
}
