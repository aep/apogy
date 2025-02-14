package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"apogy/api/go"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var (
	file    string
	address = "http://localhost:5052"

	putCmd = &cobra.Command{
		Use:     "put",
		Aliases: []string{"apply"},
		Short:   "Put a document from file",
		Run:     put,
	}

	getCmd = &cobra.Command{
		Use:   "get [model/id]",
		Short: "Get a document",
		Args:  cobra.ExactArgs(1),
		Run:   get,
	}

	editCmd = &cobra.Command{
		Use:   "edit [model/id]",
		Short: "Edit a document",
		Args:  cobra.ExactArgs(1),
		Run:   edit,
	}

	searchCmd = &cobra.Command{
		Use:     "search [model] [q]",
		Aliases: []string{"find"},
		Short:   "Search for documents",
		Args:    cobra.MinimumNArgs(2),
		Run:     search,
	}
)

func RegisterCommands(root *cobra.Command) {
	putCmd.Flags().StringVarP(&file, "file", "f", "", "Path to JSON/YAML file")
	putCmd.MarkFlagRequired("file")

	root.AddCommand(putCmd)
	root.AddCommand(getCmd)
	root.AddCommand(editCmd)
	root.AddCommand(searchCmd)
}

func parseFile(file string) ([]openapi.Document, error) {
	var data []byte
	var err error

	if file == "-" {
		data, err = ioutil.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	docs := strings.Split(string(data), "---\n")
	var objects []openapi.Document

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var obj openapi.Document
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			return nil, fmt.Errorf("failed to parse document: %v", err)
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func getClient() (*openapi.ClientWithResponses, error) {
	client, err := openapi.NewClientWithResponses(address)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}
	return client, nil
}

func put(cmd *cobra.Command, args []string) {
	objects, err := parseFile(file)
	if err != nil {
		log.Fatal(err)
	}

	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	for _, obj := range objects {
		resp, err := client.PutDocumentWithResponse(context.Background(), obj)
		if err != nil {
			log.Fatalf("Failed to put document: %v", err)
		}

		if resp.JSON200 == nil {
			if resp.JSON400 != nil {
				log.Fatalf("rejected: %s", *resp.JSON400.Message)
			} else {
				log.Fatalf("Unexpected response: %v", resp.StatusCode())
			}
		}

		fmt.Println(resp.JSON200.Path)
	}
}

func get(cmd *cobra.Command, args []string) {
	parts := strings.Split(args[0], "/")
	if len(parts) != 2 {
		log.Fatal("Invalid id format. Expected model/id")
	}
	model, id := parts[0], parts[1]

	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.GetDocumentWithResponse(context.Background(), model, id)
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	if resp.JSON200 == nil {
		log.Fatalf("Unexpected response: %v", resp.StatusCode())
	}

	enc, err := yaml.Marshal(resp.JSON200)
	if err != nil {
		log.Fatalf("Failed to encode as YAML: %v", err)
	}
	os.Stdout.Write(enc)
}

func parseFilter(arg string) openapi.Filter {
	filter := openapi.Filter{}

	if a, b, ok := strings.Cut(arg, "="); ok {
		bi := any(b)
		filter.Key = a
		filter.Equal = &bi
	} else if a, b, ok := strings.Cut(arg, ">"); ok {
		bi := any(b)
		filter.Key = a
		filter.Greater = &bi
	} else if a, b, ok := strings.Cut(arg, "<"); ok {
		bi := any(b)
		filter.Key = a
		filter.Less = &bi
	} else {
		filter.Key = arg
	}

	return filter
}

func search(cmd *cobra.Command, args []string) {
	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	var filters []openapi.Filter
	for _, arg := range args[1:] {
		filters = append(filters, parseFilter(arg))
	}

	req := openapi.SearchRequest{
		Model:   args[0],
		Filters: &filters,
	}

	resp, err := client.SearchDocumentsWithResponse(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to search documents: %v", err)
	}

	if resp.JSON200 == nil {
		log.Fatalf("Unexpected response: %v", resp.StatusCode())
	}

	if resp.JSON200.Ids != nil {
		for _, id := range *resp.JSON200.Ids {
			fmt.Printf("%s/%s\n", args[0], id)
		}
	}
}

func edit(cmd *cobra.Command, args []string) {
	parts := strings.Split(args[0], "/")
	if len(parts) != 2 {
		log.Fatal("Invalid id format. Expected model/id")
	}
	model, id := parts[0], parts[1]

	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	// Get the document first
	resp, err := client.GetDocumentWithResponse(context.Background(), model, id)
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	if resp.JSON200 == nil {
		log.Fatalf("Unexpected response: %v", resp.StatusCode())
	}

	// Create temporary file
	tmpfile, err := ioutil.TempFile("", "apogy-edit-*.yaml")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write object to temp file
	enc, err := yaml.Marshal(resp.JSON200)
	if err != nil {
		log.Fatal(err)
	}
	tmpfile.Write(enc)
	tmpfile.Close()

	// Get file info for later comparison
	originalInfo, err := os.Stat(tmpfile.Name())
	if err != nil {
		log.Fatal(err)
	}

	// Open editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	cmd2 := exec.Command(editor, tmpfile.Name())
	cmd2.Stdin = os.Stdin
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	if err := cmd2.Run(); err != nil {
		log.Fatal(err)
	}

	// Check if file was modified
	newInfo, err := os.Stat(tmpfile.Name())
	if err != nil {
		log.Fatal(err)
	}

	if newInfo.ModTime() == originalInfo.ModTime() {
		fmt.Println("Edit cancelled, no changes made")
		return
	}

	// Read modified file and put object
	file = tmpfile.Name()
	put(cmd, args)
}
