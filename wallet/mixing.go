// Copyright (c) 2019-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mixing"
	"github.com/btcsuite/btcd/mixing/mixpool"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/walletdb"
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

	return walletdb.View(wallet.db, func(dbtx walletdb.ReadTx) error {
		addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)

		// Set up our callbacks that we pass to txscript so it can
		// look up the appropriate keys and scripts by address.
		getKey := txscript.KeyClosure(func(addr btcutil.Address) (*btcec.PrivateKey, bool, error) {
			address, err := w.Manager.Address(addrmgrNs, addr)
			if err != nil {
				return nil, false, err
			}

			pka, ok := address.(waddrmgr.ManagedPubKeyAddress)
			if !ok {
				return nil, false, fmt.Errorf("address %v is not "+
					"a pubkey address", address.Address().EncodeAddress())
			}

			key, err := pka.PrivKey()
			if err != nil {
				return nil, false, err
			}

			return key, pka.Compressed(), nil
		})
		getScript := txscript.ScriptClosure(func(addr btcutil.Address) ([]byte, error) {
			address, err := w.Manager.Address(addrmgrNs, addr)
			if err != nil {
				return nil, err
			}
			sa, ok := address.(waddrmgr.ManagedScriptAddress)
			if !ok {
				return nil, errors.New("address is not a script" +
					" address")
			}

			return sa.Script()
		})

		script, err := txscript.SignTxOutput(wallet.chainParams,
			tx, index, prevScript, txscript.SigHashAll, getKey,
			getScript, in.SignatureScript)
		if err != nil {
			return err
		}
		in.SignatureScript = script

		return nil
	})
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
