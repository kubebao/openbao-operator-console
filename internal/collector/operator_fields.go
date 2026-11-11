package collector

import (
	"math"
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// These projections are specific to the console's enrolled controller Deployment.
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

func text(object map[string]any, fields ...string) string {
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found || !identifier.MatchString(value) {
		return ""
	}
	return value
}

func optionalText(object map[string]any, fields ...string) *string {
	value := text(object, fields...)
	if value == "" {
		return nil
	}
	return &value
}

func count(object map[string]any, fields ...string) *int32 {
	value, found, err := unstructured.NestedInt64(object, fields...)
	if err != nil || !found || value < 0 || value > math.MaxInt32 {
		return nil
	}
	n := int32(value)
	return &n
}
