package main

import (
	"github.com/PastureStack/catalog-service/cmd"
	_ "github.com/PastureStack/catalog-service/signals"
	log "github.com/sirupsen/logrus"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
