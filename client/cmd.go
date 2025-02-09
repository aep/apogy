package client

import (
	"apogy/proto"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v3"
	"time"
)

var (
	file    string
	address = "localhost:5051"

	CMD = &cobra.Command{
		Use:   "client",
		Short: "gRPC client",
	}

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

func init() {
	putCmd.Flags().StringVarP(&file, "file", "f", "", "Path to JSON/YAML file")
	putCmd.MarkFlagRequired("file")

	CMD.AddCommand(putCmd)
	CMD.AddCommand(getCmd)
	CMD.AddCommand(editCmd)
	CMD.AddCommand(searchCmd)
}

type History struct {
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
}
type Object struct {
	Model   string                 `yaml:"model"`
	ID      string                 `yaml:"id"`
	Version *uint64                `yaml:"version,omitempty"`
	History *History               `yaml:"history,omitempty"`
	Val     map[string]interface{} `yaml:"val"`
	Status  map[string]interface{} `yaml:"status,omitempty"`
}

func parseFile(file string) ([]Object, error) {
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
	var objects []Object

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var obj Object
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			return nil, fmt.Errorf("failed to parse document: %v", err)
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func getClient() (proto.DocumentServiceClient, *grpc.ClientConn) {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	return proto.NewDocumentServiceClient(conn), conn
}

func put(cmd *cobra.Command, args []string) {
	objects, err := parseFile(file)
	if err != nil {
		log.Fatal(err)
	}

	client, conn := getClient()
	defer conn.Close()

	for _, obj := range objects {
		val, err := structpb.NewStruct(obj.Val)
		if err != nil {
			log.Fatalf("Failed to convert value: %v", err)
		}

		doc := &proto.Document{
			Model:   obj.Model,
			Id:      obj.ID,
			Val:     val,
			Version: obj.Version,
		}

		resp, err := client.PutDocument(context.Background(), &proto.PutDocumentRequest{
			Document: doc,
		})
		if err != nil {
			log.Fatalf("Failed to put document: %v", err)
		}

		fmt.Println(resp.Path)
	}
}

func get(cmd *cobra.Command, args []string) {
	parts := strings.Split(args[0], "/")
	if len(parts) != 2 {
		log.Fatal("Invalid id format. Expected model/id")
	}
	model, id := parts[0], parts[1]

	client, conn := getClient()
	defer conn.Close()

	resp, err := client.GetDocument(context.Background(), &proto.GetDocumentRequest{
		Model: model,
		Id:    id,
	})
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	obj := Object{
		Model:   resp.Model,
		ID:      resp.Id,
		Version: resp.Version,
		Val:     resp.Val.AsMap(),
		Status:  resp.Status.AsMap(),
	}

	if resp.History != nil {
		obj.History = &History{
			Created: resp.History.Created.AsTime(),
			Updated: resp.History.Updated.AsTime(),
		}
	}

	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	if err := enc.Encode(obj); err != nil {
		log.Fatalf("Failed to encode as YAML: %v", err)
	}
}

func parseFilter(arg string) *proto.Filter {
	filter := &proto.Filter{}

	// Check for comparison operators
	if strings.Contains(arg, "=") {
		parts := strings.Split(arg, "=")
		val, err := structpb.NewValue(parts[1])
		if err != nil {
			log.Fatalf("Failed to parse value: %v", err)
		}
		filter.Key = parts[0]
		filter.Condition = &proto.Filter_Equal{Equal: val}
	} else if strings.Contains(arg, ">") {
		parts := strings.Split(arg, ">")
		val, err := structpb.NewValue(parts[1])
		if err != nil {
			log.Fatalf("Failed to parse value: %v", err)
		}
		filter.Key = parts[0]
		filter.Condition = &proto.Filter_Greater{Greater: val}
	} else if strings.Contains(arg, "<") {
		parts := strings.Split(arg, "<")
		val, err := structpb.NewValue(parts[1])
		if err != nil {
			log.Fatalf("Failed to parse value: %v", err)
		}
		filter.Key = parts[0]
		filter.Condition = &proto.Filter_Less{Less: val}
	} else {
		filter.Key = arg
	}

	return filter
}

func search(cmd *cobra.Command, args []string) {
	client, conn := getClient()
	defer conn.Close()

	var filters []*proto.Filter
	for _, arg := range args[1:] {
		filters = append(filters, parseFilter(arg))
	}

	resp, err := client.SearchDocuments(context.Background(), &proto.SearchRequest{
		Model:   args[0],
		Filters: filters,
	})
	if err != nil {
		log.Fatalf("Failed to search documents: %v", err)
	}

	for _, id := range resp.Ids {
		fmt.Printf("%s/%s\n", args[0], id)
	}
}

func edit(cmd *cobra.Command, args []string) {
	parts := strings.Split(args[0], "/")
	if len(parts) != 2 {
		log.Fatal("Invalid id format. Expected model/id")
	}
	model, id := parts[0], parts[1]

	client, conn := getClient()
	defer conn.Close()

	// Get the document first
	resp, err := client.GetDocument(context.Background(), &proto.GetDocumentRequest{
		Model: model,
		Id:    id,
	})
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	obj := Object{
		Model:   resp.Model,
		ID:      resp.Id,
		Val:     resp.Val.AsMap(),
		Version: resp.Version,
		Status:  resp.Status.AsMap(),
	}

	// Create temporary file
	tmpfile, err := ioutil.TempFile("", "apogy-edit-*.yaml")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write object to temp file
	enc := yaml.NewEncoder(tmpfile)
	enc.SetIndent(2)
	if err := enc.Encode(obj); err != nil {
		log.Fatal(err)
	}
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
