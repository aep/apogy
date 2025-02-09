![apogydb logo](./apogy.png)
=======

a cloud native reactive json schema database built on tikv


 - strongly typed with jsonschema
 - declarative migrations
 - fulltext search
 - first class object manipulation cli


## quickstart

let's create a model.
the most simple model in apogy is a jsonschema.
let's also create an object of that model.

```yaml
---
model: Model
id:    com.example.Book
val:
  properties:
    name:
      type: string
    author:
      type: string
  required:
   - name
---
model:  com.example.Book
id:     dune
val:
  name:   Dune
  author: Frank Herbert
```

    apogy apply -f examples/quickstart.yaml

try what happens when you violate the jsonschema

    apogy apply -f - <<EOF
    model:  com.example.Book
    id:     dune
    val:
      name:   11111111111
    EOF

    '/name' does not validate with schema://com.example.Book#/properties/name/type: expected string, but got number


the jsonschema does not explicitly set additionalProperties: false, so this model allows arbitrary other json keys. however, it wont be searchable.


## search

we can search by any modelled property

    apogy find val.name="Dune" val.author="Frank Herbert"
    apogy get com.example.Book/dune

while you can specify multiple search terms, databases can really only run one term in O(1) and the rest is O(n).
there is currently no automatic query planner, and i'm not sure if there should be one at all.
statistic query planners are good for first impressions but can later degrade in production.

instead, order the filters manually by decreasing cardinality,
the first filter should be the highest cardinality, meaning most specific, returning the fewest unique results.

in the above example we first specify name=Dune, which is in the world of books is very specific.
the next filter for val.author will be run against only a few items.




## optimistic concurrency

apogy does not support locking.
instead fetch the object first, then send an update keeping the version key intact
the server will reject the put if a different client has updated the object between the get and put.

this example will succeed because the persisted object version is 1

    apogy apply -f - <<EOF
    model:   com.example.Book
    id:      dune
    version: 1
    val:
      name:   Dune 2000
      author: Frank Herbert
    EOF

doing the same apply again is indempotent with no error,
however trying to change the val while using an outdated version will fail

    apogy apply -f - <<EOF
    model:   com.example.Book
    id:      dune
    version: 1
    val:
      name:   Dune 3000
      author: Frank Herbert
    EOF

    Failed to put object: 409: version is out of date




## dev

this unfortunately requires setting "pd" to 127.0.0.1 in /etc/hosts

    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

    docker compose up -d
