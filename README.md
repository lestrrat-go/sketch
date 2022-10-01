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

Then run the `sketch` command line utility. It is assumed that your
schema above resides under `/path/to/schema`, and that you want to
generate code to `/path/to/dst`

```
sketch -d /path/to/dst /path/to/schema
```

If successful, you should see several files created in the `/path/to/dst`
directory.

# Generated Code

`sketch` allows users to exclude generation of specific parts.
You can use the internal names (show below) to specify that certain exclude parts should not be generated via `--exclude`.

| Method/Struct | Internal Name | Description |
|---------------|---------------|-------------|
| Main Object   | N/A           | The main struct defnition. Will have the name provided by your schema |
| `(Object).Set`| `object.method.Set`  | Method to set the value of an arbitrary field by its JSON field name |
| `(Object).Get`| `object.method.Get`  | Method to retrieve the value of an arbitrary field by its JSON field name |
| `(Object).Has` | `object.method.Has` | Method to query the presence of a value of an arbitrary field by its JSON field name |
| `(Object).HasXXXXX` | `object.method.HasXXXXX` | Method to query the presence of a value of field `XXXXX` |
| `(Object).XXXXX` | `object.method.XXXXX` | Method to retrieve the value of field `XXXXX` |
| `(Object).Remove` | `object.method.Remove` | Method to remove the value of an arbitrary field by its JSON field name |
| `(Object).Keys` | `object.method.Keys` | Method to retrieve the JSON key names that are present in the object |
| `(Object).MarshalJSON` | `object.method.MarshalJSON` | Method to serialize the object into JSON |
| `(Object).UnmarshalJSON` | `object.method.UnmarshalJSON` | Method to deserialize the object from JSON |
| `(Object).Clone` | `object.method.Clone` | Method to clone an object |
| Builder Object | `builder.struct` | The builder struct definition. Will have the name of your object plus "Builder" |
| `(Builder).XXXXX` | `builder.method.XXXX` | Method to initialize the value of field `XXXXX` via the Builder |
| `(Builder).Build` | `builder.method.Build` | Method to build and return the object from the Builder |
| `(Builder).MustBuild` | `builder.method.MustBuild` | Method to build and return the object from the Builder |

## Optional Components

### Builder

By default only the object to hold the data is created. Accessors are
provided by default, but you can only populate the data by deserializing
from JSON.

This is a problem if you would like to programmatically build your object.
`sketch` can provide "builders" for each of the object so that you can
create them programmatically, by adding the `--with-builders` command line
option when generating code:

```
sketch --with-builders ...
```

Then you will be able to compose the object using builders in the
following fashion:

```go
b := NewBuilder().
   FieldA(...).
   FieldB(...).
   FieldC(...).
   MustBuild()
```

### "HasXXXX" Methods

If you would like to differentiate between a field initialized with
its zero value and a field which has not been initialized at all,
you will want to generate the "Has" methods.

To do this, execute the `sketch` command with the `--with-has-methods`
command line argument.

```
sketch --with-has-methods ....
```

This will generate `Has<FIELD>` methods for each field, allowing you
to query if a field has been initialized:

```go
if obj.HasFieldA() { ... }
```

# Templates

## Syntax 

Templates in `sketch` are all written using `text/template`.

## Functions

`sketch` provides several functions that can be used from within the template

| Name | Signature | Description |
|------|-----------|-------------|
| comment | comment (string, any) | Formats the comment. The first argument can be a text/template style template. The second argument is the variable passed to the template. |
| hasTemplate | hasTemplate (string) bool | Returns true if the template specified in the argument exists |
| runTemplate | runTemplate (string, any) | Executes the named template with the second argument as the template variables |

## Variables

The only available template variable is the current schema object (the one
you declared using `schema.Base`) that is being used to generate code.

This will be set to `$` globally, and is available as the
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

There are two types of templates: ones that generate files, and others that are
used as components that is called from within other templates. Any template whose
name starts with `files/` will generate files according to some heuristics.

If the path component following `files/` is named `per-object`, the template is
applied for each of the object schema found. If the value is `per-run`, then
the template is evaluated only once.

Unless otherwise stated, all file names will be transformed so that it ends
with `_gen`. If the template name contains a suffix, the filename will end
with `_gen.$suffix`.

| Name | Description |
|------|-------------|
| files/per-object/object.go | Template for the main object generation. The filename generated by this emplate is special -- the entire file name (the portion for `object.go`) is replaced with the name of the object |
| files/per-run/sketch.go | Template for common code between all generate objects |

| Name | Description |
|------|-------------|
| object/builder | Template for the biulder part of the object |
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
| ext/object/header | User specified template to insert code at the beginning of the object.go code |
| ext/object/footer | User specified template to insert code at the end of the object.go code |

# Command Line

| Name | Description |
|------|-------------|
| --dst-dir=DIR | Specify the directory to write the generate files to |
| --exclude=PATTERN | Specify a pattern to exclude. value may be a RE2 compatible regular expression. May be specified multiple times |
| --tmpl-dir=DIR | Specify a template directory provided by the user. May be specified multiple times |
| --var=NAME=VALUE / --var=NAME=VALUE:TYPE | Specify a variable to be passed to the template as key/value pair. May optionally be followed by a type name: e.g. --var=foo=true:bool would store the value for `foo` as a Go bool instead of a string. Currently only supports `string`, `bool`, and `int` |
| --verbose | Enable verbose logging |

