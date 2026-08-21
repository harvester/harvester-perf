package suites

import (
	"fmt"
	"time"

	perfsuites "github.com/harvester/hperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ perfsuites.Suite = &EtcdBenchmarkSuite{}

func init() {
	suite := &EtcdBenchmarkSuite{}
	suite.Marshal = suite
	perfsuites.Register(suite)
}

// EtcdBenchmarkSuite implements test suite to assess etcd performance.
type EtcdBenchmarkSuite struct {
	perfsuites.SuiteMarshaler
	*EtcdBenchmarkSuiteOption
}

type EtcdBenchmarkLoadSize uint64

const (
	EtcdBenchmarkLoadSizeSmall  EtcdBenchmarkLoadSize = 10000
	EtcdBenchmarkLoadSizeMedium EtcdBenchmarkLoadSize = 50000
)

type EtcdBenchmarkSuiteOption struct {
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

func (o *EtcdBenchmarkSuiteOption) Bind(s *EtcdBenchmarkSuite) {
	s.EtcdBenchmarkSuiteOption = o
}

func (s *EtcdBenchmarkSuite) Name() string {
	return "etcd-benchmark"
}

func (s *EtcdBenchmarkSuite) Description() string {
	return "run the etcd-benchmark tool to against the cluster's etcd"
}

func (s *EtcdBenchmarkSuite) IsReadWrite() bool {
	return false
}

func (s *EtcdBenchmarkSuite) RunE() (perfsuites.SuiteResult, error) {
	podName := "etcd-benchmark-" + s.TestRunID
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: s.JobPodNamespace,
		},
		Spec: corev1.PodSpec{
			HostNetwork:   true,
			NodeName:      s.JobPodNode,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "benchmark",
					Image:           s.JobPodImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"/binbash", "-c", "sleep infinity"},
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

	return perfsuites.SuiteResult{}, nil
}
