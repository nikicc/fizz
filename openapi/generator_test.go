package openapi

import (
	"encoding/json"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/loopfz/gadgeto/tonic"
	"github.com/stretchr/testify/assert"
	"github.com/tjarratt/babble"
)

var genConfig = &SpecGenConfig{
	ValidatorTag:      tonic.ValidationTag,
	PathLocationTag:   tonic.PathTag,
	QueryLocationTag:  tonic.QueryTag,
	HeaderLocationTag: tonic.HeaderTag,
	EnumTag:           tonic.EnumTag,
	DefaultTag:        tonic.DefaultTag,
}

var rt = reflect.TypeOf

type (
	X struct {
		*X // ignored, recursive embedding
		*Y
		A string `validate:"required"`
		B *int
		C bool `deprecated:"true"`
		D []*Y
		E [3]*X
		F *X
		G *Y
		H map[int]*Y // ignored, unsupported keys type
	}
	Y struct {
		H float32   `validate:"required"`
		I time.Time `format:"date"`
		J *uint8    `deprecated:"oui"` // invalid value, interpreted as false
		K *Z        `validate:"required"`
		N struct {
			Na, Nb string
			Nc     time.Duration
		}
		l int // ignored
		M int `json:"-"`
	}
	Z map[string]*Y
)

func (*X) Type() string { return "XXX" }

// TestStructFieldName tests that the name of a
// struct field can be correcttly extracted.
func TestStructFieldName(t *testing.T) {
	type T struct {
		A  string `name:"A"`
		Ba string `name:""`
		AB string `name:"-"`
		B  struct{}
	}
	to := reflect.TypeOf(T{})

	assert.Equal(t, "A", fieldNameFromTag(to.Field(0), "name"))
	assert.Equal(t, "Ba", fieldNameFromTag(to.Field(1), "name"))
	assert.Equal(t, "", fieldNameFromTag(to.Field(2), "name"))
}

func TestAddTag(t *testing.T) {
	g := gen(t)

	// Append nil tag to ensure sort works
	// works even with non-addressable values.
	g.api.Tags = append(g.api.Tags, nil)

	g.AddTag("", "Test routes")
	assert.Len(t, g.API().Tags, 1)

	g.AddTag("Test", "Test routes")
	assert.Len(t, g.API().Tags, 2)

	tag := g.API().Tags[1]
	assert.NotNil(t, tag)
	assert.Equal(t, tag.Name, "Test")
	assert.Equal(t, tag.Description, "Test routes")

	// Update tag description.
	g.AddTag("Test", "Routes test")
	assert.Equal(t, tag.Description, "Routes test")

	// Add other tag, check sort order.
	g.AddTag("A", "")
	assert.Len(t, g.API().Tags, 3)
	tag = g.API().Tags[1]
	assert.Equal(t, "A", tag.Name)
}

// TestSchemaFromPrimitiveType tests that a schema
// can be created given a primitive input type.
func TestSchemaFromPrimitiveType(t *testing.T) {
	g := gen(t)

	// Use a pointer to primitive type to test
	// pointer dereference and property nullable.
	schema := g.newSchemaFromType(rt(new(int64)))

	// Ensure it is an inlined schema before
	// accessing properties for assertions.
	if schema.Schema == nil {
		t.Error("expected an inlined schema, got a schema reference")
	}
	assert.Equal(t, "integer", schema.Type)
	assert.Equal(t, "int64", schema.Format)
	assert.True(t, schema.Nullable)
}

// TestSchemaFromUnsupportedType tests that a schema
// cannot be created given an unsupported input type.
func TestSchemaFromUnsupportedType(t *testing.T) {
	g := gen(t)

	// Test with nil input.
	schema := g.newSchemaFromType(nil)
	assert.Nil(t, schema)

	// Test with unsupported input.
	schema = g.newSchemaFromType(rt(func() {}))
	assert.Nil(t, schema)
	assert.Len(t, g.Errors(), 1)
	assert.Implements(t, (*error)(nil), g.Errors()[0])
	assert.NotEmpty(t, g.Errors()[0])
}

// TestSchemaFromMapWithUnsupportedKeys tests that a
// schema cannot be created given a map type with
// unsupported key's type.
func TestSchemaFromMapWithUnsupportedKeys(t *testing.T) {
	g := gen(t)

	schema := g.newSchemaFromType(rt(map[int]string{}))
	assert.Nil(t, schema)
	assert.Len(t, g.Errors(), 1)
	assert.Implements(t, (*error)(nil), g.Errors()[0])
	assert.NotEmpty(t, g.Errors()[0].Error())
}

