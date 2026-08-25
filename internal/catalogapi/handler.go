package catalogapi

import (
	"net/http"

	client "github.com/PastureStack/catalog-service/internal/catalogclient"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func APIHandler(schemas *client.Schemas, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestWithContext, err := withAPIContext(writer, request, schemas)
		if err != nil {
			log.WithError(err).Error("Failed to create API context")
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		handler.ServeHTTP(writer, requestWithContext)
	})
}

func VersionsHandler(schemas *client.Schemas, versions ...string) http.Handler {
	return APIHandler(schemas, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		apiContext := GetAPIContext(request)
		collection := client.GenericCollection{
			Collection: client.Collection{Links: map[string]string{}},
			Data:       []interface{}{},
		}
		for index, version := range versions {
			collection.Data = append(collection.Data, client.Resource{
				ID: version, Type: "apiVersion",
				Links: map[string]string{selfLink: apiContext.URLBuilder.Version(version)},
			})
			if index == len(versions)-1 {
				collection.Links[latestLink] = apiContext.URLBuilder.Version(version)
			}
		}
		apiContext.Write(&collection)
	}))
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func VersionHandler(schemas *client.Schemas, version string) http.Handler {
	return APIHandler(schemas, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		apiContext := GetAPIContext(request)
		resource := client.Resource{ID: version, Type: "apiVersion", Links: map[string]string{}}
		for _, schema := range schemas.Data {
			if contains(schema.CollectionMethods, "GET") {
				resource.Links[schema.PluralName] = apiContext.URLBuilder.Collection(schema.ID)
			}
		}
		apiContext.Write(&resource)
	}))
}

func populateSchema(apiContext *APIContext, schema *client.Schema) {
	if contains(schema.CollectionMethods, "GET") {
		schema.Links["collection"] = apiContext.URLBuilder.Collection(schema.ID)
	}
}

func SchemasHandler(schemas *client.Schemas) http.Handler {
	return APIHandler(schemas, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		apiContext := GetAPIContext(request)
		copyValue := *schemas
		copyValue.Data = append([]client.Schema(nil), schemas.Data...)
		for index := range copyValue.Data {
			populateSchema(apiContext, &copyValue.Data[index])
		}
		apiContext.Write(&copyValue)
	}))
}

func SchemaHandler(schemas *client.Schemas) http.Handler {
	return APIHandler(schemas, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		apiContext := GetAPIContext(request)
		schema := schemas.Schema(mux.Vars(request)["id"])
		populateSchema(apiContext, &schema)
		apiContext.Write(&schema)
	}))
}
