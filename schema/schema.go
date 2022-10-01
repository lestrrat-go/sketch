package schema

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/lestrrat-go/xstrings"
)

// InitializerArgumentStyle is used when you would like to override
// the default behavior and specify that a pseudo-TypeInfo (those types
// that are not constructed from actual Go objects)'s Builder methods
// should take a variadic form vs singular form.
type InitializerArgumentStyle int

const (
	InitializerArgumentAsSingleArg = iota
	InitializerArgumentAsSlice
)

// Interface exists to provide an abstraction for multiple
// schema objects that embed schema.Base object in the
// intermediate program that sketch produces. Users of sketch
// generally need not worry about this interface.
type Interface interface {
	Name() string
	Package() string
	Fields() []*Field
	Comment() string
	KeyNamePrefix() string
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

// GenerateMethod should return true if the given method is allowed to be
// generated. The argument consists of a prefix (e.g. "object." or "builder.")
// followed by the actual method name.
//
// By default all methods are allowed. Users may configure this on a per-object
// basis by providing their own `GenerateMethod` method.
func (b Base) GenerateMethod(s string) bool {
	m, ok := b.Variables["DefaultGenerateMethod"]
	if !ok {
		return true
	}

	if m, ok := m.(func(string) bool); ok {
		return m(s)
	}
	return true
}

func (b Base) MethodName(s string) string {
	v, ok := b.Variables["DefaultMethodNames"]
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

// CloneResultType returns the name of the type that the `Clone` method
// returns. Normally this is set to the pointer to the object, but
// sometimes you may need to return an interface.
func (b Base) CloneResultType() string {
	return b.StringVar(`DefaultCloneResultType`)
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

// Fields returns the list of fields that should be associated with the
// schema object. User usually must
func (Base) Fields() []*Field {
	return []*Field(nil)
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

// TypeInfo is used to store information about a type, and contains
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
// The `TypeInfo` object is meant to store the storage type, but
// you can associate the corresponding apparent type via the
// `ApparentType` method.
type TypeInfo struct {
	name             string
	element          string
	apparentType     string
	implementsGet    bool
	implementsAccept bool
	indirectType     string
	initArgStyle     InitializerArgumentStyle
	supportsLen      bool
	zeroVal          string
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

// TypeInfoFrom creates a new TypeInfo from a piece of Go data
// using reflection. It populates all the required fields by
// inspecting the structure, which you can override later.
func TypeInfoFrom(v interface{}) *TypeInfo {
	rv := reflect.TypeOf(v)

	typ := typeName(rv)

	var indirectType string
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Interface, reflect.Array:
		indirectType = typ
	default:
		indirectType = `*` + typ
	}

	var implementsGet bool
	if m, ok := rv.MethodByName(`Get`); ok {
		implementsGet = m.Type.NumIn() == 1 && m.Type.NumOut() == 1
	}

	var implementsAccept bool
	if m, ok := rv.MethodByName(`Accept`); ok {
		implementsAccept =
			m.Type.NumIn() == 2 &&
				m.Type.In(1) == typInterface &&
				m.Type.NumOut() == 1 &&
				m.Type.Out(0) == typError
	}

	// If the type implements the Get() xxxx interface, we can deduce
	// the apparent type from the return type
	apparentType := rv
	if implementsGet {
		m, _ := rv.MethodByName(`Get`)
		apparentType = m.Type.Out(0)
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

	return &TypeInfo{
		name:             typ,
		apparentType:     typeName(apparentType),
		element:          element,
		implementsGet:    implementsGet,
		implementsAccept: implementsAccept,
		initArgStyle:     initArgStyle,
		indirectType:     indirectType,
		supportsLen:      supportsLen,
		zeroVal:          fmt.Sprintf("%#v", reflect.Zero(rv)),
	}
}

// Type creates a TypeInfo from a string name.
//
// If you are allowed to include the struct into the schema code, you
// should not be using this function. Only use this function when
// you either have to refer to objects that you are about to generate
// using sketch, or for objects that you cannot import because of
// cyclic dependency, etc.
//
// Unlike `TypeInfoFrom`, this constructor only takes the name of the
// type and otherwise has no other information. Therefore it assumes
// many things, and you will have to set many parameers manually.
//
// The defualt zero value is assumed to be `nil`
//
// If the name starts with a `[]`, then `IsSlice()` is automatically set to true
// If the name starts with a `map[`, then `IsMap()` is automatically set to true
func Type(name string) *TypeInfo {
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

	var indirectType string
	if strings.HasPrefix(name, `*`) || (isSlice || isMap) {
		indirectType = name
	} else {
		indirectType = `*` + name
	}

	return &TypeInfo{
		name:         name,
		element:      element,
		indirectType: indirectType,
		initArgStyle: initArgStyle,
		supportsLen:  supportsLen,
		zeroVal:      `nil`,
	}
}

func (ti *TypeInfo) InitializerArgumentStyle(ias InitializerArgumentStyle) *TypeInfo {
	ti.initArgStyle = ias
	return ti
}

func (ti *TypeInfo) ZeroVal(s string) *TypeInfo {
	ti.zeroVal = s
	return ti
}

func (ti *TypeInfo) ImplementsGet(b bool) *TypeInfo {
	ti.implementsGet = b
	return ti
}

func (ti *TypeInfo) ImplementsAccept(b bool) *TypeInfo {
	ti.implementsAccept = b
	return ti
}

func (ti *TypeInfo) ApparentType(s string) *TypeInfo {
	ti.apparentType = s
	return ti
}

func (ti *TypeInfo) GetName() string {
	return ti.name
}

// GetImplementsGet returns true if the object contains a method named `Get`
// which returns a single return value. The return value is expected
// to be the ApparentType
func (ti *TypeInfo) GetImplementsGet() bool {
	return ti.implementsGet
}

func (ti *TypeInfo) GetImplementsAccept() bool {
	return ti.implementsAccept
}

func (ti *TypeInfo) GetZeroVal() string {
	return ti.zeroVal
}

func (ti *TypeInfo) GetApparentType() string {
	typ := ti.apparentType
	if typ == "" {
		typ = ti.name
	}
	return typ
}

func (ti *TypeInfo) GetElement() string {
	return ti.element
}

func (ti *TypeInfo) GetSupportsLen() bool {
	return ti.supportsLen
}

func (ti *TypeInfo) SupportsLen(b bool) *TypeInfo {
	ti.supportsLen = b
	return ti
}

// IndirectType specifies the "indirect" type of a field. The fields
// are stored as _pointers_ to the actual type, so for most types
// we simply prepend a `*` to the type. For example for a `string`
// type, the indirect type would be `*string`, whereas for `*Foo`
// type, we just use `*Foo` as the indirect type. But for cases when
// you would like to store an interface, for example, you might
// want to avoid prepending the `*` by explicitly specifying the
// name of the indirect type.
func (ti *TypeInfo) IndirectType(s string) *TypeInfo {
	ti.indirectType = s
	return ti
}

func (ti *TypeInfo) Element(s string) *TypeInfo {
	ti.element = s
	return ti
}

func (ti *TypeInfo) SliceStyleInitializerArgument() bool {
	return ti.initArgStyle == InitializerArgumentAsSlice
}

type Field struct {
	required       bool
	name           string
	typ            *TypeInfo
	typName        string
	unexportedName string
	json           string
	indirectType   string
	apparentType   string
	comment        string
	extension      bool
}

var typInfoType = reflect.TypeOf((*TypeInfo)(nil))

func NewField(name string, typ interface{}) *Field {
	// name must be an exported type
	if len(name) <= 0 || !unicode.IsUpper(rune(name[0])) {
		panic(fmt.Sprintf("schema fields must be provided an exported name: (%q is invalid)", name))
	}

	f := &Field{name: name}
	if typ == nil {
		panic("schema.NewField must receive a non-nil second parameter")
	}

	// typ can be either a real type, or an instance of sketch.CustomType
	if ti, ok := typ.(*TypeInfo); ok {
		f.typ = ti
	} else {
		f.typ = TypeInfoFrom(typ)
	}
	return f
}

func (f *Field) Required(b bool) *Field {
	f.required = b
	return f
}

func (f *Field) GetRequired() bool {
	return f.required
}

func String(name string) *Field {
	return NewField(name, ``)
}

func Int(name string) *Field {
	return NewField(name, int(0))
}

func Bool(name string) *Field {
	return NewField(name, true)
}

func (f *Field) GetName() string {
	return f.name
}

func (f *Field) GetType() *TypeInfo {
	return f.typ
}

// Unexported specifies the unexported name for this field.
// If unspecified, the name of the field is automatically
// converted into a camel-case string with the first phrase
// being lower cased
func (f *Field) Unexported(s string) *Field {
	f.unexportedName = s
	return f
}

// JSON specifies the JSON field name. If unspecified, the
// unexported name is used.
func (f *Field) JSON(s string) *Field {
	f.json = s
	return f
}

func (f *Field) GetUnexportedName() string {
	if f.unexportedName == "" {
		f.unexportedName = xstrings.Camel(f.name, xstrings.WithLowerCamel(true))
	}
	return f.unexportedName
}

func (f *Field) Comment(s string) *Field {
	f.comment = s
	return f
}

func (f *Field) GetComment() string {
	return f.comment
}

func (f *Field) GetJSON() string {
	if f.json == "" {
		f.json = f.GetUnexportedName()
	}
	return f.json
}

func (ti *TypeInfo) GetIndirectType() string {
	return ti.indirectType
}

// SExtension declares the field as an extension, and not part of the object
// as defined by the JSON representation. That is to say, this field
// exist in the Go struct, but not in the JSON structures that it
// serizlizes to or deserializes from.
//
// Fields defined as extensions are expected to be _internal_ to the object.
// They are not exposed by either Get/Set, and do not get any sort of accessors.
func (f *Field) IsExtension(b bool) *Field {
	f.extension = b
	return f
}

// GetIsExtension returns true if this field is an extension, and not
// part of the object per se. You will need to declare methods to
// get/set and/or otherwise work this variable by yourself
func (f *Field) GetIsExtension() bool {
	return f.extension
}

func (f *Field) GetKeyName(object interface{ KeyNamePrefix() string }) string {
	var b strings.Builder

	// If the object wants per-object prefix, do it. Otherwise leave it empty
	if prefix := object.KeyNamePrefix(); prefix != "" {
		b.WriteString(prefix)
	}
	b.WriteString(f.GetName())
	b.WriteString(`Key`)
	return b.String()
}