// TestSchemaFromComplex tests that a schema
// can be created from a complex type.
func TestSchemaFromComplex(t *testing.T) {
	g := gen(t)
	g.UseFullSchemaNames(false)

	sor := g.newSchemaFromType(rt(new(X)))
	assert.NotNil(t, sor)

	b, err := json.Marshal(sor)
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, `{"$ref":"#/components/schemas/XXX"}`, string(b))

	schema := g.resolveSchema(sor)
	assert.NotNil(t, schema)

	actual, err := json.Marshal(schema)
	if err != nil {
		t.Error(err)
	}
	// see testdata/X.json.
	expected, err := ioutil.ReadFile("../testdata/schemas/X.json")
	if err != nil {
		t.Error(err)
	}
	m, err := diffJSON(actual, expected)
	if err != nil {
		t.Error(err)
	}
	if !m {
		t.Error("expected json outputs to be equal")
	}

	sor = g.API().Components.Schemas["Y"]
	schema = g.resolveSchema(sor)
	assert.NotNil(t, schema)

	actual, err = json.Marshal(schema)
	if err != nil {
		t.Error(err)
	}
	// see testdata/Y.json.
	expected, err = ioutil.ReadFile("../testdata/schemas/Y.json")
	if err != nil {
		t.Error(err)
	}
	m, err = diffJSON(actual, expected)
	if err != nil {
		t.Error(err)
	}
	if !m {
		t.Error("expected json outputs to be equal")
	}
}

// TestNewSchemaFromStructErrors tests the errors
// case of generation of a schema from a struct.
func TestNewSchemaFromStructErrors(t *testing.T) {
	g := gen(t)

	// Invalid input.
	sor := g.newSchemaFromStruct(reflect.TypeOf(new(string)))
	assert.Nil(t, sor)
}

// TestNewSchemaFromStructFieldErrors tests the errors
// case of generation of a schema from a struct field.
func TestNewSchemaFromStructFieldErrors(t *testing.T) {
	g := gen(t)

	type T struct {
		A string `validate:"required" default:"foobar"`
		B int    `default:"foobaz"`
		C int    `enum:"a,1,c"`
	}
	typ := reflect.TypeOf(T{})

	// Field A is required and has a default value.
	sor := g.newSchemaFromStructField(typ.Field(0), true, "A", typ)
	assert.NotNil(t, sor)
	assert.Len(t, g.Errors(), 1)
	assert.Implements(t, (*error)(nil), g.Errors()[0])
	assert.NotEmpty(t, g.Errors()[0].Error())

	// Field B has default value that cannot be converted to type's type.
	sor = g.newSchemaFromStructField(typ.Field(1), false, "B", typ)
	assert.NotNil(t, sor)
	assert.Len(t, g.Errors(), 2)
	assert.Implements(t, (*error)(nil), g.Errors()[1])
	assert.NotEmpty(t, g.Errors()[1].Error())

	// Field C has enum values that cannot be converted to type's type.
	sor = g.newSchemaFromStructField(typ.Field(2), true, "C", typ)
	assert.NotNil(t, sor)
	// it generates two errors, one per value
	// that cannot be converted, here "a" and "b".
	assert.Len(t, g.Errors(), 4)
	assert.NotEmpty(t, g.Errors()[2].Error())
	assert.NotEmpty(t, g.Errors()[3].Error())
}

func diffJSON(a, b []byte) (bool, error) {
	var j, j2 interface{}
	if err := json.Unmarshal(a, &j); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &j2); err != nil {
		return false, err
	}
	return reflect.DeepEqual(j2, j), nil
}

// TestAddOperation tests that an operation can be added
// and generates the according specification.
func TestAddOperation(t *testing.T) {
	type InEmbed struct {
		D int     `query:"xd" enum:"1,2,3" default:"1"`
		E bool    `query:"e"`
		F *string `json:"f" description:"This is F"`
		G []byte  `validate:"required"`
		H uint16  `binding:"-"`
	}
	type In struct {
		*In // ignored
		*InEmbed

		A int       `path:"a" description:"This is A" deprecated:"oui"`
		B time.Time `query:"b" validate:"required" description:"This is B"`
		C string    `header:"X-Test-C" description:"This is C" default:"test"`
		d int       // ignored, unexported
		E int       `path:"a"` // ignored, duplicate of A
		F *string   `json:"f"` // ignored, duplicate of F in InEmbed
	}
	type CustomError struct{}

	var Header string

	g := gen(t)
	g.UseFullSchemaNames(false)

	path := "/test/:A"

	infos := &OperationInfo{
		ID:                "CreateTest",
		StatusCode:        201,
		StatusDescription: "OK",
		Summary:           "ABC",
		Description:       "XYZ",
		Deprecated:        true,
		Responses: []*OperationReponse{
			&OperationReponse{
				Code:        "400",
				Description: "Bad Request",
				Model:       CustomError{},
			},
			&OperationReponse{
				Code: "500",
			},
		},
		Headers: []*ResponseHeader{
			&ResponseHeader{
				Name:        "X-Test-Header",
				Description: "Test header",
				Model:       Header,
			},
			&ResponseHeader{
				Name:        "X-Test-Header-Alt",
				Description: "Test header alt",
			},
		},
	}
	err := g.AddOperation(path, "POST", "Test", reflect.TypeOf(&In{}), reflect.TypeOf(Z{}), infos)
	if err != nil {
		t.Error(err)
	}
	assert.Len(t, g.API().Paths, 1)

	item, ok := g.API().Paths[rewritePath(path)]
	if !ok {
		t.Errorf("expected to found item for path %s", path)
	}
	assert.NotNil(t, item.POST)

	actual, err := json.Marshal(item.POST)
	if err != nil {
		t.Error(err)
	}
	// see testdata/op.json.
	expected, err := ioutil.ReadFile("../testdata/schemas/op.json")
	if err != nil {
		t.Error(err)
	}
	m, err := diffJSON(actual, expected)
	if err != nil {
		t.Error(err)
	}
	if !m {
		t.Error("expected json outputs to be equal")
	}

	// Try to add the operation again with the same
	// identifier. Expected to fail.
	err = g.AddOperation(path, "POST", "Test", reflect.TypeOf(&In{}), reflect.TypeOf(Z{}), infos)
	assert.NotNil(t, err)

	// Add an operation with a bad input type.
	err = g.AddOperation("/", "GET", "", reflect.TypeOf(new(string)), nil, nil)
	assert.NotNil(t, err)
}

