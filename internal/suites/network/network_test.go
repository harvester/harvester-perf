package network

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/hvperf/pkg/suites"
)

func podWithIPs(ips ...string) *corev1.Pod {
	p := &corev1.Pod{}
	for _, ip := range ips {
		p.Status.PodIPs = append(p.Status.PodIPs, corev1.PodIP{IP: ip})
	}
	return p
}

func TestPodIPByIPFamily(t *testing.T) {
	testCases := []struct {
		name    string
		pod     *corev1.Pod
		family  IPFamilyOption
		want    string
		wantErr string
	}{
		{name: "v4 only, want v4", pod: podWithIPs("10.0.0.1"), family: IPv4, want: "10.0.0.1"},
		{name: "v6 only, want v6", pod: podWithIPs("fd00::1"), family: IPv6, want: "fd00::1"},
		{name: "dual stack v4 first, want v4", pod: podWithIPs("10.0.0.1", "fd00::1"), family: IPv4, want: "10.0.0.1"},
		{name: "dual stack v4 first, want v6", pod: podWithIPs("10.0.0.1", "fd00::1"), family: IPv6, want: "fd00::1"},
		{name: "dual stack v6 first, want v4", pod: podWithIPs("fd00::1", "10.0.0.1"), family: IPv4, want: "10.0.0.1"},
		{name: "dual stack v6 first, want v6", pod: podWithIPs("fd00::1", "10.0.0.1"), family: IPv6, want: "fd00::1"},
		{name: "v4 only, want v6", pod: podWithIPs("10.0.0.1"), family: IPv6, wantErr: "no server pod IP found"},
		{name: "v6 only, want v4", pod: podWithIPs("fd00::1"), family: IPv4, wantErr: "no server pod IP found"},
		{name: "no IPs", pod: podWithIPs(), family: IPv4, wantErr: "no server pod IP found"},
		{name: "unparsable IP", pod: podWithIPs("not-an-ip"), family: IPv4, wantErr: "failed to parse"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := podIPByIPFamily(tc.pod, tc.family)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewNetworkOptions(t *testing.T) {
	t.Run("defaults decode into struct fields", func(t *testing.T) {
		o, err := NewNetworkOptions(suites.Options{"toolsDir": "/tools"})
		if err != nil {
			t.Fatal(err)
		}
		if o.NetPerfIPFamily != IPv4 {
			t.Errorf("NetPerfIPFamily = %q, want %q", o.NetPerfIPFamily, IPv4)
		}
		if o.NetPerfTestStartWaitSeconds != 2 {
			t.Errorf("NetPerfTestStartWaitSeconds = %d, want 2", o.NetPerfTestStartWaitSeconds)
		}
		if o.NetPerfTestLengthSeconds != 10 {
			t.Errorf("NetPerfTestLengthSeconds = %d, want 10", o.NetPerfTestLengthSeconds)
		}
		if o.NetPerfRemoteCopyTargetDir != "/usr/local/bin/" {
			t.Errorf("NetPerfRemoteCopyTargetDir = %q", o.NetPerfRemoteCopyTargetDir)
		}
		if o.IPerfIPFamily != IPv4 {
			t.Errorf("IPerfIPFamily = %q, want %q", o.IPerfIPFamily, IPv4)
		}
		if o.IPerfClientMessageSizeBytes != 1024 {
			t.Errorf("IPerfClientMessageSizeBytes = %d, want 1024", o.IPerfClientMessageSizeBytes)
		}
		if o.IPerfClientParallelStreams != 1 {
			t.Errorf("IPerfClientParallelStreams = %d, want 1", o.IPerfClientParallelStreams)
		}
		if o.IPerfServerPort != 22865 {
			t.Errorf("IPerfServerPort = %d, want 22865", o.IPerfServerPort)
		}
		if o.IPerfTestDurationSeconds != 10 {
			t.Errorf("IPerfTestDurationSeconds = %d, want 10", o.IPerfTestDurationSeconds)
		}
		if o.IPerfRemoteCopyTargetDir != "/usr/local/bin/" {
			t.Errorf("IPerfRemoteCopyTargetDir = %q", o.IPerfRemoteCopyTargetDir)
		}
	})

	t.Run("local paths derived from toolsDir", func(t *testing.T) {
		o, err := NewNetworkOptions(suites.Options{"toolsDir": "/tools"})
		if err != nil {
			t.Fatal(err)
		}
		if o.NetPerfClientLocalPath != "/tools/netperf" {
			t.Errorf("NetPerfClientLocalPath = %q", o.NetPerfClientLocalPath)
		}
		if o.NetPerfServerLocalPath != "/tools/netserver" {
			t.Errorf("NetPerfServerLocalPath = %q", o.NetPerfServerLocalPath)
		}
		if o.IPerfBinLocalPath != "/tools/iperf3" {
			t.Errorf("IPerfBinLocalPath = %q", o.IPerfBinLocalPath)
		}
	})

	t.Run("missing toolsDir errors", func(t *testing.T) {
		if _, err := NewNetworkOptions(suites.Options{}); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("non-string toolsDir errors", func(t *testing.T) {
		if _, err := NewNetworkOptions(suites.Options{"toolsDir": 1}); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("override beats defaults", func(t *testing.T) {
		o, err := NewNetworkOptions(suites.Options{
			"toolsDir":                 "/tools",
			"netPerfTestLengthSeconds": 30,
			"netPerfIPFamily":          "-6",
			"iPerfServerPort":          5201,
			"iPerfIPFamily":            "-6",
		})
		if err != nil {
			t.Fatal(err)
		}
		if o.NetPerfTestLengthSeconds != 30 || o.NetPerfIPFamily != IPv6 {
			t.Errorf("got %d, %q", o.NetPerfTestLengthSeconds, o.NetPerfIPFamily)
		}
		if o.IPerfServerPort != 5201 || o.IPerfIPFamily != IPv6 {
			t.Errorf("got %d, %q", o.IPerfServerPort, o.IPerfIPFamily)
		}
	})
}
