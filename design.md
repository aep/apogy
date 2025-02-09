## reactors


 - they're very much backend code
 - reactors can be validators
 - but they can also be reconcilers,
    syncing the db state with the real world
    like k8s

 - wasm?


## search

the objects are stored in tikv as o/$model/$id -> json
with an additional forward search index

    f . val . author . Frank Herbert . T  . $model . $id

and a fuzzy full text index

    s . val . author . frank   . T  . $model . $id
    s . val . author . herbert . T  . $model . $id

unique index:

    u . Book . name . dune -> $id

the indexes allow most queries

    |------------------------------------- | -------------------------------- |
    | WHERE val.author = Frank Herbert     | s . val . author . Frank Herbert |
    | WHERE val.author != NULL             | s . val . author                 |
    | WHERE val.rating > 2                 | s . val . rating . 2             |
    | WHERE val.comment contains "robot"   | s . val . comment . robot        |
    | WHERE model = "Book"                 | s . model . Book                 |
