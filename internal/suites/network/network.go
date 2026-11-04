package network

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/pkg/k8s"
	"github.com/harvester/hvperf/pkg/suites"
	"k8s.io/klog/v2"
)

func init() {
	suites.Register(NewNetworkSuite())
}

var _ suites.Suite = &NetworkSuite{}

type NetworkSuite struct {
	suites.SuiteMarshaler
	*suites.ProgressReporter
	*suites.Clients
}

func NewNetworkSuite() *NetworkSuite {
	n := &NetworkSuite{}
	n.Marshal = n
	return n
}

func (s *NetworkSuite) Name() string {
	return "network-benchmark"
}

func (s *NetworkSuite) Description() string {
	return "run netperf and iperf3 to benchmark the in-cluster network latency and throughput"
}

func (s *NetworkSuite) IsExclusive() bool {
	return true
}

func (s *NetworkSuite) SetClients(clients *suites.Clients) {
	s.Clients = clients
}

func (s *NetworkSuite) SetProgressReporter(reporter *suites.ProgressReporter) {
	s.ProgressReporter = reporter
}

func (s *NetworkSuite) RunE(
	ctx context.Context,
	runID string,
	namespace string,
	opts suites.Options,
) suites.SuiteResult {
	o, err := NewNetworkOptions(opts)
	if err != nil {
		return suites.NewSuiteResult(s.Name(), runID).WithError(err)
	}

	if _, err := k8s.EnsureNamespace(ctx, s.Clients, namespace, o.NamespaceReadyTimeout); err != nil {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   fmt.Sprintf("namespace %s not ready: %v", namespace, err.Error()),
		}
	}
	klog.V(3).InfoS("namespace is ready\n", "name", namespace)

	testSuites := []struct {
		name    string
		profile ProfileName
		fn      BenchmarkFunc
	}{
		{
			name:    "netperf",
			profile: ProfileTCPStream,
			fn:      s.netperf,
		},
		{
			name:    "netperf",
			profile: ProfileTCPCRR,
			fn:      s.netperf,
		},
		{
			name:    "netperf",
			profile: ProfileTCPRR,
			fn:      s.netperf,
		},
		{
			name:    "netperf",
			profile: ProfileUDPStream,
			fn:      s.netperf,
		},
		{
			name:    "netperf",
			profile: ProfileUDPRR,
			fn:      s.netperf,
		},
		{
			name:    "iperf3",
			profile: ProfileTCPStream,
			fn:      s.iperf3,
		},
		{
			name:    "iperf3",
			profile: ProfileUDPStream,
			fn:      s.iperf3,
		},
	}

	caseResults := []*suites.CaseResult{}
	for _, testCase := range testSuites {
		tcName := fmt.Sprintf("%s-%s", testCase.name, testCase.profile)
		s.CaseStart(s.Name(), tcName)
		caseResult := testCase.fn(ctx, tcName, runID, namespace, testCase.profile, o)
		caseResults = append(caseResults, caseResult)
		s.CaseDone(s.Name(), tcName, caseResult.State, time.Since(caseResult.DateTimeStart))
	}

	suiteParams, err := suites.ToSuiteParams(o)
	if err != nil {
		// just log the params conversion error, don't fail the suite. the only effect
		// is that the suite params won't be reported in the results.
		klog.V(3).ErrorS(err, "failed to convert suite options to suite params\n")
	}

	return suites.NewSuiteResult(s.Name(), runID).WithParams(suiteParams).WithResults(caseResults)
}

type (
	BenchmarkFunc         func(context.Context, string, string, string, ProfileName, *NetworkOptions) *suites.CaseResult
	BenchmarkWorkloadType string

	// IPFamilyOption corresponds to the -4 or -6 option in netperf, which specifies
	// whether to use IPv4 or IPv6.
	IPFamilyOption string

	// ProfileName is the name of a netperf test profiles as defined in
	// https://hewlettpackard.github.io/netperf/doc/netperf.html#Options-common-to-TCP-UDP-and-SCTP-tests
	ProfileName string
)

const (
	BenchmarkClient BenchmarkWorkloadType = "client"
	BenchmarkServer BenchmarkWorkloadType = "server"

	IPv4 IPFamilyOption = "-4"
	IPv6 IPFamilyOption = "-6"

	ProfileTCPCRR    ProfileName = "TCP_CRR"
	ProfileTCPRR     ProfileName = "TCP_RR"
	ProfileTCPStream ProfileName = "TCP_STREAM"
	ProfileUDPRR     ProfileName = "UDP_RR"
	ProfileUDPStream ProfileName = "UDP_STREAM"
)

// ProfileParams are parameters for each test profile derived from k8s-netperf's
// implementation
var ProfileParams = map[ProfileName][]string{
	ProfileTCPCRR:    {"-O", "THROUGHPUT,THROUGHPUT_UNITS,MIN_LATENCY,MEAN_LATENCY,P50_LATENCY,P90_LATENCY,P99_LATENCY,MAX_LATENCY,STDDEV_LATENCY,LOCAL_TRANSPORT_RETRANS", "-r", "1024,1024", "-P", "42424"},
	ProfileTCPRR:     {"-O", "THROUGHPUT,THROUGHPUT_UNITS,MIN_LATENCY,MEAN_LATENCY,P50_LATENCY,P90_LATENCY,P99_LATENCY,MAX_LATENCY,STDDEV_LATENCY,LOCAL_TRANSPORT_RETRANS", "-r", "1024,1024", "-P", "42424"},
	ProfileTCPStream: {"-O", "THROUGHPUT,THROUGHPUT_UNITS,LOCAL_SEND_CALLS,REMOTE_RECV_CALLS,LOCAL_TRANSPORT_RETRANS", "-m", "1024", "-P", "42424"},
	ProfileUDPRR:     {"-O", "THROUGHPUT,THROUGHPUT_UNITS,MIN_LATENCY,MEAN_LATENCY,P50_LATENCY,P90_LATENCY,P99_LATENCY,MAX_LATENCY,STDDEV_LATENCY,LOCAL_TRANSPORT_RETRANS", "-r", "1024,1024", "-P", "42424"},
	ProfileUDPStream: {"-O", "THROUGHPUT,THROUGHPUT_UNITS,LOCAL_SEND_CALLS,REMOTE_RECV_CALLS,LOCAL_TRANSPORT_RETRANS", "-m", "1024", "-R", "1", "-P", "42424"},
}
