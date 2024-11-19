package runner

import (
	"testing"

	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
)

func TestMakeJobName(t *testing.T) {
	jobName := makeJobName("diName", &vahkanev1.DiscordInteractionAction{Name: "action"})
	if jobName != "job-5c9aordx16dqd2mgii4bcliw986krokvd9e9oz4gt65" {
		t.Errorf("makeJobName returns unexpected value: %s", jobName)
	}
}
