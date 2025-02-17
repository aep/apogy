package cmd

import (
	"fmt"
	"github.com/aep/apogy/kv"
	"github.com/spf13/cobra"
	"strings"
)

var CMD = &cobra.Command{
	Use:   "kv",
	Short: "direct low level tikv",
}

func init() {
	CMD.AddCommand(listCmd)
	CMD.AddCommand(getCmd)
	CMD.AddCommand(putCmd)
	CMD.AddCommand(delCmd)
}

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all key-value pairs",
	Run: func(cmd *cobra.Command, args []string) {
		kv, err := kv.NewTikv()
		if err != nil {
			panic(err)
		}
		for kv, err := range kv.Read().Iter(cmd.Context(), []byte{}, []byte{}) {
			if err != nil {
				panic(err)
			}
			fmt.Println(escapeNonPrintable(kv.K))
		}
	},
}

var getCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get value for a key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		kv, err := kv.NewTikv()
		if err != nil {
			panic(err)
		}
		v, err := kv.Read().Get(cmd.Context(), []byte(args[0]))
		if err != nil {
			panic(err)
		}
		fmt.Println(string(v))
	},
}

var putCmd = &cobra.Command{
	Use:   "put [key] [value]",
	Short: "Put a key-value pair",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		kv, err := kv.NewTikv()
		if err != nil {
			panic(err)
		}
		w := kv.Write()
		w.Put([]byte(args[0]), []byte(args[1]))
		err = w.Commit(cmd.Context())
		if err != nil {
			panic(err)
		}
	},
}

var delCmd = &cobra.Command{
	Use:     "del [key]",
	Aliases: []string{"rm"},
	Short:   "Delete a key-value pair",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		kv, err := kv.NewTikv()
		if err != nil {
			panic(err)
		}
		w := kv.Write()
		w.Del([]byte(args[0]))
		err = w.Commit(cmd.Context())
		if err != nil {
			panic(err)
		}
	},
}

func escapeNonPrintable(b []byte) string {
	var result strings.Builder
	for _, c := range b {
		if c >= 32 && c <= 126 {
			result.WriteByte(c)
		} else {
			result.WriteString(fmt.Sprintf("\\x%02x", c))
		}
	}
	return result.String()
}
