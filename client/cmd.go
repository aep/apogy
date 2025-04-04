package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	openapi "github.com/aep/apogy/api/go"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var (
	file string

	fullDoc bool

	putCmd = &cobra.Command{
		Use:     "put",
		Aliases: []string{"apply"},
		Short:   "Put a document from file",
		Run:     put,
	}

	getCmd = &cobra.Command{
		Use:   "get [model] [id]",
		Short: "Get a document",
		Args:  cobra.ExactArgs(2),
		Run:   get,
	}

	editCmd = &cobra.Command{
		Use:   "edit [model] [id]",
		Short: "Edit a document",
		Args:  cobra.ExactArgs(2),
		Run:   edit,
	}

	rmCmd = &cobra.Command{
		Aliases: []string{"delete", "del"},
		Use:     "rm [model] [id]",
		Short:   "Delete a document",
		Args:    cobra.ExactArgs(2),
		Run:     del,
	}

	searchCmd = &cobra.Command{
		Use:     "search [model] [q]",
		Aliases: []string{"find", "ls", "list"},
		Short:   "Search for documents",
		Args:    cobra.MinimumNArgs(1),
		Run:     search,
	}

	qCmd = &cobra.Command{
		Use:     "q [q]",
		Aliases: []string{"query"},
		Short:   "AQL Query",
		Args:    cobra.MinimumNArgs(1),
		Run:     query,
	}

	mutCmd = &cobra.Command{
		Use:   "mut [q]",
		Short: "AQL Mutation",
		Args:  cobra.MinimumNArgs(1),
		Run:   mutate,
	}

	buildCmd = &cobra.Command{
		Use:   "build",
		Short: "build api",
		Args:  cobra.MinimumNArgs(0),
		Run:   runBuild,
	}
)

func RegisterCommands(root *cobra.Command) {
	putCmd.Flags().StringVarP(&file, "file", "f", "", "Path to JSON/YAML file")
	putCmd.MarkFlagRequired("file")

	searchCmd.Flags().BoolVarP(&fullDoc, "full", "f", false, "Request full document for search results")

	root.AddCommand(putCmd)
	root.AddCommand(getCmd)
	root.AddCommand(editCmd)
	root.AddCommand(searchCmd)
	root.AddCommand(qCmd)
	root.AddCommand(rmCmd)
	root.AddCommand(mutCmd)

	buildCmd.Flags().StringVarP(&file, "file", "f", "", "Path to JSON/YAML file")
	buildCmd.MarkFlagRequired("file")
	root.AddCommand(buildCmd)
}

func parseFile(file string) ([]openapi.Document, error) {
	var data []byte
	var err error

	if file == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	docs := strings.Split(string(data), "\n---\n")
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
	addr := os.Getenv("APOGY_ADDR")
	if addr == "" {
		addr = "http://localhost:27666"
	}

	// Set up TLS config if client certs are provided
	clientCertPath := os.Getenv("APOGY_CLIENT_CERT")
	clientKeyPath := os.Getenv("APOGY_CLIENT_KEY")
	caCertPath := os.Getenv("APOGY_CA_CERT")

	var httpClient *http.Client
	if clientCertPath != "" && clientKeyPath != "" {
		// Load client cert and key
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate and key: %v", err)
		}

		// Create TLS config
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}

		// Load CA cert if provided
		if caCertPath != "" {
			caCert, err := os.ReadFile(caCertPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA certificate: %v", err)
			}

			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to append CA certificate")
			}
			tlsConfig.RootCAs = caCertPool
		}

		// Create HTTP client with the TLS config
		transport := &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		httpClient = &http.Client{
			Transport: transport,
		}
	}

	// If httpClient is configured, use it, otherwise use default
	var client *openapi.ClientWithResponses
	var err error
	if httpClient != nil {
		client, err = openapi.NewClientWithResponses(addr, openapi.WithHTTPClient(httpClient))
	} else {
		client, err = openapi.NewClientWithResponses(addr)
	}

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
				fmt.Fprintf(os.Stderr, "%s %s rejected: %s\n", obj.Model, obj.Id, *resp.JSON400.Message)
				os.Exit(2)
			} else if resp.JSON422 != nil {
				fmt.Fprintf(os.Stderr, "%s %s rejected: %s\n", obj.Model, obj.Id, *resp.JSON422.Message)
				os.Exit(9)
			} else if resp.JSON409 != nil {
				fmt.Fprintf(os.Stderr, "%s %s rejected: %s\n", obj.Model, obj.Id, *resp.JSON409.Message)
				os.Exit(9)
			} else {
				fmt.Fprintf(os.Stderr, "Unexpected response: %v\n", resp.StatusCode())
				os.Exit(10)
			}
		}

		fmt.Println(obj.Model, obj.Id, "accepted")
	}
}

