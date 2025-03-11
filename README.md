![apogydb logo](./apogy.png)
=======

a cloud native json schema database with composable validation

 - built on tikv and nats
 - strongly typed schema with bindings in most programming languages
 - durable external reconcilers inspired by kubernetes
 - first class object manipulation cli


## quickstart

    go install .
    docker compose up -d
    apogy server

Let's create a model, which defines a schema.
It can be hooked into many composable reactors which validate and mutate documents.
The schema is defined in [yema](https://github.com/aep/yema) which should be faily obvious.

```yaml
---
model: Model
id:    com.example.Book
val:
  schema:
    name: string
    author: string
    isbn?: string
  reactors:
    - schema
---
model:  com.example.Book
id:     dune
val:
  name:   Dune
  author: Frank Herbert
```

    apogy apply -f examples/quickstart.yaml

Try what happens when you violate the schema

    apogy apply -f - <<EOF
    model:  com.example.Book
    id:     dune
    val:
      name:   11111111111
    EOF

    com.example.Book d9e01f23-4a56-7b89-c012-d345e6789f01 rejected: reactor schema rejected change: not serializable, field 'name' must be a string



you can also define more complex validation using a [cue](https://cuelang.org) reactor. cue reactors can also mutate the document.

```yaml
---
model:   Model
id:      com.example.Book
val:
  schema:
    name: string
    author: string
    isbn?: string
  reactors:
    - cue:
        import "strings"
        name: strings.MinRunes(2)
        validatedByCue: "yes"
```


## query

we can search by any modelled property

    apogy q 'com.example.Book(val.name="Dune", val.author="Frank Herbert")'

while you can specify multiple search terms, databases can really only run one term in O(1) and the rest is O(n).
there is currently no automatic query planner, and i'm not sure if there should be one at all.
statistic query planners are good for first impressions but can later degrade in production.

instead, order the filters manually by decreasing cardinality,
the first filter should be the highest cardinality, meaning most specific, returning the fewest unique results.

in the above example we first specify name=Dune, which is in the world of books is very specific.
There are only two books named Dune in the example dataset, so the next filter only needs to look at those 2.


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


## mutations

while concurrent puts will fail, concurrent mutations never fail

    model:   com.example.Oligarch
    id:      bezos
    mut:
      money:
        add: 1200000000000
