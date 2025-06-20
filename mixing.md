# mixing instructions

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

### Setup the wallet(s)

- At least 2 separate btcwallet instances with mixing enabled are required. And at least one of the wallets must have access to the csppsolver binary (see above).

- To run separate btcwallet instances on the same computer, pass a separate appdata directory and rpclisten address to the second instance. E.g.:

```bash
$ ./btcwallet --testnet --appdata ~/.btcwallet2 --rpclisten 127.0.0.2:18332
```

- PS: The `--create` flag is required the first time each wallet is run, to create the wallet. The following sample commands will create 2 testnet wallets in the default appdata directory and the custom appdata directory.

```bash
$ ./btcwallet --testnet --create
$ ./btcwallet --testnet --appdata ~/.btcwallet2 --rpclisten 127.0.0.2:18332 --create
```

### Run the wallet(s)

- Run the wallet(s) with the following command-line args (modify as necessary) and allow the wallets to sync. **_Once the wallet(s) is/are fully synced, mixing will occur every time a new block is mined, as long as there are mixable outputs in the `changeaccount`._**

```bash
$ ./btcwallet --testnet --usespv -u "user" -P "pass" --mixing --mixchange --mixedaccount "mixed/1" --changeaccount "default"
```

- Alternatively, save the above command-line args to the config file and simply run `./btcwallet`.

- NOTE: If running multiple wallets on the same computer, specify different appdata directories and rpclisten addresses (as explained earlier).

### Additional information

Testnet wallets can be funded using a testnet faucet such as https://coinfaucet.eu/en/btc-testnet/.

First, generate a receive address from the wallet using `btcctl`.

- Download `btcctl`:

```bash
$ go install github.com/btcsuite/btcd/cmd/btcctl@latest
```

- Request a receive address using `btcctl`. Pass the rpclisten address, username, password and cert used for the wallet.

```bash
$ btcctl --wallet --testnet -s 127.0.0.2:18332 -u user -P pass -c ~/.btcwallet2/rpc.cert getaccountaddress "default"
```
