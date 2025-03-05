module github.com/btcsuite/btcwallet

require (
	decred.org/cspp/v2 v2.4.0
	github.com/btcsuite/btcd v0.24.3-0.20250318170759-4f4ea81776d6
	github.com/btcsuite/btcd/btcec/v2 v2.3.5
	github.com/btcsuite/btcd/btcutil v1.1.6
	github.com/btcsuite/btcd/btcutil/psbt v1.1.10
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0
	github.com/btcsuite/btclog v1.0.0
	github.com/btcsuite/btcwallet/wallet/txauthor v1.3.5
	github.com/btcsuite/btcwallet/wallet/txrules v1.2.2
	github.com/btcsuite/btcwallet/wallet/txsizes v1.2.5
	github.com/btcsuite/btcwallet/walletdb v1.5.1
	github.com/btcsuite/btcwallet/wtxmgr v1.5.6
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0
	github.com/decred/dcrd/mixing v0.7.3
	github.com/golang/protobuf v1.5.4
	github.com/jessevdk/go-flags v1.6.1
	github.com/jrick/logrotate v1.1.2
	github.com/lightninglabs/gozmq v0.0.0-20191113021534-d20a764486bf
	github.com/lightninglabs/neutrino v0.16.2
	github.com/lightninglabs/neutrino/cache v1.1.3
	github.com/lightningnetwork/lnd/fn/v2 v2.0.8
	github.com/lightningnetwork/lnd/ticker v1.1.1
	github.com/lightningnetwork/lnd/tlv v1.3.2
	github.com/stretchr/testify v1.10.0
	golang.org/x/crypto v0.45.0
	golang.org/x/net v0.47.0
	golang.org/x/sync v0.18.0
	golang.org/x/term v0.37.0
	google.golang.org/grpc v1.73.0
	google.golang.org/protobuf v1.36.6
)

require (
	decred.org/dcrwallet/v4 v4.3.1 // indirect
	github.com/aead/siphash v1.0.1 // indirect
	github.com/btcsuite/btcd/v2transport v1.0.1 // indirect
	github.com/btcsuite/go-socks v0.0.0-20170105172521-4720035b7bfd // indirect
	github.com/companyzero/sntrup4591761 v0.0.0-20220309191932-9e0f3af2f07a // indirect
	github.com/decred/dcrd/chaincfg/chainhash v1.0.5 // indirect
	github.com/decred/dcrd/chaincfg/v3 v3.3.0 // indirect
	github.com/decred/dcrd/container/lru v1.0.0 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/crypto/rand v1.0.1 // indirect
	github.com/decred/dcrd/lru v1.1.2 // indirect
	github.com/decred/dcrd/wire v1.7.5 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/kkdai/bstream v1.0.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lightningnetwork/lnd/clock v1.0.1 // indirect
	github.com/lightningnetwork/lnd/queue v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	go.etcd.io/bbolt v1.3.11 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	golang.org/x/exp v0.0.0-20250811191247-51f88131bc50 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)

// If you change this please run `make lint` to see where else it needs to be
// updated as well.
go 1.24.6

replace (
	github.com/btcsuite/btcd => ../btcd
	github.com/lightninglabs/neutrino => ../neutrino
)
