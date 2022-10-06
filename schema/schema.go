package schema

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/lestrrat-go/byteslice"
	"github.com/lestrrat-go/xstrings"
)

// InitializerArgumentStyle is used when you would like to override
// the default behavior and specify that a pseudo-TypeSpec (those types
// that are not constructed from actual Go objects)'s Builder methods
// should take a variadic form vs singular form.
type InitializerArgumentStyle int

const (
	InitializerArgumentAsSingleArg = iota
	InitializerArgumentAsSlice
)

const (
	defaultGetValueMethodName    = `GetValue`
	defaultAcceptValueMethodName = `AcceptValue`
)

// Interface exists to provide an abstraction for multiple
// schema objects that embed schema.Base object in the
// intermediate program that sketch produces. Users of sketch
// generally need not worry about this interface.
type Interface interface {
	Name() string
	Package() string
	Fields() []*FieldSpec
	Comment() string
	KeyNamePrefix() string
	GetKeyName(string) string
}

// Base is the struct that defines all of your schemas. You must include
// this as an embedded field in your schema object directly:
//
//	package mypkg
//	type Object struct {
//	  schema.Base
//	}
//
// Because sketch reads your schema definitions via Go's package inclusion
// mechanism, your schema object name must be an exported name (e.g.
// "MyObject" instead of "myObject")
type Base struct {
	// Variables is a storage for various global and default variables
	// used during code generation. While it is a public field, end-users
	// should not be using this variable for any purposes. It is only
	// visible because the sketch utility must be able to assign certain
	// parameters during the generation phase, and therefore the
	// field must be exported.
	//
	// tl;dr: DON'T USE IT (unless you are hacking sketch internals)
	Variables map[string]interface{}
}

var _ Interface = &Base{} // sanity

// StringVar returns the value stored in the Variables field as a string.
// If the value does not exist or the value is not of a string type, then
// returns the empty string
func (b Base) StringVar(name string) string {
	v, ok := b.Variables[name]
	if ok {
		if converted, ok := v.(string); ok {
			return converted
		}
	}
	return ""
}

// BoolVar returns the value stored in the Variables field as a bool.
// If the value does not exist or the value is not of a bool type, then
// returns false
func (b Base) BoolVar(name string) bool {
	v, ok := b.Variables[name]
	if ok {
		if converted, ok := v.(bool); ok {
			return converted
		}
	}
	return false
}

func (b Base) GetKeyName(fieldName string) string {
	return b.KeyNamePrefix() + fieldName + `Key`
}

// GenerateSymbol should return true if the given method is allowed to be
// generated. The argument consists of a prefix (e.g. "object." or "builder.")
// followed by the actual method name.
//
// By default all methods are allowed. Users may configure this on a per-object
// basis by providing their own `GenerateSymbol` method.
func (b Base) GenerateSymbol(s string) bool {
	m, ok := b.Variables["DefaultGenerateSymbol"]
	if !ok {
		return true
	}

	if m, ok := m.(func(string) bool); ok {
		return m(s)
	}
	return true
}

// SymbolName takes an internal name like "object.method.Foo" and returns
// the actual symbol name
func (b Base) SymbolName(s string) string {
	v, ok := b.Variables["DefaultSymbolRenames"]
	if ok {
		m, ok := v.(map[string]string)
		if ok {
			n, ok := m[s]
			if ok {
				return n
			}
		}
	}
	i := strings.LastIndexByte(s, '.')
	return s[i+1:]
}

// Name returns the name of the object to be generated. By default this
// value is set to the name of the schema object you created. Users may
// configure a different name by providing their own `Name` method.
//
// For example, since all schema objects must be exported, you would have
// to provide a custom `Name` method to tell sketch to generate an
// unexported object (e.g. `type Object { schema.Base }; func (Object) Name() string { return "object" }`)
func (b Base) Name() string {
	return b.StringVar(`DefaultName`)
}

