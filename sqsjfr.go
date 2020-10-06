package sqsjfr

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/robfig/cron/v3"
)

var reSpace = regexp.MustCompile("[ \t\n\v\f\r\u0085\u00A0]+")
var reTrimLeft = regexp.MustCompile("^[ \t\n\v\f\r\u0085\u00A0]+")

// App represents a sqsjfr application instance.
type App struct {
	config *Config
	cron   *cron.Cron
	sqs    *sqs.SQS
	ctx    context.Context
}

// New creates an App instance.
func New(ctx context.Context, conf *Config) (*App, error) {
	region, _, _, err := parseQueueURL(conf.QueueURL)
	if err != nil {
		return nil, err
	}
	awsConf := &aws.Config{
		Region: &region,
	}
	app := &App{
		config: conf,
		cron:   cron.New(),
		sqs:    sqs.New(session.New(), awsConf),
		ctx:    ctx,
	}
	if err := app.load(); err != nil {
		return nil, err
	}
	return app, nil
}

func (app *App) Run() {
	app.cron.Start()
	<-app.ctx.Done()
}

func (app *App) createInvokeFunc(command string) func() {
	return func() {
		message := app.config.MessageTemplate
		message.Command = command
		message.EventID = fmt.Sprintf("%x", sha256.Sum256([]byte(command)))
		log.Println("[invoke]", message.String())
		// TODO send to SQS
	}
}

func (app *App) load() error {
	f, err := os.Open(app.config.Path)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		line = reTrimLeft.ReplaceAllString(line, "")
		if line == "" || strings.HasPrefix(line, "#") { // skip
			continue
		}
		f := reSpace.Split(line, 6)
		if len(f) < 6 {
			log.Println("[warn] too few fields:", line)
			continue
		}
		spec := strings.Join(f[0:5], " ")
		command := f[5]
		id, err := app.cron.AddFunc(spec, app.createInvokeFunc(command))
		if err != nil {
			log.Println("[error] failed to add ", spec, err)
			continue
		}
		log.Println("[info] registered id", id, "schedule:", spec, "command:", command)
	}
	return nil
}
