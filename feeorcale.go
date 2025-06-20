package main

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
)

type rpcFeeOracle struct {
	rpcClient *rpcclient.Client
}

func initFeeOracle(rpcHost, user, pass string, certs []byte, disableTLS bool) (*rpcFeeOracle, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         rpcHost,
		User:         user,
		Pass:         pass,
		Certificates: certs,
		DisableTLS:   disableTLS,
		HTTPPostMode: true,
	}

	rpcClient, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}

	return &rpcFeeOracle{rpcClient: rpcClient}, nil
}

func (oracle *rpcFeeOracle) RecommendedFeeRate() (btcutil.Amount, error) {
	feeRate, err := oracle.rpcClient.EstimateFee(6)
	if err != nil {
		return 0, err
	}
	return btcutil.NewAmount(feeRate)
}
