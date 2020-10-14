package sqsjfr

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kayac/go-config"
)

type environments map[string]string

func (e environments) String() string {
	b, _ := json.Marshal(e)
	return string(b)
}

type message struct {
	Body      map[string]interface{} `json:"-"`
	Command   string                 `json:"command"`
	InvokedAt int64                  `json:"invoked_at"`
	EntryID   int                    `json:"entry_id"`
	Env       environments           `json:"envs"`
}

func (m message) String() string {
	var b strings.Builder
	if m.Body != nil {
		json.NewEncoder(&b).Encode(m.Body)
	} else {
		json.NewEncoder(&b).Encode(m)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (m message) DeduplicationID() string {
	h := sha256.New()
	h.Write([]byte(m.String()))
	h.Write([]byte(strconv.FormatInt(m.InvokedAt, 10)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func newMessage(command, messageTemplate string, now time.Time, envs environments) (*message, error) {
	min := now.Truncate(time.Minute)
	msg := message{
		Command:   command,
		InvokedAt: min.Unix(),
		Env:       envs,
	}
	if messageTemplate == "" {
		return &msg, nil
	}

	loader := config.New()
	loader.Data = msg
	if err := loader.LoadWithEnvJSON(&msg.Body, messageTemplate); err != nil {
		return nil, fmt.Errorf("failed to create message with template: %s", err)
	}
	return &msg, nil
}
