package resource

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
)

// The manifests hold the parts that never vary per run; everything tunable in
// them comes from the spec passed to render, which hvperf.yaml fills in.
//
//go:embed cfg/vm.yaml
var vmYAML []byte

//go:embed cfg/vmimage.yaml
var vmImageYAML []byte

var (
	vmTpl      = template.Must(template.New("vm.yaml").Parse(string(vmYAML)))
	vmImageTpl = template.Must(template.New("vmimage.yaml").Parse(string(vmImageYAML)))
)

// render fills a manifest template with spec and decodes the result.
func render(tpl *template.Template, spec any) (*unstructured.Unstructured, error) {
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, spec); err != nil {
		return nil, fmt.Errorf("render %s: %w", tpl.Name(), err)
	}
	u := &unstructured.Unstructured{}
	if err := sigsyaml.Unmarshal(buf.Bytes(), &u.Object); err != nil {
		return nil, fmt.Errorf("decode %s: %w", tpl.Name(), err)
	}
	return u, nil
}
