package suites

import perfsuites "github.com/harvester/hperf/pkg/suites"

var _ perfsuites.Suite = &NodeCapacitySuite{}

func init() {
	suite := &NodeCapacitySuite{}
	suite.Marshal = suite
	perfsuites.Register(suite)
}

// NodeCapacitySuite implements test suite to assess node resource capacity.
type NodeCapacitySuite struct {
	perfsuites.SuiteMarshaler
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

func (s *NodeCapacitySuite) RunE() (perfsuites.SuiteResult, error) {
	return perfsuites.SuiteResult{}, nil
}
