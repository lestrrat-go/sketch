package schema

import (
	"fmt"
	"reflect"
	"unicode"

	"github.com/lestrrat-go/xstrings"
)

// Interface exists to provide an abstraction for multiple
// schema objects that embed schema.Base object in the
// intermediate program that sketch produces
type Interface interface {
	Name() string
	Package() string
	Fields() []*Field
	Comment() string
}

type Base struct {
	DefaultPkg                string
	DefaultName               string
	DefaultGenerateHasMethods bool
	DefaultGenerateBuilders   bool
}

var _ Interface = &Base{} // sanity

func (b Base) GenerateHasMethods() bool {
	return b.DefaultGenerateHasMethods
}

func (b Base) GenerateBuilders() bool {
	return b.DefaultGenerateBuilders
}

func (b Base) Name() string {
	return b.DefaultName
}

func (b Base) Package() string {
	return b.DefaultPkg
}

func (Base) Fields() []*Field {
	return []*Field(nil)
}

func (Base) Comment() string {
	return ""
}

// TypeInfo is used to store information about a type.
type TypeInfo struct {
	kind           reflect.Kind
	implementsGet  bool
	indirectType   string
	isMap          bool
	isSlice        bool
	name           string
	userFacingType string
	zeroVal        string
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

	kind := rv.Kind()

	var implementsGet bool
	m, ok := rv.MethodByName(`Get`)
	if ok {
		implementsGet = m.Type.NumIn() == 0 && m.Type.NumOut() == 1
	}

	return &TypeInfo{
		name:           typ,
		indirectType:   indirectType,
		implementsGet:  implementsGet,
		isMap:          kind == reflect.Map,
		isSlice:        kind == reflect.Slice,
		userFacingType: typ,
		zeroVal:        fmt.Sprintf("%#v", reflect.Zero(rv)),
	}
}

// Type creates a TypeInfo
func Type(name string) *TypeInfo {
	return &TypeInfo{
		name:    name,
		zeroVal: `nil`,
	}
}

func (ti *TypeInfo) ZeroVal(s string) *TypeInfo {
	ti.zeroVal = s
	return ti
}

func (ti *TypeInfo) ImplementsGet(b bool) *TypeInfo {
	ti.implementsGet = b
	return ti
}

func (ti *TypeInfo) UserFacingType(s string) *TypeInfo {
	ti.userFacingType = s
	return ti
}

func (ti *TypeInfo) IsMap(b bool) *TypeInfo {
	ti.isMap = b
	return ti
}

func (ti *TypeInfo) IsSlice(b bool) *TypeInfo {
	ti.isSlice = b
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

type Field struct {
	required       bool
	name           string
	typ            *TypeInfo
	typName        string
	unexportedName string
	json           string
	indirectType   string
	userFacingType string
	implementsGet  *bool
	comment        string
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

func (f *Field) GetName() string {
	return f.name
}

func (f *Field) Type(s string) *Field {
	f.typ.name = s
	return f
}

func (f *Field) GetType() string {
	return f.typ.name
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

func (f *Field) GetIndirectType() string {
	typ := f.typ.indirectType
	if typ == "" {
		if f.typ.isSlice || f.typ.isMap {
			return f.GetType()
		} else {
			return `*` + f.GetType()
		}
	}
	return typ
}

func (f *Field) IsMap() bool {
	return f.typ.isMap
}

func (f *Field) IsSlice() bool {
	return f.typ.isSlice
}

func (f *Field) ImplementsGet(b bool) *Field {
	f.typ.ImplementsGet(b)
	return f
}

// GetImplementsGet returns true if the object contains a method named `Get`
// which returns a single return value. The return value is expected
// to be the UserFacingType
func (f *Field) GetImplementsGet() bool {
	return f.typ.implementsGet
}

func (f *Field) GetZeroVal() string {
	return f.typ.zeroVal
}

func (f *Field) GetUserFacingType() string {
	return f.typ.userFacingType
}
