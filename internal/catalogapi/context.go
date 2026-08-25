package catalogapi

import (
	"context"
	"net/http"

	client "github.com/PastureStack/catalog-service/internal/catalogclient"
)

type contextKey struct{}

type APIContext struct {
	Schemas        *client.Schemas
	URLBuilder     URLBuilder
	request        *http.Request
	responseWriter http.ResponseWriter
}

func GetAPIContext(request *http.Request) *APIContext {
	value, _ := request.Context().Value(contextKey{}).(*APIContext)
	return value
}

func withAPIContext(writer http.ResponseWriter, request *http.Request, schemas *client.Schemas) (*http.Request, error) {
	builder, err := newURLBuilder(request, schemas)
	if err != nil {
		return nil, err
	}
	apiContext := &APIContext{
		Schemas:        schemas,
		URLBuilder:     builder,
		request:        request,
		responseWriter: writer,
	}
	return request.WithContext(context.WithValue(request.Context(), contextKey{}, apiContext)), nil
}
