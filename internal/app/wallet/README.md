# Layer 230 wallet integration

This is an explicitly enabled, partial server wallet surface using the published
`github.com/iamxvbaba/td v1.4.1` schema and exact profile 230. Profiles 225–229
reject these methods. The wallet is disabled by default: set
`TELESRV_WALLET_STATE_DIR` to a private directory to enable it. In Docker use
`/var/lib/telesrv/wallet` on the persisted server-state volume.

Implemented: authorization-bound five-minute ownership challenges, Ed25519
import proofs, SRP-protected replacement, generated custody keys, current
state, access-checked user address associations and paginated local nanoton
ledger history. Opening state creates the first wallet only in an explicitly
enabled deployment. Concurrent creation can reuse a successfully persisted
wallet; storage, challenge, master-key and ledger failures remain errors.
Every wallet method authenticates the account and auth key and applies its
rate limit. Address-only lookup is restricted to self.

The balance and transaction source is the existing local `ton_balances` /
`ton_transactions` ledger. Its configured first-read grant remains a local
server policy. These balances are not evidence of on-chain deposits or
redeemability. `TonUSDRate` is a fixed server reference rate, not a live quote.
There is no chain transfer, gasless relay, backup/export, NFT inventory, onramp
or TON Connect session implementation. Capability metadata must not advertise
these paths. Read-only empty session/NFT/provider lists are stubs; mutating
unsupported methods refuse or retain compatibility tracing. No wallet RPC
allocates PTS or emits wallet updates.

`TELESRV_WALLET_KEY_FILE` selects the AES-256-GCM master key (hex-encoded,
0600). The wallet directory is 0700 and account records are 0600, with a
single-writer process lock, file fsync, atomic rename and directory fsync.
Back up the complete directory and master key together. Startup verifies
stored custody keys against the configured cipher and their public key;
a missing/wrong key cannot silently make a usable wallet. Import replacement
clears the unrelated custody key. Challenges distinguish create/import replay.
No raw-key legacy encoding is inferred without deployment evidence.
The file-backed custody mode currently requires POSIX permissions; Linux
container tests exercise it without weakening the Windows permission checks.

No external TON connection is made unless `TELESRV_WALLET_TONCENTER_API_KEY`
is explicitly configured. The bounded HTTPS relay keeps a fixed toncenter.com
origin, validates paths/JSON/size, refuses redirects and automatic retries,
and never logs credentials, proofs, keys or provider response bodies.

Validation covers exact response encoding, historical-profile rejection,
authorization, SRP, peer isolation, cursor flags, missing dependencies,
creation/import replay, ciphertext isolation and restart. Live wallet UI and
real-chain transfers are not implied by these automated checks.
