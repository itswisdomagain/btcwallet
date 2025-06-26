# btc peer-to-peer coin mixing implementation

## High Level Summary

The modifications in [this branch](https://github.com/itswisdomagain/btcwallet/tree/mixing), together with related changes in [btcd](https://github.com/itswisdomagain/btcd/tree/mixing) and [neutrino](https://github.com/itswisdomagain/neutrino/tree/mixing) are an adaptation of [Decred's CoinShuffle++ implementation](https://docs.decred.org/privacy/cspp/overview/) and introduce bitcoin support for p2p coin mixing.

In order to perform mixes, wallets must be connected to a btcd node that is capable of receiving and propagating mix messages between wallets/peers. Wallets can connect to btcd via SPV or RPC. A wallet can connect to multiple SPV peers and be able to mix as long as at least one of the connected SPV peers is a btcd node that supports mix message propagation between peers.

Mixes can currently only be performed in testnet; and the btcwallet codebase has hardcorded details of a testnet btcd node that supports mix message propagation between peers. When running the wallet in SPV mode, this testnet btcd node is automatically added as an SPV peer to ensure that the wallet can perform mixes with other wallets in the network. Alternatively, a wallet may use RPC for chain synchronization rather than SPV. Since the details (rpc address, username, password and cert) of the testnet btcd node are hardcoded in the codebase, users don't need to specify an `rpcconnect` address in their wallet config; the testnet btcd node rpc connection details would be automatically used.

PS: The testnet btcd node harcoded in the btcwallet codebase has pruning enabled, and thus cannot be used to sync a wallet from scratch. It is recommended to connect to another btcd node via RPC or use SPV for initial chain synchronization before switching to the provideed testnet btcd node for RPC syncrhonization.

## How to mix

### Build btcwallet from source

Clone the source code, check out the `mixing` branch and build:

```bash
$ git clone https://github.com/itswisdomagain/btcwallet.git -b mixing
$ cd ./btcwallet
$ go build
```

### Download csppsolver

It is recommend, but not required, to build csppsolver and either install it to $PATH, or provide the path to the executable with the --csppsolver option. The solver is a necessary component to complete a mix, but only one participant in the mix is required to provide it. The solver requires the C library libflint (including its development headers, if your distribution creates separate -dev packages this way), and once these dependencies are met, csppsolver can be installed with:

```bash
$ go install decred.org/cspp/v2/cmd/csppsolver@latest
```

### Create and run your wallet

- Create a testnet wallet:

```bash
$ ./btcwallet --testnet --create
```

- Run the wallet with the following command-line args (modify as necessary) and allow the wallets to sync.

```bash
$ ./btcwallet --testnet --usespv -u "user" -P "pass" --mixing --mixchange --mixedaccount "mixed/1" --changeaccount "default"
```

- Alternatively, save the above command-line args to the config file and simply run `./btcwallet`.

- Once the wallet is fully synced, mixing will occur every time a new block is mined, as long as there are mixable outputs in the `changeaccount`. See the "Additional Information" section below for how to fund your new wallet with test coins.

## Additional information

### Funding your test wallet

Testnet wallets can be funded using a testnet faucet such as https://coinfaucet.eu/en/btc-testnet/.

First, generate a receive address from the wallet using `btcctl`.

- Install `btcctl` from the updated repo:

```bash
$ git clone https://github.com/itswisdomagain/btcd.git -b mixing
$ cd ./btcd
$ go install ./cmd/btcctl
```

- Request a receive address using `btcctl`. Pass the rpclisten address, username, password and cert used for the wallet.

```bash
$ btcctl --wallet --testnet -s 127.0.0.2:18332 -u user -P pass -c ~/.btcwallet2/rpc.cert getaccountaddress "default"
```

### Number of required peers per mix

- Mixing occurs when at least 4 wallets (peers) submit a request to mix coins into the same mixed denomination. For test purposes, this minimum peer requirement is reduced to 2. Put plainly, at least 2 wallets must have submitted requests to mix coins into the same denomination for the mix to occur. Otherwise, a wallet attempting to perform a mix alone will produce a "minimum peer requirement unmet" log.

- It may be necessary to run a second btcwallet instance if there are no other active mix peers at the time of testing. The second wallet must be funded with an output of similar amount as the first wallet, so that both wallets can attempt a mix into the same denomination. Also, at least one of the wallets must have access to the csppsolver binary (see above).

- To run a separate btcwallet instances on the same computer, pass a separate appdata directory and rpclisten address to the second instance. E.g.:

```bash
$ ./btcwallet --testnet --appdata ~/.btcwallet2 --rpclisten 127.0.0.2:18332 ...
```