// TestTypeName tests that the name of a type
// can be discovered.
func TestTypeName(t *testing.T) {
	g, err := NewGenerator(genConfig)
	if err != nil {
		t.Error(err)
	}
	// TypeNamer interface.
	name := g.typeName(rt(new(X)))
	assert.Equal(t, "XXX", name)

	// Override. This has precedence
	// over the interface implementation.
	err = g.OverrideTypeName(rt(new(X)), "")
	assert.NotNil(t, err)
	assert.Equal(t, "XXX", g.typeName(rt(new(X))))

	g.OverrideTypeName(rt(new(X)), "xXx")
	assert.Equal(t, "xXx", g.typeName(rt(X{})))

	err = g.OverrideTypeName(rt(new(X)), "YYY")
	assert.NotNil(t, err)

	// Default.
	assert.Equal(t, "OpenapiY", g.typeName(rt(new(Y))))
	g.UseFullSchemaNames(false)
	assert.Equal(t, "Y", g.typeName(rt(Y{})))

	// Unnamed type.
	assert.Equal(t, "", g.typeName(rt(struct{}{})))
}

// TestSetInfo tests that the informations
// of the spec can be modified.
func TestSetInfo(t *testing.T) {
	g := gen(t)

	infos := &Info{
		Description: "Test",
	}
	g.SetInfo(infos)

	assert.NotNil(t, g.API().Info)
	assert.Equal(t, infos, g.API().Info)
}

// TestSetOperationByMethod tests that an operation
// is added to a path item accordingly to the given
// HTTP method.
func TestSetOperationByMethod(t *testing.T) {
	babbler := babble.NewBabbler()

	pi := &PathItem{}
	for method, ptr := range map[string]**Operation{
		"GET":     &pi.GET,
		"POST":    &pi.POST,
		"PUT":     &pi.PUT,
		"PATCH":   &pi.PATCH,
		"DELETE":  &pi.DELETE,
		"HEAD":    &pi.HEAD,
		"OPTIONS": &pi.OPTIONS,
		"TRACE":   &pi.TRACE,
	} {
		desc := babbler.Babble()
		op := &Operation{
			Description: desc,
		}
		setOperationBymethod(pi, op, method)
		assert.Equal(t, op, *ptr)
		assert.Equal(t, desc, (*ptr).Description)
	}
}

// TestSetOperationResponseError tests the various error
// cases that can occur while adding a response to an op.
func TestSetOperationResponseError(t *testing.T) {
	g := gen(t)
	op := &Operation{
		Responses: make(Responses),
	}
	err := g.setOperationResponse(op, reflect.TypeOf(new(string)), "200", "application/json", "", nil)
	assert.Nil(t, err)

	// Add another response with same code.
	err = g.setOperationResponse(op, reflect.TypeOf(new(int)), "200", "application/xml", "", nil)
	assert.NotNil(t, err)

	// Add invalid response code that cannot
	// be converted to an integer.
	err = g.setOperationResponse(op, reflect.TypeOf(new(bool)), "two-hundred", "", "", nil)
	assert.NotNil(t, err)
}

// TestSetOperationParamsError tests the various error
// cases that can occur while adding parameters to an op.
func TestSetOperationParamsError(t *testing.T) {
	g := gen(t)
	op := &Operation{}

	// Use invalid input type for parameters.
	typ := reflect.TypeOf([]string{})
	err := g.setOperationParams(op, typ, typ, false)
	assert.NotNil(t, err)
}

// TestParamLocationConflict tests that using conflicting
// locations in the tag of a parameter throws an error.
func TestParamLocationConflict(t *testing.T) {
	type T struct {
		A string `path:"a" query:"b"`
	}
	g := gen(t)

	_, err := g.paramLocation(reflect.TypeOf(T{}).Field(0), reflect.TypeOf(T{}))
	assert.NotNil(t, err)
}

// TestNewGenWithoutConfig tests that creating a
// new generator without config fails.
func TestNewGenWithoutConfig(t *testing.T) {
	_, err := NewGenerator(nil)
	assert.NotNil(t, err)
}

func gen(t *testing.T) *Generator {
	g, err := NewGenerator(genConfig)
	if err != nil {
		t.Error(err)
	}
	g.UseFullSchemaNames(false)

	return g
}
