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
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/mixing"
	"github.com/btcsuite/btcd/mixing/mixclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet/txrules"
	"github.com/btcsuite/btcwallet/wallet/txsizes"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/decred/go-socks/socks"
	"golang.org/x/crypto/ripemd160"
	"golang.org/x/sync/errgroup"
)

var (
	errNoSplitDenomination = errors.New("no suitable split denomination")
	errThrottledMixRequest = errors.New("throttled mix request for split denomination")
)

// must be sorted large to small
var splitPoints = [...]btcutil.Amount{
	1 << 36, // 687.19476736
	1 << 34, // 171.79869184
	1 << 32, // 042.94967296
	1 << 30, // 010.73741824
	1 << 28, // 002.68435456
	1 << 26, // 000.67108864
	1 << 24, // 000.16777216
	1 << 22, // 000.04194304
	1 << 20, // 000.01048576
	1 << 18, // 000.00262144
	1 << 16, // 000.00065536 // TODO: Remove
}

func estimateSerializeSizeFromScriptSizes(inputSizes []int, outputSizes []int, changeScriptSize int) int {
	outputs := make([]*wire.TxOut, len(outputSizes))
	for i := range outputSizes {
		outputs[i] = wire.NewTxOut(0, make([]byte, outputSizes[i]))
	}
	addChangeOutput := changeScriptSize > 0
	return txsizes.EstimateSerializeSize(len(inputSizes), outputs, addChangeOutput)
}

func smallestMixChange(feeRate btcutil.Amount) btcutil.Amount {
	inScriptSizes := []int{txsizes.RedeemP2PKHSigScriptSize}
	outScriptSizes := []int{txsizes.P2PKHPkScriptSize}
	size := estimateSerializeSizeFromScriptSizes(inScriptSizes, outScriptSizes, 0)
	fee := txrules.FeeForSerializeSize(feeRate, size)
	return fee + splitPoints[len(splitPoints)-1]
}

type mixSemaphores struct {
	splitSems [len(splitPoints)]chan struct{}
}

func newMixSemaphores(n int) mixSemaphores {
	var m mixSemaphores
	for i := range m.splitSems {
		m.splitSems[i] = make(chan struct{}, n)
	}
	return m
}

// Hash160er is an interface that allows the RIPEMD-160 hash to be obtained from
// addresses that involve them.
type Hash160er interface {
	Hash160() *[ripemd160.Size]byte
}

func (w *Wallet) makeGen(account, branch uint32) mixclient.GenFunc {
	gen := func(mcount uint32) (wire.MixVect, error) {
		acceptAddress := func(mixAddr waddrmgr.ManagedAddress) bool {
			_, ok := mixAddr.Address().(Hash160er)
			if !ok {
				log.Infof("address %T does not have hash160 method", mixAddr)
			}
			return ok
		}

		addresses, err := w.NewAddresses(account, branch, mcount, waddrmgr.KeyScopeBIP0044, acceptAddress)
		if err != nil {
			return nil, err
		}

		gen := make(wire.MixVect, mcount)
		for i := uint32(0); i < mcount; i++ {
			hash160er := addresses[i].(Hash160er)
			gen[i] = *hash160er.Hash160()
		}

		return gen, nil
	}
	return mixclient.GenFunc(gen)
}

func dicemixExpiry(chainClient chain.Interface, chainParams *chaincfg.Params) (uint32, error) {
	_, height, err := chainClient.GetBestBlock()
	if err != nil {
		return 0, err
	}
	return mixing.MaxExpiry(uint32(height), chainParams) - 2, nil
}

// addCoinJoinInput adds a wallet's controlled UTXO to the coinjoin
// transaction.  This method looks up the private key of the previous output
// to create the UTXO signature proof and requires the wallet or account to be
// unlocked.
func (w *Wallet) addCoinJoinInput(cj *mixclient.CoinJoin,
	input *wire.TxIn, prevScript []byte, prevScriptVersion uint16, value int64) error {
	privKey, _, privKeyDone, err := w.privateKey(prevScript)
	if err != nil {
		return err
	}

	err = cj.AddInput(input, value, prevScript, prevScriptVersion, privKey)
	privKeyDone()
	return err
}