// BuilderName returns the name of the Builder object.
// By default a name comprising of the return value from schema's `Name()`
// method and `Builder` will be used (e.g. "FooBuilder").
//
// If you are using an unexported name for your schema, you probably
// want to provide your own `BuilderName()` and `BuilderResultType()`
// methods, which control the struct name of the builder, and the
// returning value from calling `Build()` on the builder, respectively
func (b Base) BuilderName() string {
	return b.StringVar(`DefaultBuilderName`)
}

// BuilderResultType returns the name of the type that the builder
// object returns upon calling `Build()`.
//
// If you are using an unexported name for your schema, you probably
// want to provide your own `BuilderName()` and `BuilderResultType()`
// methods, which control the struct name of the builder, and the
// returning value from calling `Build()` on the builder, respectively
func (b Base) BuilderResultType() string {
	return b.StringVar(`DefaultBuilderResultType`)
}

// Package returns the name of the package that a schema belongs to.
// By default this value is set to the last element of the destination
// directory. For example, if you are generating files under `/home/lestrrat/foo`,
// the package name shall be `foo` by default.
//
// Users may configure a different name by providing their own `Package`
// method -- however,
func (b Base) Package() string {
	return b.StringVar(`DefaultPkg`)
}

// FilenameBase is used to generate the filename when generating files.
// Normally the name snake-cased version of the schema object (NOT the
// return value of `Name` method call) is used, but when you provide
// a value for thie method, the value is used verbatim
func (b Base) FilenameBase() string {
	return ""
}

// Fields returns the list of fields that should be associated with the
// schema object. User usually must
func (Base) Fields() []*FieldSpec {
	return []*FieldSpec(nil)
}

// Imports returns the list of packges to be imported.
func (Base) Imports() []string {
	return []string(nil)
}

// Comment returns the comment that should go withh the generated object.
// The comment should NOT contain the object name, as it would be taken
// from the return value of `Name` method
func (Base) Comment() string {
	return ""
}

// KeyNamePrefix returns the prefix that should be added to key name
// constants. By default no prefix is added, but if you have multiple
// objects with same field names, you will have to provide them
// with a prefix.
//
// When --with-key-name-prefix is specified, the default value is set
// to the name of the object, forcing sketch to generate key name
// constants in the form of `ObjectName` + `FieldName` + `Key`.
// You only need to provide your custom `KeyNamePrefix` method when
// you want to override this default behavior
func (b Base) KeyNamePrefix() string {
	return b.StringVar(`DefaultKeyNamePrefix`)
}

// TypeSpec is used to store information about a type, and contains
// various pieces of hints to generate objects/builders.
//
// One important concept that you need to be aware of is that of
// Apparent Types and Storage Types. An apparent type refers to the
// type that the end user sees. The pparent type may or may not
// match the storage type, which is the type that the generated
// object stores the data as.
//
// For example, you may want to expose a field as accepting a slice
// of strings (`[]string`), but store it as an object, maybe something
// like a custom `StringList` that you created. In this case
// `[]string` is the apparent type, and `StringList` is the storage type.
//
// The `TypeSpec` object is meant to store the storage type, but
// you can associate the corresponding apparent type via the
// `ApparentType` method.
type TypeSpec struct {
	name                  string // The name that the user procided us with
	element               string
	rawType               string // non-pointer type (could be the same as name)
	ptrType               string // pointer type (could be the same as name)
	apparentType          string // what the user sees
	acceptValueMethodName string
	getValueMethodName    string
	initArgStyle          InitializerArgumentStyle
	supportsLen           bool
	zeroVal               string
	isInterface           bool
	interfaceDecoder      string
}

func typeName(rv reflect.Type) string {
	var name string
	switch rv.Kind() {
	case reflect.Ptr:
		name = `*` + typeName(rv.Elem())
	case reflect.Slice:
		name = `[]` + typeName(rv.Elem())
	case reflect.Array:
		name = fmt.Sprintf(`[%d]%s`, rv.Len(), typeName(rv.Elem()))
	case reflect.Map:
		name = fmt.Sprintf(`map[%s]%s`, typeName(rv.Key()), typeName(rv.Elem()))
	default:
		name = rv.String()
	}

	return name
}

