package notify

import (
	"context"
	"testing"
)

func TestEmpty(t *testing.T) {
	if err := Send(context.Background(), Notification{}); err == nil {
		t.Error("an empty notification was sent")
	}
}
