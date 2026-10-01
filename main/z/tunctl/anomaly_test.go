package tunctl

import (
	"testing"
	"time"
)

func TestAnomalyNotifiesOnceUntilCleared(t *testing.T) {
	prev := anomalyNotify
	got := make(chan string, 4)
	anomalyNotify = func(_, body string) { got <- body }
	t.Cleanup(func() {
		anomalyNotify = prev
		clearAnomaly(HijackPausedIfaceGone, HijackPausedCNOutage, anomalyBypassRefresh)
	})
	clearAnomaly(HijackPausedIfaceGone, HijackPausedCNOutage, anomalyBypassRefresh)

	notifyAnomalyOnce(HijackPausedIfaceGone, "默认网卡丢失，暂直出")
	waitAnomaly(t, got, "默认网卡丢失，暂直出")
	notifyAnomalyOnce(HijackPausedIfaceGone, "默认网卡丢失，暂直出")
	expectNoAnomaly(t, got)

	clearAnomaly(HijackPausedIfaceGone)
	notifyAnomalyOnce(HijackPausedIfaceGone, "默认网卡丢失，暂直出")
	waitAnomaly(t, got, "默认网卡丢失，暂直出")
}

func waitAnomaly(t *testing.T, got <-chan string, want string) {
	t.Helper()
	select {
	case body := <-got:
		if body != want {
			t.Fatalf("body %q want %q", body, want)
		}
	case <-time.After(time.Second):
		t.Fatal("notification missing")
	}
}

func expectNoAnomaly(t *testing.T, got <-chan string) {
	t.Helper()
	select {
	case body := <-got:
		t.Fatalf("unexpected notification %q", body)
	case <-time.After(30 * time.Millisecond):
	}
}
