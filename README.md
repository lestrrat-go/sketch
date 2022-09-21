sketch - Generate JSON (De)serializable Object From Go Schema
===

`sketch` is tool to generate Go structs and utility methods from a
schema defined in Go code.

It is aimed to provide the basic implementation for those structs that
represents something that is given through a relatively
free-form definition format (such as JSON and YAML), while providing
basic type-safety along with the ability to work with user-defined
fields in an uniform manner.

The goal of this project is to provide a foundation for projects like
`github.com/lestrrat-go/jwx` that need to express in Go objects that
are represented as JSON.

WARNING: THIS PROJECT IS EXPERIMENTAL. It does work, but you will
most likely have to implement the missing pieces if it does not do
what you want it to do.

# Use-Case / Assumptions

`sketch` should be used if you are dealing with objects that need to
represent a piece of that datat that matches the following criteria:

1. The data has a JSON/YAML/etc based schema.
1. The data is defined with certain pre-defined fields. Fields may have been assigned a type, and you would like to handle them in a type-safe manner.
1. The data needs to be serialized from / deserialized from their respective formats.
1. The data may contain arbitrary user-defined fields, of which you do not know the schema before hand. These need to be serialized / deserialized the same way as the pre-defined fields.

To sum this all up: if you have a schema that defines an object,
but the user is free to extend the schema, and you need to serialize/deserialize
from JSON/YAML/etc, this may be the right tool for you.

# How `sketch` works

`sketch` provides the `schema` package for you to describe an object definition.

Once you have these objects, you can use `sketch` command to generate the actual
code to be used.

First install the `sketch` tool. Note that the version of your `sketch` tool
must match the version of the `github.com/lestrrat-go/sketch` library you are
using.

```
# TODO: change @latest to a released version
go get github.com/lestrrat-go/sketch/cmd/sketch@latest
```

Then declare a module that contains your schema.

We will assume that you want to implement a module named `myproject.com/mymodule`,
hosted in your `~/go` directory:

```
~/go/myproject.com/mymodule`
```

We also assume that the above directory is where you want to generate the
files into, but you want to "declare" the definitions under `~/go/myproject.com/mymodule/schema`.

Given the above assumptions, create the directory `~/go/myproject.com/mymodule`
and initialize a new go module if you have not already done so:

```bash
mkdir ~/go/myproject.com/mymodule
cd ~/go/myproject.com/mymodule

go mod init myproject.com/mymodule
```

It is important to setup a proper module, as we need to compile
a small intermediate program to generate the final code, and your code must
be referencable from this small program.

Note that your module does NOT need to be available at an external URL,
as `sketch` will take care of this resolution by using the `replace`
directive in the generate `go.mod` file.

Next, create the directory to store your schema.

```bash
mkdir schema
```

Inside this directory, you can have any number of Go files, but `sketch`
will pick up only those `struct`s that are declared as embedding
`schema.Base` types:

```go
// mymodule/schema/schema.go
package schema

import (
  "github.com/lestrrat-go/sketch/schema"
)

type Thing struct {
  schema.Base
}
```

This will declare that you want `sketch` to generate code for an object `Thing`.
The name of this package must be uppercased so that `sketch` can see it.

You may opt to "rename" the object. By default the name of the schema object will
be used as-is, but by declaring the method `Name()` on the schema object, you can
alter the generated object name. If below code is included, sketch will generate
a struct named `fooBarBaz`:

```go
func (Thing) Name() string {
  return "fooBarBaz"
}
```

You may opt to specify the package name that the object belongs to.
By default last component from the output directory (in this example's case
`mymodule`) will be used, but you can override this by declaring a method
`Package()` on the schema object:

```go
func (Thing) Package() string {
  return "awesomeModule"
}
```

Finally, you ou will want to declare the list of fields in this object.
This is done by declaring a method named `Fields()` on the schema object,
which returns a list of `schema.Field` objects.

```go
func (Thing) Fields() []schema.Field{
  return []schema.Field{
    schema.String("Foo").
      JSON("foo-field"),
  }
}
```

In the above sample, the use of `schema.String()` implies that a field
of `string` type is to be declared, with an exported name of `Foo`. It
also specifies that the value of this field will be stored in a field
named `foo-field` when serialized to JSON.

This will in turn instruct `sketch` to generate code that

1. Creates `mymodule.Thing` struct
1. Creates an accessor for `Foo`, but will store `Foo` as an unexported field.
1. Creates UnmarshalJSON/MarshalJSON methods that recognizes field `Foo` -- that it is stored as `foo-field`, and that its value must be a `string`
1. The JSON methods will also recognize other fields and stores them appropriately, but you will onlybe able to get to them via `(*Thing).Get()`

# Templates

Users can specify their own templates to be processed along side with the
system templates that come with this tool.

Provide your templates that define templates named  as below, and
they will be included automatically.

```
package ...
...
{{ ext/object/header }}
...
// object definition
type Object struct { ... }

// methods ...
...
{{ ext/object/footer }}
...
```
