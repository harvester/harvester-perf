package prometheus

import (
	"context"
	"fmt"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type MeasureWindow struct {
	Duration time.Duration
	Step     time.Duration
}

func (w MeasureWindow) String() string {
	return fmt.Sprintf("[%s:%s]", promDuration(w.Duration), promDuration(w.Step))
}

func promDuration(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// RunInstant runs a PromQL instant query at time.Now().
func RunInstant(ctx context.Context, client promv1.API, query string) (model.Vector, error) {
	result, warnings, err := client.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("instant query %q: %w", query, err)
	}
	for _, w := range warnings {
		fmt.Printf("warning: %s\n", w)
	}
	v, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("instant query %q: expected Vector, got %T", query, result)
	}
	return v, nil
}

func VectorByLabel(v model.Vector, label string) map[string]model.SampleValue {
	m := map[string]model.SampleValue{}
	for _, s := range v {
		m[string(s.Metric[model.LabelName(label)])] = s.Value
	}
	return m
}
