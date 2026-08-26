// Package main runs Momobase with the optional MTN Mobile Money provider.
//
// Provider credentials are not read by this example. After the server starts,
// create an MTN provider account through the Admin API and store the encrypted
// provider configuration there. See providers/mtn/README.md for the recognized
// configuration keys.
package main

import (
	"log"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/providers/mtn"
)

func main() {
	instance, err := momobase.New(
		momobase.WithProvider("mtn", mtn.New),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if closeErr := instance.Close(); closeErr != nil {
			log.Printf("close momobase: %v", closeErr)
		}
	}()

	if err := instance.Run(); err != nil {
		log.Fatal(err)
	}
}
