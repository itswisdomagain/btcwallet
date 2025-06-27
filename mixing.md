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

### Network fees

Participants in a mix transaction contribute fees to the transaction. The minimum fee amount required per participant is calculated as `the total serialization size of the partipant's inputs and outputs (in kilobytes)` \* `minimum fee rate (i.e. 1000 sats/kb)`.

However, when the wallet is used to initiate a mix request for a participant, the minimum fee rate is NOT used in determining how much fee the participant should contribute. To ensure that the final mix transaction gets mined in a few blocks, each participant's fee contribution is determined as `the total serialization size of the partipant's inputs and outputs (in kilobytes)` \* `recommended fee rate (in sats/kb)`. The `recommended fee rate` is gotten by issuing the `estimatefee` rpc command to a btcd RPC server.

Thus, btcd rpc connection details are required for mixing, even if the wallet is synchronizing via SPV. The `mixing` branch contains default btcd rpc connection parameters that are used, unless separate values are provided in the wallet config.

A `maxfeerate` option has been added to the wallet config to protect against excessive fees recommended by the btcd rpc server. If the recommended fee rate exceeds the user-configured maxfeerate, mixing will be skipped until when the recommended fee rate falls below the maxfeerate.

### Changelog

#### btcd

- Add new wire message types for communicating mix messages between peers over the p2p network. ([6dd79bc](https://github.com/itswisdomagain/btcd/commit/6dd79bc5299a596b834f79d40bdd4a0ebb4624aa))
- Add `mixing/mixpool` package for storing mix messages in memory before they are eventually processed during a mix. ([f941250](https://github.com/itswisdomagain/btcd/commit/f941250b3651b3ce4b318ab5075ba0a8da8073c1))
- Add `mixing/mixclient` package to be used by wallets in performing mixes using mix messages that are stored in a mixpool. ([9f918fd](https://github.com/itswisdomagain/btcd/commit/9f918fd7a689f59649f0cfedc5659e2cc4c44000))
- Update the SyncManager to receive and process mix messages from peers and relay to other peers. ([a335bf7](https://github.com/itswisdomagain/btcd/commit/a335bf71739c93e9c1bad6d2e13c7e27140ce334))
- Add rpc commands to the rpc server for sending and retrieving mix messages and for requesting notifications when the rpc server receives mix messages from other peers. ([152c676](https://github.com/itswisdomagain/btcd/commit/152c676917b7504ab408143fe41307cfb15797bc))

#### neutrino

- Update the spv peer message handler to accept and process mix inventories and messages received from peers. ([b85a1bd](https://github.com/itswisdomagain/btcd/commit/b85a1bde83c977d1ac3f59320346f4eaa4f4a785))
- Add the ability to publish mix messages from a connected wallet to other spv peers. ([6063027](https://github.com/itswisdomagain/btcd/commit/606302793f49e9f2fa8bfdf9898ee01a4600bc20))

#### btcwallet

- Add initial support for mixing by passing a mixpool to the neutrino spv chain client for processing and storing mix messages received from other peers. ([c9e91af](https://github.com/itswisdomagain/btcd/commit/c9e91af5d2fe4a2e4c8458cafab7f9656de6394a))
- Add mixclient to wallet for creating, sending and processing mix messages. ([cc96a94](https://github.com/itswisdomagain/btcd/commit/cc96a94d04f9de6211fedfc13130bdaa912e50da))
- Implement wallet.MixAccount and wallet.MixOutput methods. ([b86403c](https://github.com/itswisdomagain/btcd/commit/b86403cf68c1971e0a2bbdea6e0c9e2020efe2b2))
- Add wallet automixer capability, to automatically mix change outputs on every new block. ([9454abf](https://github.com/itswisdomagain/btcd/commit/9454abff956857d67d1fa42d46ebd0a5322ec52f))
- Use btcd rpc for fee recommendation when mixing. ([1aadd1a](https://github.com/itswisdomagain/btcd/commit/1aadd1a5f5001b6312bf61eb2bdac59a8ba343f7))
- Add ability to perform mixes when connected to btcd node via rpc instead of spv. ([208192b](https://github.com/itswisdomagain/btcd/commit/208192b1cff6fbf4288c30eb6a059841adfa9e96))
- Implement wallet mixaccount and mixoutput json-rpc commands. ([c8f5dcc](https://github.com/itswisdomagain/btcd/commit/c8f5dcc14bd4728a162f877fa7af9be959ab8465))

### Next steps

- Skip mixing if using the recommended fee rate will result in a large portion of a participant's inputs being used as fees.
- Remove default btcd rpc connection parameters from codebase.
- Reset minimum number of required mix participants to 4.
- Permit mixing in mainnet network.
