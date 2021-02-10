package sqsjfr_test

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
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
	testResults = testResults[0:0]

	opt := sqsjfr.Option{CrontabURL: "tests/crontab"}
	f, err := opt.ReadCrontabFile()
	if err != nil {
		t.Error(err)
	}
	c, envs, err := sqsjfr.ReadCrontab(f, newJob)
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

	if envs["FOO"] != `foo " foo` {
		t.Error("unexpected FOO", envs["FOO"])
	}
	if envs["BAR"] != `bar` {
		t.Error("unexpected BAR", envs["BAR"])
	}
}

func TestReadCrontabFail(t *testing.T) {
	testResults = testResults[0:0]

	opt := sqsjfr.Option{CrontabURL: "tests/crontab.bad"}
	f, err := opt.ReadCrontabFile()
	if err != nil {
		t.Error(err)
	}
	c, _, err := sqsjfr.ReadCrontab(f, newJob)
	t.Log(err)
	if err == nil {
		t.Error("must be failed")
	}
	if c != nil {
		t.Error("must be nil when failed")
	}
}

func TestReadCrontabFailEnv(t *testing.T) {
	testResults = testResults[0:0]

	opt := sqsjfr.Option{CrontabURL: "tests/crontab.badenv"}
	f, err := opt.ReadCrontabFile()
	if err != nil {
		t.Error(err)
	}
	c, envs, err := sqsjfr.ReadCrontab(f, newJob)
	t.Log(err)
	if err == nil {
		t.Error("must be failed")
	}
	if !strings.HasPrefix(err.Error(), "error on line 8:") {
		t.Error("unexpected error message", err)
	}
	if c != nil {
		t.Error("must be nil when failed")
	}
	if envs != nil {
		t.Error("must be nil when failed")
	}
}

func TestReadCrontabHTTP(t *testing.T) {
	testResults = testResults[0:0]

	h := http.FileServer(http.Dir("."))
	ts := httptest.NewServer(h)
	defer ts.Close()
	t.Logf("testing URL %s", ts.URL)

	opt := sqsjfr.Option{CrontabURL: ts.URL + "/tests/crontab"}
	f, err := opt.ReadCrontabFile()
	if err != nil {
		t.Error(err)
	}
	c, envs, err := sqsjfr.ReadCrontab(f, newJob)
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

	if envs["FOO"] != `foo " foo` {
		t.Error("unexpected FOO", envs["FOO"])
	}
	if envs["BAR"] != `bar` {
		t.Error("unexpected BAR", envs["BAR"])
	}
}
