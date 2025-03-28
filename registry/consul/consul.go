package consul

import (
	"github.com/shangzongyu/mqant/registry"
)

func NewRegistry(opts ...registry.Option) registry.Registry {
	return registry.NewRegistry(opts...)
}
