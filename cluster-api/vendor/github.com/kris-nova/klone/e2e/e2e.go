package e2e

import (
	"github.com/kris-nova/klone/pkg/klone"
	"os"
)

func IdempotentKlone(path, query string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	err := klone.Klone(query)
	if err != nil {
		return err
	}
	return nil
}