var typInterface = reflect.TypeOf((*interface{})(nil)).Elem()
var typError = reflect.TypeOf((*error)(nil)).Elem()

// Type creates a new TypeSpec from a piece of Go data
// using reflection. It populates all the required fields by
// inspecting the structure, which you can override later.
func Type(v interface{}) *TypeSpec {
	rv := reflect.TypeOf(v)

	typ := typeName(rv)

	var isInterface bool
	switch rv.Kind() {
	case reflect.String:
		if v.(string) != "" {
			panic(fmt.Sprintf(`schema.Type received a non-empty string value %q. possible misuse of schema.TypeName?`, v))
		}
	case reflect.Interface:
		isInterface = true
	}

	var ptrType string
	var rawType string
	switch rv.Kind() {
	case reflect.Ptr:
		rawType = typeName(rv.Elem())
		ptrType = typ
	case reflect.Slice, reflect.Interface, reflect.Array:
		rawType = typ
		ptrType = typ
	default:
		rawType = typ
		ptrType = `*` + typ
	}

	// If the type implements the Get() xxxx interface, we can deduce
	// the apparent type from the return type
	apparentType := rv
	var getValueMethodName string
	if m, ok := rv.MethodByName(defaultGetValueMethodName); ok {
		if m.Type.NumIn() == 1 && m.Type.NumOut() == 1 {
			apparentType = m.Type.Out(0)
			getValueMethodName = defaultGetValueMethodName
		}
	}

	var acceptValueMethodName string
	if m, ok := rv.MethodByName(defaultAcceptValueMethodName); ok {
		if m.Type.NumIn() == 2 && m.Type.In(1) == typInterface && m.Type.NumOut() == 1 && m.Type.Out(0) == typError {
			acceptValueMethodName = defaultAcceptValueMethodName
		}
	}

	var initArgStyle InitializerArgumentStyle

	// The initialization style depends on the apparent
	element := "sketch.UnknownType" // so it's easier to see
	if apparentType.Kind() == reflect.Slice {
		element = typeName(apparentType.Elem())
		initArgStyle = InitializerArgumentAsSlice
	}

	// Check if the storage type supports len() operation
	var supportsLen bool
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Chan:
		supportsLen = true
	}

	return &TypeSpec{
		name:                  typ,
		apparentType:          typeName(apparentType),
		rawType:               rawType,
		ptrType:               ptrType,
		element:               element,
		acceptValueMethodName: acceptValueMethodName,
		getValueMethodName:    getValueMethodName,
		initArgStyle:          initArgStyle,
		supportsLen:           supportsLen,
		zeroVal:               fmt.Sprintf("%#v", reflect.Zero(rv)),
		isInterface:           isInterface,
	}
}

// TypeName creates a TypeSpec from a string name.
//
// If you are allowed to include the struct into the schema code, you
// should not be using this function. Only use this function when
// you either have to refer to objects that you are about to generate
// using sketch, or for objects that you cannot import because of
// cyclic dependency, etc.
//
// Unlike `Type`, this constructor only takes the name of the
// type and otherwise has no other information. Therefore it assumes
// many things, and you will have to set many parameers manually.
//
// The defualt zero value is assumed to be `nil`
//
// If the name starts with a `[]`, then `IsSlice()` is automatically set to true
// If the name starts with a `map[`, then `IsMap()` is automatically set to true
func TypeName(name string) *TypeSpec {
	isSlice := strings.HasPrefix(name, `[]`)
	isMap := strings.HasPrefix(name, `map[`)
	element := "sketch.UnknownType" // so it's easier to see
	var initArgStyle InitializerArgumentStyle
	if isSlice {
		initArgStyle = InitializerArgumentAsSlice
		element = strings.TrimPrefix(name, `[]`)
	}

	var supportsLen bool
	if isSlice || isMap {
		supportsLen = true
	}

	var ptrType string
	var rawType string
	if strings.HasPrefix(name, `*`) {
		rawType = strings.TrimPrefix(name, `*`)
		ptrType = name
	} else if isSlice || isMap {
		rawType = name
		ptrType = name
	} else {
		rawType = name
		ptrType = `*` + name
	}

	return &TypeSpec{
		name:         name,
		element:      element,
		ptrType:      ptrType,
		rawType:      rawType,
		initArgStyle: initArgStyle,
		supportsLen:  supportsLen,
		zeroVal:      `nil`,
	}
}

