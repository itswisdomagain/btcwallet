package main

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
)

var defaultRPCClientCfg = &rpcclient.ConnConfig{
	Host: "159.65.29.55:18334",
	User: "user",
	Pass: "pass",
	Certificates: []byte(`-----BEGIN CERTIFICATE-----
MIICwDCCAiGgAwIBAgIQd/MZ+H/WZIpNzjUGpPJkwzAKBggqhkjOPQQDBDA/MSAw
HgYDVQQKExdidGNkIGF1dG9nZW5lcmF0ZWQgY2VydDEbMBkGA1UEAxMSdWJ1bnR1
LWMtMi1sb24xLTAxMB4XDTI1MDUwNDEzNDExNFoXDTM1MDUwMzEzNDExNFowPzEg
MB4GA1UEChMXYnRjZCBhdXRvZ2VuZXJhdGVkIGNlcnQxGzAZBgNVBAMTEnVidW50
dS1jLTItbG9uMS0wMTCBmzAQBgcqhkjOPQIBBgUrgQQAIwOBhgAEALdR8kDxYvol
EWfAglkrtRV1jlDfYVKsPOhZRMHkDVSgB7k5EckPL7iGr5J2wMHjYFymjM5xoRyX
IZE96e3hAQEuAGfX3hRgtDQUqMLniJOE7bv5KvVGkA429YYt2GNejetJdoJN9yql
hYQXOMV38jkJPgtbX6Jd2stvkrs9DKrw/iP2o4G7MIG4MA4GA1UdDwEB/wQEAwIC
pDAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBQ9Cll1aM/8VGRWaLQ8Be/ikrJy
/DB2BgNVHREEbzBtghJ1YnVudHUtYy0yLWxvbjEtMDGCCWxvY2FsaG9zdIcEfwAA
AYcQAAAAAAAAAAAAAAAAAAAAAYcEn0EdN4cEChAABocECmoAA4cQ/oAAAAAAAAAg
X1f//vVmd4cQ/oAAAAAAAAB4mRj//vCmpTAKBggqhkjOPQQDBAOBjAAwgYgCQgEP
yL7GF7l9vinSOzms6BO9ioiBHVvR7j7mmAXi/yLNF3zAaejTzVZgLKsq8bvJ6LdS
u9gTD8VGtZHwrRKBFzWqaQJCAZJ1RE2Hr4sYJOK0OBJFK6defDbou+WTJINPd9T4
ugZprz4E8X4hV9l8SfJokxNCZMKWkcl716OFTZaBlCRn8kZL
-----END CERTIFICATE-----`),
	HTTPPostMode: true,
}

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
	if rpcHost == "" {
		connCfg = defaultRPCClientCfg
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
