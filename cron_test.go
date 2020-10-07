package sqsjfr_test

import (
	"log"
	"os"
	"testing"

	"github.com/kayac/sqsjfr"
	"github.com/robfig/cron/v3"
)

var testResults []string

type testJob struct {
	command string
}

func (j *testJob) Run() {
	log.Println("[invoke]", j.command)
	testResults = append(testResults, "result of "+j.command)
}

func newJob(command string) cron.Job {
	return &testJob{command: command}
}

func TestReadCrontab(t *testing.T) {
	f, err := os.Open("tests/crontab")
	if err != nil {
		t.Error(err)
	}
	c, err := sqsjfr.ReadCrontab(f, newJob)
	if err != nil {
		t.Error(err)
	}
	if len(c.Entries()) != 2 {
		t.Errorf("unexpected loaded entries len %d", len(c.Entries()))
	}
	for _, entry := range c.Entries() {
		entry.Job.Run()
	}
	if len(testResults) != 2 {
		t.Errorf("unexpected entries len %d", len(testResults))
	}
	if testResults[0] != `result of echo "hello world!"` {
		t.Error("unexpected test result[0]", testResults[0])
	}
	if testResults[1] != `result of date` {
		t.Error("unexpected test result[1]", testResults[1])
	}
}

func TestReadCrontabFail(t *testing.T) {
	f, err := os.Open("tests/crontab.bad")
	if err != nil {
		t.Error(err)
	}
	c, err := sqsjfr.ReadCrontab(f, newJob)
	t.Log(err)
	if err == nil {
		t.Error("must be failed")
	}
	if c != nil {
		t.Error("must be nil when failed")
	}
}
