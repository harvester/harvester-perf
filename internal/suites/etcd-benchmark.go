package suites

import (
	"context"
	"fmt"
	"time"

	pkgsuites "github.com/harvester/hperf/pkg/suites"

	batchv1 "k8s.io/api/batch/v1"
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
	*pkgsuites.Clients
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

func (s *EtcdBenchmarkSuite) RunE(ctx context.Context, opts pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	o := EtcdBenchmarkSuiteOptionsDefaults()
	custom, err := FromOptions(opts["etcd-benchmark"])
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	// TODO: merge custom options into defaults
	fmt.Printf("%+v\n", custom.TestRunID)

	jobName := "etcd-benchmark-" + o.TestRunID
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: o.JobPodNamespace,
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   new(int64(o.JobActiveDeadline.Seconds())),
			TTLSecondsAfterFinished: new(int32(o.JobPodTTLAfterFinished.Seconds())),
			Suspend:                 new(o.JobSuspend),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					HostNetwork:   true,
					NodeName:      o.JobPodNode,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "benchmark",
							Image:           o.JobPodImageName,
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
			},
		},
	}

	created, err := s.K8sClientSet.BatchV1().Jobs(o.JobPodNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return pkgsuites.SuiteResult{}, nil
	}
	fmt.Printf("%+v\n", created)

	return pkgsuites.SuiteResult{}, nil
}

func (s *EtcdBenchmarkSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

type EtcdBenchmarkSuiteOptions struct {
	TestRunID       string
	EtcdEndpoint    string
	EtcdMetricsPort int

	JobActiveDeadline      time.Duration
	JobPodImageName        string
	JobPodImageTag         string
	JobPodNamespace        string
	JobPodNode             string
	JobPodTTLAfterFinished time.Duration
	JobSuspend             bool

	LoadSize   EtcdBenchmarkLoadSize
	ConnSize   string
	ClientSize string
	ValSize    string
}

func EtcdBenchmarkSuiteOptionsDefaults() *EtcdBenchmarkSuiteOptions {
	return &EtcdBenchmarkSuiteOptions{
		TestRunID:              time.Now().Format("20060102150405"),
		JobActiveDeadline:      600 * time.Second,
		JobPodImageName:        "registry.suse.com/bci/bci-nano",
		JobPodImageTag:         "latest",
		JobPodNamespace:        "default",
		JobPodTTLAfterFinished: 3600 * time.Second,
	}
}

type EtcdBenchmarkLoadSize uint64

const (
	EtcdBenchmarkLoadSizeSmall  EtcdBenchmarkLoadSize = 10000
	EtcdBenchmarkLoadSizeMedium EtcdBenchmarkLoadSize = 50000
)
