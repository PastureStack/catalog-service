package catalogclient

import (
	"reflect"
	"strings"
)

type Schemas struct {
	Collection
	Data []Schema `json:"data,omitempty"`
}

func (schema *Schema) CheckField(name string) (Field, bool) {
	field, ok := schema.ResourceFields[name]
	return field, ok
}

func (schemas *Schemas) CheckSchema(name string) (Schema, bool) {
	for i := range schemas.Data {
		if strings.EqualFold(schemas.Data[i].ID, name) {
			return schemas.Data[i], true
		}
	}
	return Schema{}, false
}

func (schemas *Schemas) Schema(name string) Schema {
	schema, _ := schemas.CheckSchema(name)
	return schema
}

func typeToFields(valueType reflect.Type) map[string]Field {
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	result := map[string]Field{}
	if valueType.Kind() != reflect.Struct {
		return result
	}
	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if field.Anonymous {
			for name, inherited := range typeToFields(field.Type) {
				result[name] = inherited
			}
			continue
		}

		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = strings.ToLower(field.Name[:1]) + field.Name[1:]
		}

		kind := field.Type.Kind()
		if kind == reflect.Pointer {
			kind = field.Type.Elem().Kind()
		}
		schemaField := Field{}
		switch kind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			schemaField.Type = "int"
		case reflect.Bool:
			schemaField.Type = "bool"
		case reflect.Float32, reflect.Float64:
			schemaField.Type = "float"
		case reflect.String:
			schemaField.Type = "string"
		case reflect.Map:
			schemaField.Type = "map[string]"
		case reflect.Slice, reflect.Array:
			schemaField.Type = "array[string]"
		case reflect.Struct:
			schemaField.Type = field.Type.String()
		}
		if schemaField.Type != "" {
			result[jsonName] = schemaField
		}
	}
	return result
}

func (schemas *Schemas) AddType(name string, value interface{}) *Schema {
	schema := Schema{
		Resource:          Resource{ID: name, Type: "schema", Links: map[string]string{}},
		PluralName:        pluralName(name),
		ResourceFields:    typeToFields(reflect.TypeOf(value)),
		CollectionMethods: []string{"GET"},
		ResourceMethods:   []string{"GET"},
	}
	schemas.Data = append(schemas.Data, schema)
	return &schemas.Data[len(schemas.Data)-1]
}

func pluralName(name string) string {
	if name == "" {
		return ""
	}
	if strings.HasSuffix(name, "s") || strings.HasSuffix(name, "ch") || strings.HasSuffix(name, "x") {
		return name + "es"
	}
	return name + "s"
}
