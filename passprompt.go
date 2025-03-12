package main

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcwallet/internal/prompt"
	"github.com/btcsuite/btcwallet/wallet"
)

func passPrompt() (passphrase []byte, err error) {
	quit := make(chan struct{})
	addInterruptHandler(func() {
		close(quit)
	})

	os.Stdout.Sync()
	c := make(chan struct{}, 1)
	go func() {
		passphrase, err = prompt.ProvidePrivPassphrase()
		c <- struct{}{}
	}()
	select {
	case <-quit:
		return nil, fmt.Errorf("shutting down")
	case <-c:
		return passphrase, err
	}
}

// startPromptPass prompts the user for a password to unlock their wallet in
// the event that it was restored from seed or --promptpass flag is set.
func startPromptPass(w *wallet.Wallet) []byte {
	promptPass := cfg.PromptPass

	// Watching only wallets never require a password.
	if w.Manager.WatchOnly() {
		return nil
	}

	if cfg.MixChange {
		promptPass = true
	}

	if !promptPass {
		return nil
	}

	for {
		passphrase, err := passPrompt()
		if err != nil {
			return nil
		}

		err = w.Unlock(passphrase, nil)
		if err != nil {
			fmt.Println("Incorrect password entered. Please " +
				"try again.")
			continue
		}
		return passphrase
	}
}
