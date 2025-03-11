// Copyright (c) 2019-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mixing"
	"github.com/btcsuite/btcd/mixing/mixpool"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
)

// mixingWallet implements the mixclient.Wallet interface.
type mixingWallet Wallet

// BestBlock returns the wallet's current best tip block height and hash.
func (w *mixingWallet) BestBlock() (uint32, chainhash.Hash) {
	wallet := (*Wallet)(w)
	chainClient, err := wallet.requireChainClient()
	if err != nil {
		return 0, chainhash.Hash{}
	}
	hash, height, err := chainClient.GetBestBlock()
	if err != nil {
		return 0, chainhash.Hash{}
	}
	return uint32(height), *hash
}

// Mixpool returns access to the wallet's mixing message pool.
//
// The mixpool should only be used for message access and deletion,
// but never publishing; SubmitMixMessage must be used instead for
// message publishing.
func (w *mixingWallet) Mixpool() *mixpool.Pool {
	wallet := (*Wallet)(w)
	return wallet.mixpool
}

// SubmitMixMessage submits a mixing message to the wallet's mixpool
// and broadcasts it to the network.
func (w *mixingWallet) SubmitMixMessage(ctx context.Context, msg mixing.Message) (err error) {
	wallet := (*Wallet)(w)

	cc, err := wallet.requireChainClient()
	if err != nil {
		return err
	}

	chainClient, ok := cc.(chain.MixingInterface)
	if !ok {
		return fmt.Errorf("wallet backend does not support mixing")
	}

	defer func() {
		if err != nil {
			wallet.mixpool.RemoveMessage(msg)
		}
	}()

	_, err = wallet.mixpool.AcceptMessage(msg)
	if err != nil {
		return err
	}

	err = chainClient.PublishMixMessages(msg)
	if err != nil {
		log.Errorf("Failed to publish mix transaction: %v", err)
	}
	return err
}

// SignInput adds a signature script to a transaction input.
func (w *mixingWallet) SignInput(tx *wire.MsgTx, index int, prevScript []byte) error {
	wallet := (*Wallet)(w)
	in := tx.TxIn[index]

	privKey, compressed, privKeyDone, err := wallet.privateKey(prevScript)
	if err != nil {
		return err
	}

	defer privKeyDone()
	sigscript, err := txscript.SignatureScript(tx, index, prevScript,
		txscript.SigHashAll, privKey, compressed)
	if err != nil {
		return fmt.Errorf("txscript.SignatureScript error: %w", err)
	}

	in.SignatureScript = sigscript
	return nil
}

// PublishTransaction adds the transaction to the wallet and publishes
// it to the network.
func (w *mixingWallet) PublishTransaction(ctx context.Context, tx *wire.MsgTx) error {
	wallet := (*Wallet)(w)
	chainClient, err := wallet.requireChainClient()
	if err != nil {
		return err
	}

	_, err = chainClient.SendRawTransaction(tx, true) // TODO: allowHighFees?
	if err != nil {
		log.Errorf("Failed to publish mix transaction: %v", err)
	}
	return err
}
