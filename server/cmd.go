package server

import (
	"github.com/spf13/cobra"
)

var (
	caCertPath     string
	serverCertPath string
	serverKeyPath  string
)

var CMD = &cobra.Command{
	Use:   "server",
	Short: "start a grpc server",
	Run: func(cmd *cobra.Command, args []string) {
		Main(caCertPath, serverCertPath, serverKeyPath)
	},
}

func init() {
	CMD.Flags().StringVar(&caCertPath, "ca-cert", "", "Path to CA certificate file for client verification (enables mTLS)")
	CMD.Flags().StringVar(&serverCertPath, "server-cert", "", "Path to server certificate file")
	CMD.Flags().StringVar(&serverKeyPath, "server-key", "", "Path to server private key file")
}
