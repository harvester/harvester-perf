package suites

import (
	"fmt"
	"time"

	"github.com/harvester/hperf/pkg/suites"
	pkgsuites "github.com/harvester/hperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	pkgsuites.Register(NewEtcdBenchmarkSuite())
}

var _ pkgsuites.Suite = &EtcdBenchmarkSuite{}

// EtcdBenchmarkSuite implements test suite to assess etcd performance.
type EtcdBenchmarkSuite struct {
	pkgsuites.SuiteMarshaler
}

// NewEtcdBenchmarkSuite creates a new instance of EtcdBenchmarkSuite with the
// provided options.
func NewEtcdBenchmarkSuite() *EtcdBenchmarkSuite {
	s := &EtcdBenchmarkSuite{}
	s.Marshal = s
	return s
}

func (s *EtcdBenchmarkSuite) Name() string {
	return "etcd-benchmark"
}

func (s *EtcdBenchmarkSuite) Description() string {
	return "run the etcd-benchmark tool to against the cluster's etcd"
}

func (s *EtcdBenchmarkSuite) IsReadWrite() bool {
	return true
}

func (s *EtcdBenchmarkSuite) RunE(opts suites.Options) (pkgsuites.SuiteResult, error) {
	o, err := FromOptions(opts["etcd-benchmark"])
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	podName := "etcd-benchmark-" + o.TestRunID
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: o.JobPodNamespace,
		},
		Spec: corev1.PodSpec{
			HostNetwork:   true,
			NodeName:      o.JobPodNode,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "benchmark",
					Image:           o.JobPodImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"/bin/bash", "-c", "sleep infinity"},
					SecurityContext: &corev1.SecurityContext{
						RunAsUser:  new(int64(0)),
						Privileged: new(true),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "rancher",
							MountPath: "/host/rancher",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "rancher",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/lib/rancher",
							Type: new(corev1.HostPathDirectory),
						},
					},
				},
			},
		},
	}
	fmt.Printf("%+v\n", pod)

	return pkgsuites.SuiteResult{}, nil
}

type EtcdBenchmarkSuiteOptions struct {
	TestRunID       string
	EtcdEndpoint    string
	EtcdMetricsPort int

	JobPodImage     string
	JobPodNamespace string
	JobPodTimeout   time.Duration
	JobPodKeep      bool
	JobPodNode      string

	LoadSize   EtcdBenchmarkLoadSize
	ConnSize   string
	ClientSize string
	ValSize    string
}

type EtcdBenchmarkLoadSize uint64

const (
	EtcdBenchmarkLoadSizeSmall  EtcdBenchmarkLoadSize = 10000
	EtcdBenchmarkLoadSizeMedium EtcdBenchmarkLoadSize = 50000
)
