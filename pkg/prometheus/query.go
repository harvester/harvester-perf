package prometheus

import (
	"context"
	"fmt"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// RunInstant executes an instant Prometheus query and returns the result vector and warnings.
func RunInstant(ctx context.Context, client promv1.API, query string) (model.Vector, promv1.Warnings, error) {
	result, warnings, err := client.Query(ctx, query, time.Now())
	if err != nil {
		return nil, warnings, fmt.Errorf(
			"instant query %q: %w",
			query,
			err,
		)
	}

	v, ok := result.(model.Vector)
	if !ok {
		return nil, warnings, fmt.Errorf(
			"instant query %q: expected Vector, got %T",
			query,
			result,
		)
	}

	return v, warnings, nil
}

func VectorByLabel(v model.Vector, label string) map[string]model.SampleValue {
	m := map[string]model.SampleValue{}
	for _, s := range v {
		m[string(s.Metric[model.LabelName(label)])] = s.Value
	}
	return m
}