func get(cmd *cobra.Command, args []string) {
	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.GetDocumentWithResponse(context.Background(), args[0], args[1])
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

func del(cmd *cobra.Command, args []string) {
	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.DeleteDocumentWithResponse(context.Background(), args[0], args[1])
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}
	if resp.StatusCode() != 200 && resp.StatusCode() != 404 {
		if resp.JSON400 != nil {
			log.Fatalf("failed to delete: %s", *resp.JSON400.Message)
		}
		log.Fatalf("Unexpected response: %v", resp.StatusCode())
	}
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
	} else if a, b, ok := strings.Cut(arg, "^"); ok {
		bi := any(b)
		filter.Key = a
		filter.Prefix = &bi
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

	var cursor *string
	for {
		req := openapi.SearchRequest{
			Model:   args[0],
			Filters: &filters,
			Cursor:  cursor,
			Full:    &fullDoc,
		}

		resp, err := client.SearchDocumentsWithResponse(context.Background(), req)
		if err != nil {
			log.Fatalf("Failed to search documents: %v", err)
		}

		if resp.JSON200 == nil {
			log.Fatalf("Unexpected response: %v", resp.StatusCode())
		}

		for _, doc := range resp.JSON200.Documents {
			if fullDoc {
				enc, err := yaml.Marshal(doc)
				if err != nil {
					log.Fatalf("Failed to encode as YAML: %v", err)
				}
				os.Stdout.Write(enc)
				fmt.Println("---")
			} else {
				fmt.Println(doc.Id)
			}
		}

		// If there's no cursor or no IDs returned, we've reached the end
		if resp.JSON200.Cursor == nil || len(resp.JSON200.Documents) == 0 {
			break
		}

		cursor = resp.JSON200.Cursor
	}
}

func query(cmd *cobra.Command, args []string) {
	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	var argss []interface{}
	for _, arg := range args[1:] {
		argss = append(argss, interface{}(arg))
	}

	it := client.Query(context.Background(), args[0], argss...)

	for doc, err := range it {
		if err != nil {
			log.Fatalf("query error: %s", err)
		}
		fmt.Println("---")
		enc, err := yaml.Marshal(doc)
		if err != nil {
			log.Fatalf("Failed to encode as YAML: %v", err)
		}
		os.Stdout.Write(enc)
	}
}

func mutate(cmd *cobra.Command, args []string) {
	client, err := getClient()
	if err != nil {
		log.Fatal(err)
	}

	var argss []interface{}
	for _, arg := range args[1:] {
		argss = append(argss, interface{}(arg))
	}

	it := client.Query(context.Background(), args[0], argss...)

	for doc, err := range it {
		if err != nil {
			log.Fatalf("query error: %s", err)
		}
		fmt.Println("---")
		enc, err := yaml.Marshal(doc)
		if err != nil {
			log.Fatalf("Failed to encode as YAML: %v", err)
		}
		os.Stdout.Write(enc)
	}
}

func edit(cmd *cobra.Command, args []string) {
	model, id := args[0], args[1]

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
	tmpfile, err := os.CreateTemp("", "apogy-edit-*.yaml")
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
