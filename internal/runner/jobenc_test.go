package runner

import (
	"testing"

	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
)

func TestMakeJobName(t *testing.T) {
	jobName := makeJobName("diName", &vahkanev1.DiscordInteractionAction{Name: "action"})
	if jobName != "job-59nc-t8hpbbznshuxk3sj6464emuiu8eob6y08lsrn1" {
		t.Errorf("makeJobName returns unexpected value: %s", jobName)
	}
}