func (ts *TypeSpec) InitializerArgumentStyle(ias InitializerArgumentStyle) *TypeSpec {
	ts.initArgStyle = ias
	return ts
}

func (ts *TypeSpec) ZeroVal(s string) *TypeSpec {
	ts.zeroVal = s
	return ts
}

// IsInterface should be set to true if the type is an interface.
// When this is true, the decoding logic generated changes.
// See also `InterfaceDecoder`
func (ts *TypeSpec) IsInterface(b bool) *TypeSpec {
	ts.isInterface = b
	return ts
}

func (ts *TypeSpec) GetIsInterface() bool {
	return ts.isInterface
}

// InterfaceDecoder should be set to the name of the function that
// can take a `[]byte` variable and return a value assignable to
// the type. For example a type specified as below
//
//	schema.Type(`mypkg.Interface`).InterfaceDecoder(`mypkg.Parse`)
//
// may produce code resembling
//
//	var val mypkg.Interface
//	var err error
//	val, err = mypkg.Parse(src) // src is []byte
//
// This value is not automatically assigned. Therefore you will
// laways need to specify this if you are referring to an interface
func (ts *TypeSpec) InterfaceDecoder(s string) *TypeSpec {
	ts.interfaceDecoder = s
	return ts
}

func (ts *TypeSpec) GetInterfaceDecoder() string {
	return ts.interfaceDecoder
}

// GetValue specifies that this type implements the `GetValue` method.
// The `GetValue` method must return a single element, which represents
// the apparent (user-facing) type of the field.
//
// For example, if you are storing `time.Time` as `mypkg.EpochTime`,
// and you want users to get `time.Time` values from the accessor
// (instead of `mypkg.EpochTime`), you will want to implement a
// `GetValue() time.Time` method, and make sure that the TypeSpec
// recognizes its existance.
//
// By default the method name for this method is `GetValue`, but
// you will be able to change it by setting a value with the `GetValueMethodName`.
// Calling `GetValue(true)` is equivalent to `GetValueMethodName("GetValue")`
func (ts *TypeSpec) GetValue(b bool) *TypeSpec {
	if b {
		ts.GetValueMethodName(defaultGetValueMethodName)
	} else {
		ts.GetValueMethodName("")
	}
	return ts
}

// GetValueMethodName sets the name of the method that fulfills the
// `GetValue` semantics. Set to the empty string if you would like to
// indicate that the type does not implement the `GetValue` interface.
func (ts *TypeSpec) GetValueMethodName(s string) *TypeSpec {
	ts.getValueMethodName = s
	return ts
}

// AcceptValue specifies that this type implements the `AcceptValue` method.
// The `AcceptValue` method must take a single `interface{}` argument, and
// set the internal value from the given argument, which could be anything
// that can either be accepted from JSON source, or from a user attempting
// to set a value to a field of this type.
//
// For example, if you are storing `time.Time` as `mypkg.EpochTime`,
// and you want users to set `time.Time` values via the Builder
// (instead of `mypkg.EpochTime`), you will want to implement a
// `AcceptValue(interface{}) error` method, and implement it such that
// the value of given `interface{}` is properly set to the internal
// representation of `mypkg.EpochTime`.
//
// Similarly, if this `mypkg.EpochTime` field is represented as an integer
// in the JSON source, you might want to handle that as well.
//
// By default the method name for this method is `AcceptValue`, but
// you will be able to change it by setting a value with the `AcceptValueMethodName`.
// Calling `AcceptValue(true)` is equivalent to `AcceptValueMethodName("AcceptValue")`
func (ts *TypeSpec) AcceptValue(b bool) *TypeSpec {
	if b {
		ts.AcceptValueMethodName(defaultAcceptValueMethodName)
	} else {
		ts.AcceptValueMethodName("")
	}
	return ts
}

