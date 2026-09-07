package prometheus

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestVectorByLabel(t *testing.T) {
	tests := []struct {
		name  string
		vec   model.Vector
		label string
		want  map[string]model.SampleValue
	}{
		{
			name:  "empty vector",
			vec:   model.Vector{},
			label: "node",
			want:  map[string]model.SampleValue{},
		},
		{
			name: "single sample",
			vec: model.Vector{
				{Metric: model.Metric{"node": "node-a"}, Value: 1.5},
			},
			label: "node",
			want:  map[string]model.SampleValue{"node-a": 1.5},
		},
		{
			name: "multiple samples with different label values",
			vec: model.Vector{
				{Metric: model.Metric{"node": "node-a"}, Value: 1.0},
				{Metric: model.Metric{"node": "node-b"}, Value: 2.0},
				{Metric: model.Metric{"node": "node-c"}, Value: 3.0},
			},
			label: "node",
			want: map[string]model.SampleValue{
				"node-a": 1.0,
				"node-b": 2.0,
				"node-c": 3.0,
			},
		},
		{
			name: "label not present — key is empty string",
			vec: model.Vector{
				{Metric: model.Metric{"pod": "p1"}, Value: 9.0},
			},
			label: "node",
			want:  map[string]model.SampleValue{"": 9.0},
		},
		{
			name: "duplicate label value — last sample wins",
			vec: model.Vector{
				{Metric: model.Metric{"node": "node-a"}, Value: 1.0},
				{Metric: model.Metric{"node": "node-a"}, Value: 2.0},
			},
			label: "node",
			want:  map[string]model.SampleValue{"node-a": 2.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VectorByLabel(tt.vec, tt.label)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for k, wantVal := range tt.want {
				gotVal, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("key %q: value = %v, want %v", k, gotVal, wantVal)
				}
			}
		})
	}
}
