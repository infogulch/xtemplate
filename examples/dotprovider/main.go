// Custom dot provider example: the "repository pattern". A custom provider
// exposes hardcoded in-memory data to templates as {{.Shop}}, which can range
// over products and look one up by id.
//
// To write a dot provider: implement xtemplate.Provider (FieldName,
// Prototype, Value), then register the instance with xtemplate.WithProvider
// passed to app.Main. FieldName is the dot field name; Prototype is a typed
// zero value for reflection; Value returns the value assigned to that field
// for each request. Init is optional (Initializer).
package main

import (
	"net/http"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"
)

type Product struct {
	ID    int
	Name  string
	Price string
}

// Shop is the value assigned to {{.Shop}}. Its methods are callable from
// templates: {{.Shop.Products}} and {{.Shop.Product 1}}.
type Shop struct{}

var products = []Product{
	{1, "Widget", "$9.99"},
	{2, "Gadget", "$19.99"},
	{3, "Gizmo", "$29.99"},
}

func (Shop) Products() []Product { return products }

func (Shop) Product(id int) Product {
	for _, p := range products {
		if p.ID == id {
			return p
		}
	}
	return Product{}
}

// shopProvider implements xtemplate.Provider, attaching Shop under {{.Shop}}.
type shopProvider struct{}

func (shopProvider) FieldName() string { return "Shop" }
func (shopProvider) Prototype() any    { return Shop{} }
func (shopProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return Shop{}, nil
}

func main() {
	app.Main(xtemplate.WithProvider(shopProvider{}))
}
