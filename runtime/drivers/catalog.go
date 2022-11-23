package drivers

import (
	"context"
	"time"

	runtimev1 "github.com/rilldata/rill/proto/gen/rill/runtime/v1"
)

// CatalogStore is implemented by drivers capable of storing catalog info for a specific instance
type CatalogStore interface {
	FindObjects(ctx context.Context, instanceID string, typ CatalogObjectType) []*CatalogObject
	FindObject(ctx context.Context, instanceID string, name string) (*CatalogObject, bool)
	CreateObject(ctx context.Context, instanceID string, object *CatalogObject) error
	UpdateObject(ctx context.Context, instanceID string, object *CatalogObject) error
	DeleteObject(ctx context.Context, instanceID string, name string) error
}

// CatalogObject represents one object in the catalog, such as a source
type CatalogObject struct {
	// basic fields
	Name    string
	Type    CatalogObjectType
	SQL     string
	Schema  *runtimev1.StructType
	Managed bool

	// artifact fields
	Definition []byte
	Path       string

	CreatedOn   time.Time `db:"created_on"`
	UpdatedOn   time.Time `db:"updated_on"`
	RefreshedOn time.Time `db:"refreshed_on"`
}

// Constants representing different kinds of catalog objects

type CatalogObjectType string

const (
	CatalogObjectTypeUnspecified CatalogObjectType = ""
	CatalogObjectTypeTable       CatalogObjectType = "table"
	CatalogObjectTypeSource      CatalogObjectType = "source"
	CatalogObjectTypeModel       CatalogObjectType = "model"
	CatalogObjectTypeMetricsView CatalogObjectType = "metrics_view"
)
