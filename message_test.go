package sqsjfr_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kayac/sqsjfr"
)

type testMessage struct {
	Command      string            `json:"command"`
	Environments map[string]string `json:"environments"`
	InvokedAt    int64             `json:"invokedAt"`
}

func TestNewMessage(t *testing.T) {
	shell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", shell)

	os.Setenv("SHELL", "/bin/zsh")
	now := time.Date(2020, 10, 7, 11, 22, 33, 123456, time.Local)
	nowMin := now.Truncate(time.Minute)
	envs := map[string]string{
		"FOO": `foo " foo`,
		"BAR": "bar",
	}
	msg, err := sqsjfr.NewMessage(`echo "hello world"`, "tests/message.json", now, envs)
	if err != nil {
		t.Error(err)
	}
	var res testMessage
	if err := json.Unmarshal([]byte(msg.String()), &res); err != nil {
		t.Error(err)
	}
	t.Log(msg.String())
	dupID := msg.DeduplicationID()
	if res.Command != `echo "hello world"` ||
		res.InvokedAt%60 != 0 ||
		res.InvokedAt < nowMin.Unix() ||
		res.InvokedAt > now.Unix() ||
		len(res.Environments) != 2 ||
		res.Environments["SHELL"] != "/bin/zsh" ||
		res.Environments["FOO"] != `foo " foo` {
		t.Errorf("unexpected encoded message JSON: %s", msg.String())
	}
	msg.Body["xxx"] = "yyy"
	if dupID == msg.DeduplicationID() {
		t.Error("duplication id must be changed when body modified")
	}
	delete(msg.Body, "xxx")
	if dupID != msg.DeduplicationID() {
		t.Error("duplication id must be equals")
	}
	msg.InvokedAt += 60
	if dupID == msg.DeduplicationID() {
		t.Error("duplication id must be changed when invokedAt modified")
	}
}
