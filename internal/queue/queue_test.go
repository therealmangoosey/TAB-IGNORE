package queue

import (
	"testing"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/config"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

func TestCheckBatteryUnknownAndZero(t *testing.T) {
	q := NewQueue(nil, nil, nil, config.Config{Power: config.PowerConfig{MinBatteryPct: 12}})
	if ok, _ := q.CheckBattery(); ok {
		t.Fatal("unknown battery reading must pause downloads")
	}
	q.SetBattery(0, false)
	if ok, _ := q.CheckBattery(); ok {
		t.Fatal("0% battery must pause downloads")
	}
	q.SetBattery(12, false)
	if ok, _ := q.CheckBattery(); ok {
		t.Fatal("battery at minimum must pause downloads")
	}
	q.SetBattery(13, false)
	if ok, _ := q.CheckBattery(); !ok {
		t.Fatal("battery above minimum should allow downloads")
	}
}

func TestFailJobRetriesAndEventuallyFails(t *testing.T) {
	_ = hermit.Job{}
	if got := retryBackoff(1); got != 30*time.Second {
		t.Fatalf("retryBackoff(1) = %s", got)
	}
	if got := retryBackoff(2); got != time.Minute {
		t.Fatalf("retryBackoff(2) = %s", got)
	}
	if got := retryBackoff(10); got != 30*time.Minute {
		t.Fatalf("retryBackoff(10) = %s", got)
	}
}
