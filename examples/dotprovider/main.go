// Custom DotProvider example: the "repository pattern". A custom provider
// exposes hardcoded in-memory data to templates as {{.Shop}}, which can range
// over products and look one up by id.
//
// To write a DotProvider: implement xtemplate.DotConfig (FieldName, Init,
// Value), then register the instance with xtemplate.WithProvider passed to
// app.Main. FieldName is the dot field name; Value returns the value assigned
// to that field for each request.
package main

import (
	"context"

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

// shopProvider implements xtemplate.DotConfig, attaching Shop under {{.Shop}}.
type shopProvider struct{}

func (shopProvider) FieldName() string                    { return "Shop" }
func (shopProvider) Init(context.Context) error           { return nil }
func (shopProvider) Value(xtemplate.Request) (any, error) { return Shop{}, nil }

func main() {
	app.Main(xtemplate.WithProvider(shopProvider{}))
}
