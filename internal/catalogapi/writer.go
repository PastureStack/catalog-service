package catalogapi

import (
	"encoding/json"
	"fmt"
	"reflect"

	client "github.com/PastureStack/catalog-service/internal/catalogclient"
	log "github.com/sirupsen/logrus"
)

func toMap(value interface{}) (map[string]interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func embedded(value interface{}, wanted reflect.Type) interface{} {
	current := reflect.ValueOf(value)
	for current.IsValid() && (current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface) {
		if current.IsNil() {
			return nil
		}
		current = current.Elem()
	}
	if !current.IsValid() || current.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < current.NumField(); i++ {
		fieldType := current.Type().Field(i)
		if fieldType.Anonymous && fieldType.Type == wanted {
			field := current.Field(i)
			if field.CanAddr() {
				return field.Addr().Interface()
			}
			copyValue := reflect.New(wanted)
			copyValue.Elem().Set(field)
			return copyValue.Interface()
		}
	}
	return nil
}

func collectionValue(value interface{}) *client.Collection {
	if collection, ok := value.(*client.Collection); ok {
		return collection
	}
	result, _ := embedded(value, reflect.TypeOf(client.Collection{})).(*client.Collection)
	return result
}

func resourceValue(value interface{}) *client.Resource {
	if resource, ok := value.(*client.Resource); ok {
		return resource
	}
	if resource, ok := value.(client.Resource); ok {
		return &resource
	}
	result, _ := embedded(value, reflect.TypeOf(client.Resource{})).(*client.Resource)
	return result
}

func resourceToMap(value interface{}, schemas *client.Schemas) (map[string]interface{}, error) {
	resource := resourceValue(value)
	if resource == nil {
		return nil, fmt.Errorf("value is not a Resource")
	}
	allFields, err := toMap(value)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	schema := schemas.Schema(resource.Type)
	for name, field := range allFields {
		if _, ok := schema.CheckField(name); ok {
			result[name] = field
		}
	}
	return result, nil
}

func collectionToMap(value interface{}, schemas *client.Schemas) (map[string]interface{}, []map[string]interface{}, error) {
	if collectionValue(value) == nil {
		return nil, nil, fmt.Errorf("value is not a Collection")
	}
	current := reflect.ValueOf(value)
	for current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface {
		current = current.Elem()
	}
	data := current.FieldByName("Data")
	resources := make([]map[string]interface{}, 0, data.Len())
	for i := 0; i < data.Len(); i++ {
		item := data.Index(i)
		for item.Kind() == reflect.Interface {
			item = item.Elem()
		}
		var raw interface{}
		if item.Kind() == reflect.Pointer {
			raw = item.Interface()
		} else if item.CanAddr() {
			raw = item.Addr().Interface()
		} else {
			raw = item.Interface()
		}
		resource, err := resourceToMap(raw, schemas)
		if err != nil {
			return nil, nil, err
		}
		resources = append(resources, resource)
	}
	result, err := toMap(value)
	if err != nil {
		return nil, nil, err
	}
	result["data"] = resources
	return result, resources, nil
}

func stringMap(value interface{}) map[string]string {
	result := map[string]string{}
	switch typed := value.(type) {
	case map[string]string:
		for key, item := range typed {
			result[key] = item
		}
	case map[string]interface{}:
		for key, item := range typed {
			result[key] = fmt.Sprint(item)
		}
	}
	return result
}

func ensureStringMap(data map[string]interface{}, name string) map[string]string {
	result := stringMap(data[name])
	data[name] = result
	return result
}

func setDefault(values map[string]string, name, value string) {
	if values[name] == "" {
		values[name] = value
	}
}

func (apiContext *APIContext) populateResource(resource map[string]interface{}) {
	resourceType := fmt.Sprint(resource["type"])
	resourceID := fmt.Sprint(resource["id"])
	links := ensureStringMap(resource, "links")
	if resourceType != "" && resourceID != "" {
		setDefault(links, selfLink, apiContext.URLBuilder.ReferenceByIDLink(resourceType, resourceID))
	}
	setDefault(links, selfLink, apiContext.URLBuilder.Current())
	ensureStringMap(resource, "actions")
}

func (apiContext *APIContext) Write(value interface{}) {
	var output map[string]interface{}
	var err error
	if collectionValue(value) != nil {
		var resources []map[string]interface{}
		output, resources, err = collectionToMap(value, apiContext.Schemas)
		if err == nil {
			links := ensureStringMap(output, "links")
			setDefault(links, selfLink, apiContext.URLBuilder.Current())
			if _, ok := output["type"]; !ok {
				output["type"] = "collection"
			}
			if len(resources) > 0 {
				if _, ok := output["resourceType"]; !ok {
					output["resourceType"] = resources[0]["type"]
				}
			}
			for _, resource := range resources {
				apiContext.populateResource(resource)
			}
		}
	} else {
		output, err = resourceToMap(value, apiContext.Schemas)
		if err == nil {
			apiContext.populateResource(output)
		}
	}
	if err != nil {
		log.WithError(err).Error("Failed to write API response")
		apiContext.responseWriter.WriteHeader(500)
		return
	}
	apiContext.responseWriter.Header().Set("X-API-Schemas", apiContext.URLBuilder.Collection("schema"))
	apiContext.responseWriter.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(apiContext.responseWriter)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		log.WithError(err).Error("Failed to encode API response")
	}
}
