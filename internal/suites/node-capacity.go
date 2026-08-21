package suites

import (
	"github.com/harvester/hperf/pkg/suites"
	pkgsuites "github.com/harvester/hperf/pkg/suites"
)

var _ pkgsuites.Suite = &NodeCapacitySuite{}

func init() {
	suite := &NodeCapacitySuite{}
	suite.Marshal = suite
	pkgsuites.Register(suite)
}

// NodeCapacitySuite implements test suite to assess node resource capacity.
type NodeCapacitySuite struct {
	pkgsuites.SuiteMarshaler
}

func (s *NodeCapacitySuite) Name() string {
	return "node-capacity"
}

func (s *NodeCapacitySuite) Description() string {
	return "assess the node resource capacity of the cluster"
}

func (s *NodeCapacitySuite) IsReadWrite() bool {
	return false
}

func (s *NodeCapacitySuite) RunE(opts suites.Options) (pkgsuites.SuiteResult, error) {
	return pkgsuites.SuiteResult{}, nil
}
