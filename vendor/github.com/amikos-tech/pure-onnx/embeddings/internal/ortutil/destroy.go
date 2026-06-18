package ortutil

import (
	"errors"
	"reflect"
)

// Destroyer is implemented by ONNX runtime resources that must be explicitly destroyed.
type Destroyer interface {
	Destroy() error
}

// DestroyAll destroys each resource and joins all non-nil errors.
// Typed nil values are ignored.
func DestroyAll(resources ...Destroyer) error {
	var err error
	for _, resource := range resources {
		if isNilDestroyer(resource) {
			continue
		}
		if destroyErr := resource.Destroy(); destroyErr != nil {
			err = errors.Join(err, destroyErr)
		}
	}
	return err
}

func isNilDestroyer(resource Destroyer) bool {
	if resource == nil {
		return true
	}
	value := reflect.ValueOf(resource)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
