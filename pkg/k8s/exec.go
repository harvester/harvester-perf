package k8s

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/harvester/hvperf/pkg/suites"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
)

// CopyToJobPod copies the specified source files to the target directory in the given pod.
func CopyToJobPod(
	ctx context.Context,
	c *suites.Clients,
	pod *corev1.Pod,
	remoteTargetDir string,
	sourcePaths ...string,
) error {
	var deferFuncs []func() error
	defer func() {
		for _, f := range deferFuncs {
			if err := f(); err != nil {
				klog.Errorf("failed to perform post-copy clean up for pod %s/%s: %v", pod.GetNamespace(), pod.GetName(), err)
			}
		}
	}()

	for _, path := range sourcePaths {
		fileInfo, err := os.Stat(path)
		if err != nil {
			return err
		}
		bin, err := os.Open(path)
		if err != nil {
			return err
		}
		deferFuncs = append(deferFuncs, bin.Close)

		// archive the bin as a tar before sending to the cluster
		var buf bytes.Buffer
		tarWriter := tar.NewWriter(&buf)
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: fileInfo.Name(),
			Mode: 0o755,
			Size: fileInfo.Size(),
		}); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}
		if _, err := io.Copy(tarWriter, bin); err != nil {
			return fmt.Errorf("failed to copy file to tar: %w", err)
		}
		if err := tarWriter.Close(); err != nil {
			return fmt.Errorf("failed to close tar writer: %w", err)
		}

		// exec command: tar -xf - -C <remoteDir>
		cmd := []string{"tar", "-xf", "-", "-C", remoteTargetDir}
		req := c.K8sClientSet.CoreV1().RESTClient().
			Post().
			Resource("pods").
			Name(pod.GetName()).
			Namespace(pod.GetNamespace()).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: "hvperf",
				Command:   cmd,
				Stdin:     true,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, scheme.ParameterCodec)

		klog.V(5).Infof("exec cmd: %q\n", strings.Join(cmd, " "))

		// setup spdy executor and stream the tar to the pod's stdin
		exec, err := remotecommand.NewSPDYExecutor(c.RestConfig, "POST", req.URL())
		if err != nil {
			return fmt.Errorf("failed to init SPDY executor: %w", err)
		}

		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  &buf,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		})
		if err != nil {
			return fmt.Errorf("failed to stream binary: %w", err)
		}
	}
	return nil
}

// ExecPod executes the specified command in the given pod and returns the
// combined stdout and stderr outputs of the command.
func ExecPod(
	ctx context.Context,
	c *suites.Clients,
	pod *corev1.Pod,
	cmd []string,
) (io.Reader, io.Reader, error) {
	req := c.K8sClientSet.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(pod.GetName()).
		Namespace(pod.GetNamespace()).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "hvperf",
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	klog.V(5).Infof("exec cmd: %q\n", strings.Join(cmd, " "))

	// setup spdy executor and exec the command in the pod
	exec, err := remotecommand.NewSPDYExecutor(c.RestConfig, "POST", req.URL())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init SPDY executor: %w", err)
	}

	var (
		bout = &bytes.Buffer{}
		berr = &bytes.Buffer{}
	)
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: bout,
		Stderr: berr,
	}); err != nil {
		return bout, berr, fmt.Errorf("failed to exec command '%s': %w", strings.Join(cmd, " "), err)
	}
	return bout, berr, nil
}
