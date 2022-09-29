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
~/go/myproject.com/mymodule
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

## Syntax 

Templates in `sketch` are all written using `text/template`.

## Variables

The only available template variable is the current schema object that is being
used to generate code. This will be set to `$` globally, and is available as the
default context variable for each template block

```
{{ define "ext/object/header" }}
{{ .Name }}{{# will print schema object name }}
{{ $.Name }}{{# same as above }}
{{ end }}
```

## Extra Templates / Overriding Templates

Users can specify their own templates to be processed along side with the
system templates that come with this tool.

Assuming you have your templates located under `/path/to/templates`, you can
specify `sketch` to use templates in this directory by using the `--tmpl-dir`
command line option:

```
sketch --tmpl-dir=/path/to/templates
```

### Core Templates

Core Templates are templates that come with `sketch` tool itself. Normally you do
not need to do anything with these, but if you want to fundamentally change the
way `sketch` generates code, you can override them from your extra templates.

To do this, simply define template blocks with the same name as the
template blocks provided by the core templates.

For example, to override the main template that generates object headers, you can
declare a template block named `"object/header"` in your template:

```
{{ define "object/header" }}
// Your custom header code goes here
{{ end }}
```

Note that this does not _ADD_ to the core template, but completely replaces it.

The following is the list of template block names that you may override.

| Name | Description |
|------|-------------|
| object | The main template for generating object code. |
| object/header | Template for the header part of the object, including the top comment, package name, imports |
| object/footer | Template for the footer part of the object |
| object/struct | Template for the struct definition of the object |

### Optional Templates

Optional templates are only rendered if the user provides them
(you can think of them as hooks).

If you would like to augment the generated template, choose an appropriate
optional template, and declare a template block by that name.

The following is the list of template block names that you may provide.

| Name | Description |
|------|-------------|
| ext/object/header | Called from within object/header template |
| ext/object/footer | Called from within object/footer template |
