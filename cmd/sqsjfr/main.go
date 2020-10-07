package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hashicorp/logutils"
	"github.com/kayac/sqsjfr"
)

var trapSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGTERM,
}
var sigCh = make(chan os.Signal, 1)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

func main() {
	err := _main()
	if err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}
}

func _main() error {
	var opt sqsjfr.Option
	var logLevel string
	var dryRun bool

	flag.StringVar(&opt.QueueURL, "queue-url", "", "SQS queue URL")
	flag.StringVar(&opt.MessageTemplate, "message-template", "", "SQS message template(JSON)")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.BoolVar(&dryRun, "dry-run", false, "dry run")
	flag.VisitAll(envToFlag)
	flag.Parse()

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"debug", "info", "warn", "error"},
		MinLevel: logutils.LogLevel(logLevel),
		Writer:   os.Stderr,
	}
	log.SetOutput(filter)

	args := flag.Args()
	if len(args) != 1 {
		return errors.New("crontab is required")
	}
	opt.Path = args[0]
	log.Printf("[debug] option:%#v", opt)
	if err := opt.Validate(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signal.Notify(sigCh, trapSignals...)
	go func() {
		sig := <-sigCh
		log.Println("[info] got signal", sig)
		cancel()
	}()

	log.Println("[info] starting up")
	app, err := sqsjfr.New(ctx, &opt)
	if err != nil {
		return err
	}
	if dryRun {
		log.Println("[info] dry run OK")
		return nil
	}
	app.Run()
	return nil
}

func envToFlag(f *flag.Flag) {
	name := strings.ToUpper(strings.Replace(f.Name, "-", "_", -1))
	if s, ok := os.LookupEnv("SQSJFR_" + name); ok {
		f.Value.Set(s)
	}
}
