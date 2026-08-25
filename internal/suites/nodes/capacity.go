package nodes

import (
	"context"

	pkgsuites "github.com/harvester/hvperf/pkg/suites"
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

func (s *NodeCapacitySuite) RunE(ctx context.Context, opts pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	return pkgsuites.SuiteResult{
		Name: s.Name(),
	}, nil
}

func (s *NodeCapacitySuite) SetClients(clientSets *pkgsuites.Clients) {
}
