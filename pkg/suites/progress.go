package suites

import (
	"fmt"
	"io"
	"os"
	"time"

	"k8s.io/klog/v2"
)

// ProgressReporter writes suite and case execution progress to an io.Writer.
type ProgressReporter struct {
	out         io.Writer
	runID       string
	totalSuites int
}

// NewProgressReporter creates a new ProgressReporter that writes to the given
// io.Writer.
func NewProgressReporter(out io.Writer, runID string, totalSuites int) *ProgressReporter {
	if out == nil {
		out = os.Stderr
	}

	return &ProgressReporter{
		out:         out,
		runID:       runID,
		totalSuites: totalSuites,
	}
}

// SuiteStart writes a message indicating that a test suite has started running.
func (p *ProgressReporter) SuiteStart(suiteName string, i int) {
	if _, err := fmt.Fprintf(p.out, "[%d/%d] %s: running... (id=%s)\n", i, p.totalSuites, suiteName, p.runID); err != nil {
		klog.ErrorS(err, "failed to write suite started message")
	}
}

// SuiteDone writes a message indicating that a test suite has finished running.
func (p *ProgressReporter) SuiteDone(suiteName string, elapsed time.Duration) {
	if _, err := fmt.Fprintf(p.out, "      %s: finished (%s)\n", suiteName, elapsed); err != nil {
		klog.ErrorS(err, "failed to write suite finished message")
	}
}

// CaseStart writes a message indicating that a test case has started running.
func (p *ProgressReporter) CaseStart(suiteName, caseName string) {
	if _, err := fmt.Fprintf(p.out, "\r\033[K[%s] %s: running...", suiteName, caseName); err != nil {
		klog.ErrorS(err, "failed to write case started message")
	}
}

// CaseDone writes a message indicating that a test case has finished running,
// along with its result (pass/fail) and duration.
func (p *ProgressReporter) CaseDone(suiteName, caseName string, passed bool, dur time.Duration) {
	mark := "✓"
	if !passed {
		mark = "✗"
	}
	if _, err := fmt.Fprintf(p.out, "\r\033[K[%s] %s %s (%s)\n", suiteName, caseName, mark, dur.Round(time.Millisecond)); err != nil {
		klog.ErrorS(err, "failed to write case done message")
	}
}
