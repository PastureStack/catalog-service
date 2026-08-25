package model

import client "github.com/PastureStack/catalog-service/internal/catalogclient"

type CatalogError struct {
	client.Resource
	Status  string `json:"status"`
	Message string `json:"message"`
}
