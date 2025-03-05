package client

import (
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	yg "github.com/aep/yema/golang"
	yp "github.com/aep/yema/parser"
	"github.com/spf13/cobra"
)

func runBuild(cmd *cobra.Command, args []string) {

	os.Mkdir("apogy", 0750)

	var templateMain = template.Must(template.New("client.go").Parse(TEMPLATE_MAIN))
	var templateModel = template.Must(template.New("model.go").Parse(TEMPLATE_MODEL))

	docs, err := parseFile(file)
	if err != nil {
		log.Fatal(err)
	}

	var path string

	var types = make(map[string]string)

	for _, doc := range docs {

		if doc.Model != "Model" {
			continue
		}

		id := strings.Split(doc.Id, ".")
		if path == "" {
			path = strings.Join(id[:len(id)-1], ".")
		} else if path != strings.Join(id[:len(id)-1], ".") {
			panic(fmt.Errorf("models with different domains cannot be generated into the same api package. %s != %s", path, strings.Join(id[:len(id)-1], ".")))
		}

		types[id[len(id)-1]] = doc.Id

		val, _ := doc.Val.(map[string]interface{})

		schema, _ := val["schema"].(map[string]interface{})

		yy, err := yp.From(schema)
		if err != nil {
			log.Fatal(err)
		}

		code, err := yg.ToGolang(yy, yg.Options{
			Package:  "apogy",
			RootType: id[len(id)-1] + "Val",
		})
		if err != nil {
			log.Fatal(err)
		}

		fo, err := os.Create("apogy/" + id[len(id)-1] + "Model.go")
		if err != nil {
			log.Fatal(err)
		}
		defer fo.Close()
		fo.Write(code)

		fo, err = os.Create("apogy/" + id[len(id)-1] + "Client.go")
		if err != nil {
			log.Fatal(err)
		}
		defer fo.Close()

		err = templateModel.Execute(fo, map[string]interface{}{
			"Type":    id[len(id)-1],
			"ModelId": doc.Id,
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	fo, err := os.Create("apogy/client.go")
	if err != nil {
		log.Fatal(err)
	}
	defer fo.Close()

	err = templateMain.Execute(fo, map[string]interface{}{
		"Types": types,
	})
	if err != nil {
		log.Fatal(err)
	}
}

const TEMPLATE_MODEL = `
package apogy

import (
)

`

const TEMPLATE_MAIN = `
package apogy

import (
	openapi "github.com/aep/apogy/api/go"
)


type Document[Val any] struct {
	openapi.Document
	Val Val ` + "`json:\"val\"`" + `
}

{{range $t, $n := .Types}} 
type {{$t}} Document[{{$t}}Val]
{{end}} 

type Client struct {
	openapi.ClientInterface
	{{range $t, $n := .Types}}
	{{$t}} *openapi.TypedClient[{{$t}}]
	{{end}}
}

type ClientOption openapi.ClientOption

func NewClient(server string, opts ...ClientOption) (*Client, error) {

	var optss []openapi.ClientOption
	for _, o := range opts {
		optss = append(optss, openapi.ClientOption(o))
	}

	client, err := openapi.NewClient(server, optss...)
	if err != nil {
		return nil, err
	}

	r := &Client{ClientInterface: client}

	{{range $t,$n := .Types}}
	r.{{$t}}  = &openapi.TypedClient[{{$t}}]{client, "{{$n}}"}
	{{end}}

	return r, nil
}

`
