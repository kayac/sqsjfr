package main

import (
	"context"
	"log"
	"os"

	"github.com/kayac/go-config"
	"github.com/kayac/sqsjfr"
)

func main() {
	err := _main()
	if err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}
}

func _main() error {
	var conf sqsjfr.Config
	if err := config.LoadWithEnv(&conf, "config.yaml"); err != nil {
		return err
	}
	app, err := sqsjfr.New(context.Background(), &conf)
	if err != nil {
		return err
	}
	app.Run()
	return nil
}