func (w *Wallet) privateKey(prevScript []byte) (*btcec.PrivateKey, bool, func(), error) {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(prevScript, w.chainParams)
	if err != nil {
		return nil, false, nil, err
	}
	if len(addrs) != 1 {
		return nil, false, nil, fmt.Errorf("previous output is not P2PKH")
	}
	prevP2PKH, ok := addrs[0].(*btcutil.AddressPubKeyHash)
	if !ok {
		return nil, false, nil, fmt.Errorf("previous output is not P2PKH")
	}

	var compressed bool
	var privKey *btcec.PrivateKey
	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		addrmgrNs := tx.ReadBucket(waddrmgrNamespaceKey)
		ma, err := w.Manager.Address(addrmgrNs, prevP2PKH)
		if err != nil {
			return err
		}

		// Only those addresses with keys needed.
		pka, ok := ma.(waddrmgr.ManagedPubKeyAddress)
		if !ok {
			return fmt.Errorf("no privkey found for address")
		}

		compressed = pka.Compressed()
		privKey, err = pka.PrivKey()
		return err
	})
	if err != nil {
		if privKey != nil {
			privKey.Zero()
		}
		return nil, false, nil, err
	}

	return privKey, compressed, privKey.Zero, nil
}

// MixOutput performs a mix of a single output into standard sized outputs
// under the current ticket price.
func (w *Wallet) MixOutput(output *wire.OutPoint, changeAccount, mixAccount, mixBranch uint32,
	feeRate btcutil.Amount) error {

	makeError := func(s string, values ...interface{}) error {
		values = append([]any{output}, values...)
		return fmt.Errorf("wallet.MixOutput(%v): "+s, values...)
	}

	// Mixing requests require wallet mixing support.
	if !w.mixing {
		return makeError("wallet mixing support is disabled")
	}

	chainClient, err := w.requireChainClient()
	if err != nil {
		return err
	}

	w.lockedOutpointsMtx.Lock()
	if _, exists := w.lockedOutpoints[*output]; exists {
		w.lockedOutpointsMtx.Unlock()
		return makeError("output %v already locked", output)
	}

	var prevScript []byte
	var prevScriptVersion uint16
	var amount btcutil.Amount
	err = walletdb.View(w.db, func(dbtx walletdb.ReadTx) error {
		txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)
		txDetails, err := w.TxStore.TxDetails(txmgrNs, &output.Hash)
		if err != nil {
			return err
		}
		out := txDetails.MsgTx.TxOut[output.Index]
		prevScript = out.PkScript
		// prevScriptVersion = out.Version
		amount = btcutil.Amount(txDetails.MsgTx.TxOut[output.Index].Value)
		return nil
	})
	if err != nil {
		w.lockedOutpointsMtx.Unlock()
		return makeError("%w", err)
	}
	w.lockedOutpoints[*output] = struct{}{}
	w.lockedOutpointsMtx.Unlock()

	defer func() {
		w.lockedOutpointsMtx.Lock()
		delete(w.lockedOutpoints, *output)
		w.lockedOutpointsMtx.Unlock()
	}()

	var i int
	var count uint32
	var mixValue, remValue, changeValue btcutil.Amount
	var smallestMixChange = smallestMixChange(feeRate)
SplitPoints:
	for i = 0; i < len(splitPoints); i++ {
		last := i == len(splitPoints)-1
		mixValue = splitPoints[i]

		count = uint32(amount / mixValue)
		if count > 4 {
			count = 4
		}
		for ; count > 0; count-- {
			remValue = amount - btcutil.Amount(count)*mixValue
			if remValue < 0 {
				continue
			}

			// Determine required fee and change value, if possible.
			// No change is ever included when mixing at the
			// smallest amount.
			const P2PKHv0Len = 25
			inScriptSizes := []int{txsizes.RedeemP2PKHSigScriptSize}
			outScriptSizes := make([]int, count)
			for i := range outScriptSizes {
				outScriptSizes[i] = P2PKHv0Len
			}
			size := estimateSerializeSizeFromScriptSizes(
				inScriptSizes, outScriptSizes, P2PKHv0Len)
			fee := txrules.FeeForSerializeSize(feeRate, size)
			changeValue = remValue - fee
			if last {
				changeValue = 0
			}
			if changeValue <= 0 {
				// Determine required fee without a change
				// output.  A lower mix count or amount is
				// required if the fee is still not payable.
				size = estimateSerializeSizeFromScriptSizes(
					inScriptSizes, outScriptSizes, 0)
				fee = txrules.FeeForSerializeSize(feeRate, size)
				if remValue < fee {
					continue
				}
				changeValue = 0
			}
			if changeValue < smallestMixChange {
				changeValue = 0
			}

			break SplitPoints
		}
	}
	if i == len(splitPoints) {
		return makeError("output %v (%v): %w", output, amount, errNoSplitDenomination)
	}
	select {
	case <-w.quitChan():
		return fmt.Errorf("wallet is shutting down or has shut down")
	case w.mixSems.splitSems[i] <- struct{}{}:
		// Indicate that we're about to start up a new mix connection for the split
		// amount at splitPoints[i]. There is a maximum number of connections
		// allowed per amount which is equal to the channel capacity for each split
		// amount. When the channel becomes full (too many active split requests for
		// an amount), this will block (until a previous connection/request ends)
		// and this method will return errThrottledMixRequest. If the channel is
		// able to accept a new value however, register a defer fn to remove the
		// value once this method returns.
		defer func() { <-w.mixSems.splitSems[i] }()
	default:
		return errThrottledMixRequest
	}

	var change *wire.TxOut
	if changeValue > 0 {
		addr, err := w.NewChangeAddress(changeAccount, waddrmgr.KeyScopeBIP0044)
		if err != nil {
			return makeError("%w", err)
		}

		changeScript, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return makeError("change address error: %w", err)
		}

		change = &wire.TxOut{
			Value:    int64(changeValue),
			PkScript: changeScript,
			// Version:  version,
		}
	}

	log.Infof("Mixing output %v (%v)", output, amount)

	expires, err := dicemixExpiry(chainClient, w.chainParams)
	if err != nil {
		return makeError("dicemixExpiry error: %w", err)
	}

	gen := w.makeGen(mixAccount, mixBranch)
	cj := mixclient.NewCoinJoin(gen, change, int64(mixValue), expires, count)
	input := wire.NewTxIn(output, nil, nil)
	err = w.addCoinJoinInput(cj, input, prevScript, prevScriptVersion, int64(amount))
	if err != nil {
		return makeError("addCoinJoinInput error: %w", err)
	}

	// TODO: Get a ctx that is canceled when the wallet is about to be shutdown.
	err = w.mixClient.Dicemix(context.TODO(), cj)
	if err != nil {
		return makeError("mixClient.Dicemix error: %w", err)
	}

	tx := cj.Tx()
	cjHash := tx.TxHash()
	log.Infof("Completed CoinShuffle++ mix of output %v in transaction %v", output, &cjHash)
	return nil
}