// AcceptValueMethodName sets the name of the method that fulfills the
// `AcceptValue` semantics. Set to the empty string if you would like to
// indicate that the type does not implement the `AcceptValue` interface.
func (ts *TypeSpec) AcceptValueMethodName(s string) *TypeSpec {
	ts.acceptValueMethodName = s
	return ts
}

func (ts *TypeSpec) ApparentType(s string) *TypeSpec {
	ts.apparentType = s
	return ts
}

func (ts *TypeSpec) GetName() string {
	return ts.name
}

// GetGetValueMethodName returns the name of the `GetValue` method.
func (ts *TypeSpec) GetGetValueMethodName() string {
	return ts.getValueMethodName
}

// GetAcceptValueMethodName returns the name of the `AcceptValue` method.
func (ts *TypeSpec) GetAcceptValueMethodName() string {
	return ts.acceptValueMethodName
}

func (ts *TypeSpec) GetZeroVal() string {
	return ts.zeroVal
}

func (ts *TypeSpec) GetApparentType() string {
	typ := ts.apparentType
	if typ == "" {
		typ = ts.name
	}
	return typ
}

func (ts *TypeSpec) GetElement() string {
	return ts.element
}

func (ts *TypeSpec) GetSupportsLen() bool {
	return ts.supportsLen
}

func (ts *TypeSpec) SupportsLen(b bool) *TypeSpec {
	ts.supportsLen = b
	return ts
}

// PointerType specifies the "indirect" type of a field. The fields
// are stored as _pointers_ to the actual type, so for most types
// we simply prepend a `*` to the type. For example for a `string`
// type, the indirect type would be `*string`, whereas for `*Foo`
// type, we just use `*Foo` as the indirect type. But for cases when
// you would like to store an interface, for example, you might
// want to avoid prepending the `*` by explicitly specifying the
// name of the indirect type.
func (ts *TypeSpec) PointerType(s string) *TypeSpec {
	ts.ptrType = s
	return ts
}

func (ts *TypeSpec) RawType(s string) *TypeSpec {
	ts.rawType = s
	return ts
}

func (ts *TypeSpec) Element(s string) *TypeSpec {
	ts.element = s
	return ts
}

func (ts *TypeSpec) SliceStyleInitializerArgument() bool {
	return ts.initArgStyle == InitializerArgumentAsSlice
}

// FieldSpec represents a field that belongs to a particular schema.
type FieldSpec struct {
	required       bool
	name           string
	typ            *TypeSpec
	typName        string
	unexportedName string
	json           string
	comment        string
	extension      bool
	extra          map[string]interface{}
	constant       *string
}

var typInfoType = reflect.TypeOf((*TypeSpec)(nil))

func Field(name string, typ interface{}) *FieldSpec {
	// name must be an exported type
	if len(name) <= 0 || !unicode.IsUpper(rune(name[0])) {
		panic(fmt.Sprintf("schema fields must be provided an exported name: (%q is invalid)", name))
	}

	f := &FieldSpec{
		name:  name,
		extra: make(map[string]interface{}),
	}
	if typ == nil {
		panic("schema.Field must receive a non-nil second parameter")
	}

	// typ can be either a real type, or an instance of sketch.CustomType
	if ti, ok := typ.(*TypeSpec); ok {
		f.typ = ti
	} else {
		f.typ = Type(typ)
	}
	return f
}

func (f *FieldSpec) Extra(name string, value interface{}) *FieldSpec {
	f.extra[name] = value
	return f
}

func (f *FieldSpec) GetExtra(name string) interface{} {
	return f.extra[name]
}

