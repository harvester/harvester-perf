package k8s

import (
	"bufio"
	"io"

	"k8s.io/klog/v2"
)

func pipeToKlog(level klog.Level) io.Writer {
	if !klog.V(level).Enabled() {
		return io.Discard
	}

	pr, pw := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			klog.V(level).Info(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			klog.Errorf("stream read error: %v", err)
		}
	}()
	return pw
}

func closeWriter(w io.Writer) {
	if c, ok := w.(io.Closer); ok {
		if err := c.Close(); err != nil {
			klog.Errorf("failed to close writer: %v", err)
		}
	}
}
