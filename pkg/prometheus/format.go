package prometheus

import (
	"math"

	"k8s.io/apimachinery/pkg/api/resource"
)

func FormatMilliCPU(cores float64) *resource.Quantity {
	return resource.NewMilliQuantity(int64(cores*1000), resource.DecimalSI)
}

// FormatMiB converts a float64 byte count from a PromQL query result into a resource.Quantity rounded to the nearest MiB.
// The maximum rounding error is 0.5 MiB.
func FormatMiB(bytes float64) *resource.Quantity {
	return resource.NewQuantity(int64(math.Round(bytes/1024/1024)*1024*1024), resource.BinarySI)
}
