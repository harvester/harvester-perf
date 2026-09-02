package prometheus

import "fmt"

func MilliCores(v float64) string { return fmt.Sprintf("%dm", int(v*1000)) }
func Mebibytes(v float64) string  { return fmt.Sprintf("%dMi", int(v/1024/1024)) }
func Gibibytes(v float64) string  { return fmt.Sprintf("%.1f Gi", v/1024/1024/1024) }
func Pct(v, total float64) string {
	if total == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d", int(v/total*100))
}
