package notify

import (
	"context"
	"os"
	"testing"
)

// TestLiveNotification shows one real notification on this Mac through
// osascript. Run with DOPPEL_LIVE_NOTIFY=1 go test ./internal/notify -run Live
func TestLiveNotification(t *testing.T) {
	if os.Getenv("DOPPEL_LIVE_NOTIFY") != "1" {
		t.Skip("set DOPPEL_LIVE_NOTIFY=1 to show a real notification")
	}
	n := New(Options{Backend: BackendOSAScript})
	if err := n.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
}
