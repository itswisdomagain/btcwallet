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
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mixing"
	"github.com/btcsuite/btcd/mixing/mixclient"
	"github.com/btcsuite/btcd/mixing/mixpool"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet/txrules"
	"github.com/btcsuite/btcwallet/wallet/txsizes"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/decred/dcrd/crypto/ripemd160"
	"github.com/decred/go-socks/socks"
	"golang.org/x/sync/errgroup"
)

// MixMessage queries the mixpool for a message.  Only messages that have been
// recently inv'd should be queried.
func (w *Wallet) MixMessage(query *chainhash.Hash) (mixing.Message, error) {
	return w.mixpool.Message(query)
}

type mixpoolBlockchain Wallet

func (b *mixpoolBlockchain) CurrentTip() (chainhash.Hash, int64) {
	w := (*Wallet)(b)
	hash, height, err := w.ChainClient().GetBestBlock()
	if err != nil {
		return chainhash.Hash{}, 0
	}
	return *hash, int64(height)
}

func (b *mixpoolBlockchain) ChainParams() *chaincfg.Params {
	return (*Wallet)(b).ChainParams()
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

	chainClient, err := wallet.requireChainClient()
	if err != nil {
		return err
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

	return chainClient.PublishMixMessages(msg)
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

// must be sorted large to small
var splitPoints = [...]btcutil.Amount{
	1 << 20, // 000.01048576
	1 << 18, // 000.00262144
	1 << 16, // 000.00065536
	1 << 14, // 000.00016384
	1 << 12, // 000.00004096
	1 << 10, // 000.00001024
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

var (
	errNoSplitDenomination = errors.New("no suitable split denomination")
	errThrottledMixRequest = errors.New("throttled mix request for split denomination")
)

// MixOutput performs a mix of a single output into standard sized outputs
// under the current ticket price.
func (w *Wallet) MixOutput(ctx context.Context, output *wire.OutPoint, changeAccount, mixAccount, mixBranch uint32,
	feeRate btcutil.Amount) error {

	makeError := func(s string, values ...interface{}) error {
		values = append([]any{output}, values...)
		return fmt.Errorf("wallet.MixOutput(%v): "+s, values...)
	}

	// Mixing requests require wallet mixing support.
	if !w.mixingEnabled {
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
	// NOTE: use the minimum fee rate to determine smallestMixChange!
	// TODO: the smallestMixChange can be a constant since the min. fee rate is
	// a constant.
	var smallestMixChange = smallestMixChange(txrules.DefaultRelayFeePerKb)
SplitPoints:
	for i = 0; i < len(splitPoints); i++ {
		last := i == len(splitPoints)-1
		mixValue = splitPoints[i]

		count = min(uint32(amount/mixValue), 4)
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
		defer func() { <-w.mixSems.splitSems[i] }()
	default:
		return errThrottledMixRequest
	}

	// Skip mixing this input at this time if the fee would eat up a high
	// percentage of the input amount. Mixing would be re-attempted when the fee
	// rate drops.
	fee := amount - btcutil.Amount(count)*mixValue
	if changeValue > 0 {
		fee -= changeValue
	}
	feePercentage := fee.ToBTC() / amount.ToBTC()

	// Maximum allowed fee percentage is 60% for the lowest split denomination
	// and 30% for higher denominations.
	maxFeePercentage := 0.3
	if mixValue == splitPoints[len(splitPoints)-1] {
		maxFeePercentage = 0.6
	}
	if feePercentage > maxFeePercentage && feeRate > txrules.DefaultRelayFeePerKb {
		return makeError("fee rate too high (%d sats/kb), %.2f%% of input amount will be used as fees",
			feeRate, feePercentage*100)
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

	err = w.mixClient.Dicemix(ctx, cj)
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
func (w *Wallet) MixAccount(ctx context.Context, changeAccount, mixAccount, mixBranch uint32, feeRate btcutil.Amount) error {
	// Mixing requests require wallet mixing support.
	if !w.mixingEnabled {
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
			err := w.MixOutput(ctx, op, changeAccount, mixAccount, mixBranch, feeRate)
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

func (w *Wallet) StartAutoMixer(changeAccount, mixAccount, mixBranch uint32, fetchFeeRate func() (btcutil.Amount, error), maxFeeRate btcutil.Amount) error {
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
				// Fetch a fee rate to use from oracle.
				feeRate, err := fetchFeeRate()
				if err != nil {
					log.Errorf("Skipping automixer actions: cannot fetch fee rate: %v", err)
					return
				}

				if feeRate > maxFeeRate {
					log.Errorf("Skipping automixer actions: recommended fee rate (%s) > max fee rate (%s)",
						feeRate, maxFeeRate)
					return
				}

				err = w.MixAccount(w.mixCtx, changeAccount, mixAccount, mixBranch, feeRate)
				if err != nil {
					log.Error(err)
				}
			}()
		}
	}
}

// AcceptMixMessage adds a mixing message received from the network backend to
// the wallet's mixpool.
func (w *Wallet) AcceptMixMessage(msg mixing.Message) error {
	_, err := w.mixpool.AcceptMessage(msg)
	if err != nil {
		return err
	}

	return nil
}