func (f *FieldSpec) Required(b bool) *FieldSpec {
	f.required = b
	return f
}

func (f *FieldSpec) GetRequired() bool {
	return f.required
}

// String creates a new field with the given name and a string type
func String(name string) *FieldSpec {
	return Field(name, ``)
}

// Int creates a new field with the given name and a int type
func Int(name string) *FieldSpec {
	return Field(name, int(0))
}

// Bool creates a new field with the given name and a bool type
func Bool(name string) *FieldSpec {
	return Field(name, true)
}

// ByteSliceType represents a `[]byte` type. Since `sketch` mostly works with JSON,
// it needs to handle `[]byte` fields being encoded/decoded with base64 encoding
// transparently. Therefore unless the default behavior for `encoding/json`
// already work for you, you may need tweaking. The `ByteSliceType` uses
// `byteslice.Type` internally, which allows the user to specify the
// base64 encoding.
var ByteSliceType = Type(byteslice.Type{}).
	ApparentType(`[]byte`).
	AcceptValue(true).
	GetValueMethodName(`Bytes`).
	ZeroVal(`[]byte(nil)`)

var NativeByteSliceType = Type([]byte(nil)).
	ApparentType(`[]byte`).
	PointerType(`[]byte`).
	RawType(`[]byte`)

// ByteSlice creates a new field with the given name and a []byte type
func ByteSlice(name string) *FieldSpec {
	return Field(name, ByteSliceType)
}

func (f *FieldSpec) GetName() string {
	return f.name
}

func (f *FieldSpec) GetType() *TypeSpec {
	return f.typ
}

// Unexported specifies the unexported name for this field.
// If unspecified, the name of the field is automatically
// converted into a camel-case string with the first phrase
// being lower cased
func (f *FieldSpec) Unexported(s string) *FieldSpec {
	f.unexportedName = s
	return f
}

// JSON specifies the JSON field name. If unspecified, the
// unexported name is used.
func (f *FieldSpec) JSON(s string) *FieldSpec {
	f.json = s
	return f
}

func (f *FieldSpec) GetUnexportedName() string {
	if f.unexportedName == "" {
		f.unexportedName = xstrings.Camel(f.name, xstrings.WithLowerCamel(true))
	}
	return f.unexportedName
}

func (f *FieldSpec) Comment(s string) *FieldSpec {
	f.comment = s
	return f
}

func (f *FieldSpec) GetComment() string {
	return f.comment
}

func (f *FieldSpec) GetJSON() string {
	if f.json == "" {
		f.json = f.GetUnexportedName()
	}
	return f.json
}

func (ts *TypeSpec) GetPointerType() string {
	return ts.ptrType
}

func (ts *TypeSpec) GetRawType() string {
	return ts.rawType
}

// SExtension declares the field as an extension, and not part of the object
// as defined by the JSON representation. That is to say, this field
// exist in the Go struct, but not in the JSON structures that it
// serizlizes to or deserializes from.
//
// Fields defined as extensions are expected to be _internal_ to the object.
// They are not exposed by either Get/Set, and do not get any sort of accessors.
func (f *FieldSpec) IsExtension(b bool) *FieldSpec {
	f.extension = b
	return f
}

// GetIsExtension returns true if this field is an extension, and not
// part of the object per se. You will need to declare methods to
// get/set and/or otherwise work this variable by yourself
func (f *FieldSpec) GetIsExtension() bool {
	return f.extension
}

func (f *FieldSpec) GetKeyName(object Interface) string {
	return object.GetKeyName(f.GetName())
}

// ConstantValue sets the string value that should be used
// when fetching this field. When ConstantValue is specified,
// calling `Set` on this field would be a no-op (no error
// is returned)
func (f *FieldSpec) ConstantValue(s string) *FieldSpec {
	f.constant = &s
	return f
}

func (f *FieldSpec) GetIsConstant() bool {
	return f.constant != nil
}

func (f *FieldSpec) GetConstantValue() string {
	return *(f.constant)
}