// MixAccount individually mixes outputs of an account into standard
// denominations, creating newly mixed outputs for a mixed account.
//
// Due to performance concerns of timing out in a CoinShuffle++ run, this
// function may throttle how many of the outputs are mixed each call.
func (w *Wallet) MixAccount(changeAccount, mixAccount, mixBranch uint32, feeRate btcutil.Amount) error {
	// Mixing requests require wallet mixing support.
	if !w.mixing {
		return fmt.Errorf("wallet.MixAccount: wallet mixing support is disabled")
	}

	chainClient, err := w.requireChainClient()
	if err != nil {
		return err
	}

	// Get current block's height and hash.
	bs, err := chainClient.BlockStamp()
	if err != nil {
		return err
	}

	var credits []wtxmgr.Credit
	err = walletdb.View(w.db, func(dbtx walletdb.ReadTx) error {
		var minAmount = splitPoints[len(splitPoints)-1]
		var maxResults = cap(w.mixSems.splitSems[0]) * len(splitPoints)
		var foundUtxosCount int
		allowUtxo := func(utxo wtxmgr.Credit) bool {
			if utxo.Amount < minAmount {
				return false
			}
			if foundUtxosCount+1 >= maxResults {
				return false
			}
			return true
		}

		var err error
		const minconf = 2
		credits, err = w.findEligibleOutputs(dbtx, &waddrmgr.KeyScopeBIP0044, changeAccount, minconf,
			bs, allowUtxo)
		return err
	})
	if err != nil {
		return fmt.Errorf("wallet.MixAccount: %w", err)
	}

	var g errgroup.Group
	for i := range credits {
		op := &credits[i].OutPoint
		g.Go(func() error {
			err := w.MixOutput(op, changeAccount, mixAccount, mixBranch, feeRate)
			if errors.Is(err, errThrottledMixRequest) {
				return nil
			}
			if errors.Is(err, errNoSplitDenomination) {
				return nil
			}
			if errors.Is(err, socks.ErrPoolMaxConnections) {
				return nil
			}
			return err
		})
	}
	err = g.Wait()
	if err != nil {
		return fmt.Errorf("wallet.MixAccount: %w", err)
	}
	return nil
}

func (w *Wallet) StartAutoMixer(changeAccount, mixAccount, mixBranch uint32, feeRate btcutil.Amount) error {
	c := w.NtfnServer.TransactionNotifications()
	defer c.Done()

	for {
		select {
		case <-w.quitChan():
			return fmt.Errorf("wallet is shutting down or has shut down")
		case n := <-c.C:
			if len(n.AttachedBlocks) == 0 {
				continue
			}

			// Don't perform any actions while transactions are not synced through
			// the tip block.
			// TODO: w.ChainSynced() isn't really reliable.
			if !w.ChainSynced() {
				log.Debugf("Skipping automixer actions: transactions are not synced")
				continue
			}

			go func() {
				err := w.MixAccount(changeAccount, mixAccount, mixBranch, feeRate)
				if err != nil {
					log.Error(err)
				}
			}()
		}
	}
}
