package client

import (
	"apogy/api"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	file    string
	baseURL = "http://localhost:5051"

	CMD = &cobra.Command{
		Use:   "client",
		Short: "HTTP client",
	}

	putCmd = &cobra.Command{
		Use:     "put",
		Aliases: []string{"apply"},
		Short:   "Put an object from file",
		Run:     put,
	}

	getCmd = &cobra.Command{
		Use:   "get [model/id]",
		Short: "Get an object",
		Args:  cobra.ExactArgs(1),
		Run:   get,
	}

	editCmd = &cobra.Command{
		Use:   "edit [model/id]",
		Short: "Edit an object",
		Args:  cobra.ExactArgs(1),
		Run:   edit,
	}

	searchCmd = &cobra.Command{
		Use:     "search [model] [q]",
		Aliases: []string{"find"},
		Short:   "Search for objects",
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

func parseFile(file string) ([]api.Object, error) {
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
	var objects []api.Object

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var obj api.Object
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			return nil, fmt.Errorf("failed to parse document: %v", err)
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func put(cmd *cobra.Command, args []string) {
	objects, err := parseFile(file)
	if err != nil {
		log.Fatal(err)
	}

	for _, obj := range objects {
		var req api.PutObjectRequest
		req.Object = obj

		jsonData, err := json.Marshal(req)
		if err != nil {
			panic(err)
		}

		resp, err := http.Post(baseURL+"/o", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Fatalf("Failed to put object: %v", err)
		}
		defer resp.Body.Close()

		var putResp api.PutObjectResponse
		if resp.StatusCode >= 300 {
			if err := json.NewDecoder(resp.Body).Decode(&putResp); err == nil && putResp.Error != "" {
				log.Fatalf("Failed to put object: %d: %s", resp.StatusCode, putResp.Error)
			}
			log.Fatalf("Failed to put object: %d: %s", resp.StatusCode, resp.Status)
		}

		if err := json.NewDecoder(resp.Body).Decode(&putResp); err != nil {
			log.Fatalf("Failed to decode response: %v", err)
		}

		if putResp.Error != "" {
			fmt.Fprintf(os.Stderr, "remote error: %s\n", putResp.Error)
			os.Exit(1)
			return
		}
		fmt.Println(putResp.Path)
	}
}

func get(cmd *cobra.Command, args []string) {
	id := args[0]

	resp, err := http.Get(fmt.Sprintf("%s/o/%s", baseURL, id))
	if err != nil {
		log.Fatalf("Failed to get object: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "not found\n")
		os.Exit(1)
		return
	}

	var obj api.Object
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}

	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	if err := enc.Encode(obj); err != nil {
		log.Fatalf("Failed to encode as YAML: %v", err)
	}
}

func parseFilter(arg string) api.Filter {
	filter := api.Filter{}
	// Check for comparison operators
	if strings.Contains(arg, "=") {
		parts := strings.Split(arg, "=")
		filter.Key = parts[0]
		filter.Equal = parts[1]
	} else if strings.Contains(arg, ">") {
		parts := strings.Split(arg, ">")
		filter.Key = parts[0]
		filter.Greater = parts[1]
	} else if strings.Contains(arg, "<") {
		parts := strings.Split(arg, "<")
		filter.Key = parts[0]
		filter.Less = parts[1]
	} else {
		filter.Key = arg
	}

	return filter
}

func search(cmd *cobra.Command, args []string) {

	var sq api.SearchRequest
	sq.Model = args[0]

	for _, arg := range args[1:] {
		sq.Filters = append(sq.Filters, parseFilter(arg))
	}

	filterJson, err := json.Marshal(sq)
	if err != nil {
		log.Fatalf("Failed to encode filter: %v", err)
	}

	resp, err := http.Post(baseURL+"/q", "application/json", bytes.NewBuffer(filterJson))
	if err != nil {
		log.Fatalf("Failed to search objects: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatalf("Failed to search: %d: %s", resp.StatusCode, resp.Status)
	}

	var cursor api.Cursor
	if err := json.NewDecoder(resp.Body).Decode(&cursor); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}

	for _, p := range cursor.Keys {
		fmt.Println(sq.Model + "/" + p)
	}
}

func edit(cmd *cobra.Command, args []string) {
	id := args[0]

	// Get the object first
	resp, err := http.Get(fmt.Sprintf("%s/o/%s", baseURL, id))
	if err != nil {
		log.Fatalf("Failed to get object: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "not found\n")
		os.Exit(1)
		return
	}

	var obj api.Object
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
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
