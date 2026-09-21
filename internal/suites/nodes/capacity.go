package nodes

import (
	"context"
	"strconv"
	"time"

	"github.com/harvester/hvperf/pkg/suites"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ suites.Suite = &NodeCapacitySuite{}

func init() {
	suites.Register(NewNodeCapacitySuite())
}

// NodeCapacitySuite implements test suite to assess node resource capacity.
type NodeCapacitySuite struct {
	suites.SuiteMarshaler
	*suites.ProgressReporter
	*suites.Clients
}

// NewNodeCapacitySuite creates a new instance of NodeCapacitySuite.
func NewNodeCapacitySuite() *NodeCapacitySuite {
	s := &NodeCapacitySuite{}
	s.Marshal = s
	return s
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

func (s *NodeCapacitySuite) RunE(ctx context.Context, runID, namespace string, opts suites.Options) suites.SuiteResult {
	cases := []struct {
		caseName string
		caseFunc func(context.Context, string) *suites.CaseResult
	}{
		{
			caseName: "node os info",
			caseFunc: s.execNodeOSInfo,
		},
	}

	var caseResults []*suites.CaseResult
	for _, c := range cases {
		s.CaseStart(s.Name(), c.caseName)
		caseResult := s.execNodeOSInfo(ctx, c.caseName)
		caseResults = append(caseResults, caseResult)
		s.CaseDone(s.Name(), c.caseName, caseResult.State, time.Since(caseResult.DateTimeStart))
	}

	return suites.SuiteResult{
		Name:    s.Name(),
		RunID:   runID,
		Results: caseResults,
	}
}

func (s *NodeCapacitySuite) execNodeOSInfo(ctx context.Context, name string) *suites.CaseResult {
	klog.V(3).Infof("Executing test case: %s", name)
	caseResult := suites.NewCaseResult(name, time.Now(), time.Now(), nil, nil)
	nodes, err := s.K8sClientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		caseResult.WithErr(err)
		return caseResult
	}

	var results []*suites.K8sResourceResult
	for _, node := range nodes.Items {
		result := &suites.K8sResourceResult{
			Resource: "node",
			Subject:  node.GetName(),
			Data:     map[string]string{},
		}

		pods, err := s.K8sClientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + node.GetName() + ",status.phase=Running",
		})
		if err != nil {
			result.Err = err
			continue
		}

		status := node.Status
		nodeInfo := status.NodeInfo
		result.Data["architecture"] = nodeInfo.Architecture
		result.Data["container.runtime"] = nodeInfo.ContainerRuntimeVersion
		result.Data["cpu.allocatable"] = status.Allocatable.Cpu().String()
		result.Data["cpu.capacity"] = status.Capacity.Cpu().String()
		result.Data["ephemeral-storage.allocatable"] = status.Allocatable.StorageEphemeral().String()
		result.Data["ephemeral-storage.capacity"] = status.Capacity.StorageEphemeral().String()
		result.Data["kernel.version"] = nodeInfo.KernelVersion
		result.Data["kubelet.version"] = nodeInfo.KubeletVersion
		result.Data["memory.allocatable"] = status.Allocatable.Memory().String()
		result.Data["memory.capacity"] = status.Capacity.Memory().String()
		result.Data["os.image"] = nodeInfo.OSImage
		result.Data["pods.capacity"] = status.Capacity.Pods().String()
		result.Data["pods.running"] = strconv.Itoa(len(pods.Items))

		results = append(results, result)

	}
	caseResult.WithK8sResourceResults(results).DateTimeEnd = time.Now()
	return caseResult
}

func (s *NodeCapacitySuite) SetClients(clientSets *suites.Clients) {
	s.Clients = clientSets
}

func (s *NodeCapacitySuite) SetProgressReporter(reporter *suites.ProgressReporter) {
	s.ProgressReporter = reporter
}
